//go:build integration

package integration

import (
	"bytes"
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/hibiken/asynq"
	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"go.uber.org/zap"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"labelhub-api/internal/model"
	exportsvc "labelhub-api/internal/service/export"
	"labelhub-api/internal/service/outbox"
	reviewsvc "labelhub-api/internal/service/review"
	"labelhub-api/internal/service/submission"

	"labelhub.local/exporter"
)

func TestMainFlowWithRealMySQLRedis(t *testing.T) {
	t.Setenv("LLM_ALLOWED_MODELS", "mock-model")
	exportDir, err := filepath.Abs(t.TempDir())
	if err != nil {
		t.Fatalf("abs export dir: %v", err)
	}
	t.Setenv("EXPORT_DIR", exportDir)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	db, sqlDB := startMySQL(t, ctx)
	redisAddr := startRedis(t, ctx)
	applyMigrations(t, ctx, sqlDB)
	seedMainFlow(t, ctx, sqlDB)

	task := loadTask(t, db)
	item := loadItem(t, db)

	first, err := submission.Save(db, submission.SaveInput{
		Task:      task,
		Item:      item,
		AnswerRaw: []byte(`{"label":"needs work"}`),
		UserID:    2,
		Draft:     false,
	})
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}
	if first.Status != "ai_reviewing" || first.CurrentRevisionID == nil {
		t.Fatalf("first submit = status %q revision %v, want ai_reviewing with revision", first.Status, first.CurrentRevisionID)
	}
	assertOutboxPublishedToRedis(t, ctx, db, redisAddr)

	simulateAIResult(t, ctx, sqlDB, first.ID, *first.CurrentRevisionID, "reject", 35)
	if _, err := reviewsvc.Apply(db, reviewsvc.ApplyInput{
		SubmissionID: first.ID,
		Verdict:      "revise",
		Reason:       "please fix the answer",
		ReviewerID:   3,
		Roles:        []string{"reviewer"},
	}); err != nil {
		t.Fatalf("revise review: %v", err)
	}

	second, err := submission.Save(db, submission.SaveInput{
		Task:      loadTask(t, db),
		Item:      loadItem(t, db),
		AnswerRaw: []byte(`{"label":"fixed"}`),
		UserID:    2,
		Draft:     false,
	})
	if err != nil {
		t.Fatalf("resubmit after revise: %v", err)
	}
	if second.Status != "ai_reviewing" || second.CurrentRevisionID == nil || *second.CurrentRevisionID == *first.CurrentRevisionID {
		t.Fatalf("second submit = status %q revision %v, want new ai_reviewing revision", second.Status, second.CurrentRevisionID)
	}

	simulateAIResult(t, ctx, sqlDB, second.ID, *second.CurrentRevisionID, "pass", 92)
	// 多级人工审核:approve 需 RequiredHumanReviewLevels(=3)次才定稿(per current revision)。
	// 前两次(初审/复审)只 advance stage,submission 停留 human_reviewing;第三次(终审)推到 approved。
	for level := 1; level <= reviewsvc.RequiredHumanReviewLevels; level++ {
		if _, err := reviewsvc.Apply(db, reviewsvc.ApplyInput{
			SubmissionID: second.ID,
			Verdict:      "approve",
			ReviewerID:   3,
			Roles:        []string{"reviewer"},
		}); err != nil {
			t.Fatalf("approve review level %d: %v", level, err)
		}
		if level < reviewsvc.RequiredHumanReviewLevels {
			var s model.Submission
			if err := db.First(&s, second.ID).Error; err != nil {
				t.Fatalf("reload submission after approve level %d: %v", level, err)
			}
			if s.Status != "human_reviewing" {
				t.Fatalf("after approve level %d: status %q, want human_reviewing", level, s.Status)
			}
		}
	}
	assertApproved(t, db, second.ID)

	record, err := exportsvc.Enqueue(db, exportsvc.EnqueueInput{
		Task:           loadTask(t, db),
		CreatedBy:      1,
		Format:         "jsonl",
		IncludeReviews: true,
	})
	if err != nil {
		t.Fatalf("enqueue export: %v", err)
	}
	if err := exporter.Run(ctx, sqlDB, record.ID, exportDir); err != nil {
		t.Fatalf("run export: %v", err)
	}
	assertExportSucceeded(t, db, record.ID)
}

func startMySQL(t *testing.T, ctx context.Context) (*gorm.DB, *sql.DB) {
	t.Helper()
	ctr, err := tcmysql.Run(ctx,
		"mysql:8.0.36",
		tcmysql.WithDatabase("labelhub_it"),
		tcmysql.WithUsername("root"),
		tcmysql.WithPassword("password"),
	)
	if err != nil {
		t.Fatalf("start mysql container: %v", err)
	}
	testcontainers.CleanupContainer(t, ctr)

	// clientFoundRows=true:让 UPDATE 的 RowsAffected 返回"匹配行数"而非"实际变更行数"。
	// review.Apply 的中间级 approve 用 `UPDATE ... SET updated_at=now WHERE status='human_reviewing'`
	// + RowsAffected==1 做乐观守卫;在快测里两次 approve 可能落在同一秒,updated_at 不变,
	// 默认的"变更行数"会读成 0 而误报 ErrConcurrentWrite。匹配行数语义才是该守卫的本意。
	dsn, err := ctr.ConnectionString(ctx, "parseTime=true", "multiStatements=true", "clientFoundRows=true")
	if err != nil {
		t.Fatalf("mysql dsn: %v", err)
	}
	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sql open mysql: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	db, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("gorm open mysql: %v", err)
	}
	return db, sqlDB
}

func startRedis(t *testing.T, ctx context.Context) string {
	t.Helper()
	ctr, err := tcredis.Run(ctx, "redis:7")
	if err != nil {
		t.Fatalf("start redis container: %v", err)
	}
	testcontainers.CleanupContainer(t, ctr)

	raw, err := ctr.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("redis connection string: %v", err)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse redis url %q: %v", raw, err)
	}
	return parsed.Host
}

func applyMigrations(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "migration", "*.up.sql"))
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	sort.Strings(files)
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read migration %s: %v", file, err)
		}
		if _, err := db.ExecContext(ctx, string(body)); err != nil {
			t.Fatalf("apply migration %s: %v", file, err)
		}
	}
}

func seedMainFlow(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	statements := []string{
		`INSERT INTO users (id, username, password_hash, display_name, status) VALUES
			(1, 'owner1', 'x', 'Owner', 'active'),
			(2, 'labeler1', 'x', 'Labeler', 'active'),
			(3, 'reviewer1', 'x', 'Reviewer', 'active')`,
		`INSERT INTO user_roles (user_id, role) VALUES (1, 'owner'), (2, 'labeler'), (3, 'reviewer')`,
		`INSERT INTO tasks (id, owner_id, title, baseline_description, status, ai_review_enabled, human_review_enabled, total_items)
			VALUES (1, 1, 'Integration Task', 'baseline', 'published', 1, 1, 1)`,
		`INSERT INTO task_templates (id, task_id, version, schema_json, created_by)
			VALUES (101, 1, 1, '{"fields":[]}', 1)`,
		`UPDATE tasks SET template_id = 101 WHERE id = 1`,
		`INSERT INTO ai_prompt_configs (id, task_id, version, prompt_template, dimensions, pass_threshold, uncertain_min, model, created_by)
			VALUES (33, 1, 1, 'review {{answer.label}}', '[{"name":"相关性"}]', 80, 60, 'mock-model', 1)`,
		`UPDATE tasks SET ai_prompt_id = 33 WHERE id = 1`,
		`INSERT INTO task_reviewers (task_id, user_id, assigned_by) VALUES (1, 3, 1)`,
		`INSERT INTO task_items (id, task_id, external_id, payload, status, claimed_by, claimed_at)
			VALUES (11, 1, 'item-1', '{"question":"q"}', 'claimed', 2, NOW())`,
	}
	for _, stmt := range statements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed statement failed: %v\n%s", err, stmt)
		}
	}
}

func loadTask(t *testing.T, db *gorm.DB) model.Task {
	t.Helper()
	var task model.Task
	if err := db.First(&task, 1).Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	return task
}

func loadItem(t *testing.T, db *gorm.DB) model.TaskItem {
	t.Helper()
	var item model.TaskItem
	if err := db.First(&item, 11).Error; err != nil {
		t.Fatalf("load item: %v", err)
	}
	return item
}

func assertOutboxPublishedToRedis(t *testing.T, ctx context.Context, db *gorm.DB, redisAddr string) {
	t.Helper()
	client := asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})
	defer client.Close()
	publisher := outbox.NewPublisher(db, client, zap.NewNop(), time.Millisecond, 10)
	if err := publisher.PublishOnce(ctx); err != nil {
		t.Fatalf("publish outbox: %v", err)
	}

	var event model.OutboxEvent
	if err := db.Where("topic = ?", "ai:review").Order("id ASC").First(&event).Error; err != nil {
		t.Fatalf("load ai outbox event: %v", err)
	}
	if event.Status != "published" {
		t.Fatalf("outbox status = %q, want published", event.Status)
	}
}

func simulateAIResult(t *testing.T, ctx context.Context, db *sql.DB, submissionID uint64, revisionID uint64, verdict string, score float64) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin ai simulation: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	dimensions := `[{"name":"相关性","score":88,"reason":"ok"}]`
	if _, err := tx.ExecContext(ctx,
		`UPDATE ai_reviews SET status = 'succeeded', verdict = ?, overall_score = ?, dimensions = ?, reason = ?, raw_response = '{}', finished_at = NOW()
		 WHERE submission_id = ? AND revision_id = ? AND status IN ('pending','running','failed')`,
		verdict, score, dimensions, "simulated ai result", submissionID, revisionID,
	); err != nil {
		t.Fatalf("update ai review: %v", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE submissions SET status = 'human_reviewing', ai_verdict = ?, ai_score = ? WHERE id = ? AND current_revision_id = ?`,
		verdict, score, submissionID, revisionID,
	); err != nil {
		t.Fatalf("update submission after ai: %v", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO audit_logs (entity_type, entity_id, from_state, to_state, actor_type, event, payload)
		 VALUES ('submission', ?, 'ai_reviewing', 'human_reviewing', 'ai_worker', 'ai_done', ?)`,
		submissionID, `{"integration":true}`,
	); err != nil {
		t.Fatalf("insert ai audit: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit ai simulation: %v", err)
	}
}

func assertApproved(t *testing.T, db *gorm.DB, submissionID uint64) {
	t.Helper()
	var sub model.Submission
	if err := db.First(&sub, submissionID).Error; err != nil {
		t.Fatalf("load approved submission: %v", err)
	}
	if sub.Status != "approved" || sub.HumanVerdict == nil || *sub.HumanVerdict != "approve" {
		t.Fatalf("submission = status %q human %v, want approved/approve", sub.Status, sub.HumanVerdict)
	}
	var item model.TaskItem
	if err := db.First(&item, sub.ItemID).Error; err != nil {
		t.Fatalf("load finished item: %v", err)
	}
	if item.Status != "finished" {
		t.Fatalf("item status = %q, want finished", item.Status)
	}
	var task model.Task
	if err := db.First(&task, sub.TaskID).Error; err != nil {
		t.Fatalf("load finished task: %v", err)
	}
	if task.FinishedItems != 1 {
		t.Fatalf("finished_items = %d, want 1", task.FinishedItems)
	}
}

func assertExportSucceeded(t *testing.T, db *gorm.DB, exportID uint64) {
	t.Helper()
	var record model.Export
	if err := db.First(&record, exportID).Error; err != nil {
		t.Fatalf("load export: %v", err)
	}
	if record.Status != "succeeded" || record.RowCount == nil || *record.RowCount != 1 || !record.FilePath.Valid {
		t.Fatalf("export = status %q rows %v file %v, want succeeded/1/file", record.Status, record.RowCount, record.FilePath)
	}
	body, err := os.ReadFile(record.FilePath.String)
	if err != nil {
		t.Fatalf("read export file: %v", err)
	}
	if !bytes.Contains(body, []byte(`"fixed"`)) || !strings.Contains(string(body), `"ai_review.verdict"`) {
		t.Fatalf("export file missing final answer/review fields: %s", string(body))
	}
}
