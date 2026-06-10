package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"time"

	"gorm.io/gorm"

	"labelhub-api/internal/auth"
	"labelhub-api/internal/db"
	"labelhub-api/internal/model"
)

type seedUser struct {
	username    string
	displayName string
	email       string
	roles       []string
}

const demoPassword = "123456"

func main() {
	// 生产环境(GIN_MODE=release,见 deploy compose)默认拒绝运行,避免把 demo 账号和
	// 共享弱口令灌进真实库;确需在 release 下重置 demo 数据时显式 SEED_ALLOW_IN_PROD=true。
	if os.Getenv("GIN_MODE") == "release" && os.Getenv("SEED_ALLOW_IN_PROD") != "true" {
		log.Fatal("refusing to seed in release mode (GIN_MODE=release); set SEED_ALLOW_IN_PROD=true to override")
	}
	database := db.Init()
	db.RunMigrations()

	users := []seedUser{
		{username: "owner1", displayName: "任务负责人一号", email: "owner1@example.com", roles: []string{"owner"}},
		{username: "owner2", displayName: "任务负责人二号", email: "owner2@example.com", roles: []string{"owner"}},
		{username: "labeler1", displayName: "标注员一号", email: "labeler1@example.com", roles: []string{"labeler"}},
		{username: "labeler2", displayName: "标注员二号", email: "labeler2@example.com", roles: []string{"labeler"}},
		{username: "labeler3", displayName: "标注员三号", email: "labeler3@example.com", roles: []string{"labeler"}},
		{username: "reviewer1", displayName: "审核员一号", email: "reviewer1@example.com", roles: []string{"reviewer"}},
		{username: "reviewer2", displayName: "审核员二号", email: "reviewer2@example.com", roles: []string{"reviewer"}},
		{username: "admin1", displayName: "管理员", email: "admin1@example.com", roles: []string{"admin"}},
		{username: "system_ai", displayName: "AI 审核 Agent", email: "system@example.com", roles: []string{"system"}},
	}

	for _, user := range users {
		if err := upsertUser(database, user); err != nil {
			log.Fatalf("seed user %s: %v", user.username, err)
		}
	}
	if err := seedQAQuality(database); err != nil {
		log.Fatalf("seed qa_quality: %v", err)
	}
	if err := seedPreferenceCompare(database); err != nil {
		log.Fatalf("seed preference_compare: %v", err)
	}

	log.Print("seed ready; demo accounts share a fixed demo password (see seed source / docs, not logged)")
}

func upsertUser(database *gorm.DB, seed seedUser) error {
	passwordHash, err := auth.HashPassword(demoPassword)
	if err != nil {
		return err
	}

	return database.Transaction(func(tx *gorm.DB) error {
		user := model.User{Username: seed.username}
		if err := tx.Where(model.User{Username: seed.username}).
			Attrs(model.User{
				PasswordHash: passwordHash,
				DisplayName:  seed.displayName,
				Email:        seed.email,
				Status:       "active",
			}).
			FirstOrCreate(&user).Error; err != nil {
			return err
		}

		if err := tx.Model(&user).Updates(map[string]any{
			"password_hash": passwordHash,
			"display_name":  seed.displayName,
			"email":         seed.email,
			"status":        "active",
		}).Error; err != nil {
			return err
		}

		for _, role := range seed.roles {
			var count int64
			if err := tx.Model(&model.UserRole{}).
				Where("user_id = ? AND role = ?", user.ID, role).
				Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				if err := tx.Create(&model.UserRole{UserID: user.ID, Role: role}).Error; err != nil {
					return err
				}
			}
		}

		return nil
	})
}

type officialSeedConfig struct {
	TaskTitle       string
	TaskDescription string
	BaselinePath    string
	TemplatePath    string
	DatasetPath     string
	GoldenAnswer    func(map[string]any) map[string]any
}

func seedQAQuality(database *gorm.DB) error {
	return seedOfficialTask(database, officialSeedConfig{
		TaskTitle:       "官方 qa_quality 质检标注",
		TaskDescription: "基于官方 qa_quality 数据集的问答质量标注任务。",
		BaselinePath:    "tools/seed/datasets/qa_quality/标注要求.md",
		TemplatePath:    "tools/seed/templates/qa_quality_review.json",
		DatasetPath:     "tools/seed/datasets/qa_quality/json/qa_quality.json",
		GoldenAnswer:    qaQualityGoldenAnswer,
	})
}

func seedPreferenceCompare(database *gorm.DB) error {
	return seedOfficialTask(database, officialSeedConfig{
		TaskTitle:       "官方 preference_compare 偏好对比标注",
		TaskDescription: "基于官方 preference_compare 数据集的偏好对比标注任务。",
		BaselinePath:    "tools/seed/datasets/preference_compare/标注要求.md",
		TemplatePath:    "tools/seed/templates/preference_compare_review.json",
		DatasetPath:     "tools/seed/datasets/preference_compare/json/preference_compare.json",
		GoldenAnswer:    preferenceCompareGoldenAnswer,
	})
}

const defaultAIPromptTemplate = `请根据任务验收基线、题目 payload 和标注员 answer 做结构化预审。判断标注结果是否需要人工重点关注；AI 结论只作为人工审核参考，不自动入库。`

const defaultAIPromptDimensions = `[
  {"name":"相关性","description":"答案是否贴合题目与任务验收基线。","weight":0.35},
  {"name":"准确性","description":"结论、评分或偏好判断是否正确。","weight":0.45},
  {"name":"安全性","description":"是否存在安全、合规或明显不可接受内容。","weight":0.20}
]`

const defaultAIPromptModel = "doubao-seed-2.0-lite"
const defaultGoldenVerdict = "uncertain"

func seedOfficialTask(database *gorm.DB, config officialSeedConfig) error {
	return database.Transaction(func(tx *gorm.DB) error {
		var owner model.User
		if err := tx.Where("username = ?", "owner1").First(&owner).Error; err != nil {
			return err
		}
		var reviewer model.User
		if err := tx.Where("username = ?", "reviewer1").First(&reviewer).Error; err != nil {
			return err
		}

		baseline, err := os.ReadFile(projectFile(config.BaselinePath))
		if err != nil {
			return err
		}
		task := model.Task{
			OwnerID:             owner.ID,
			Title:               config.TaskTitle,
			Description:         model.StringFrom(config.TaskDescription),
			BaselineDescription: model.StringFrom(string(baseline)),
			Status:              "published",
			Distribution:        "first_come",
			AIReviewEnabled:     true,
			HumanReviewEnabled:  true,
			PublishedAt:         model.TimeFrom(time.Now().UTC()),
		}
		if err := tx.Where("title = ?", task.Title).Attrs(task).FirstOrCreate(&task).Error; err != nil {
			return err
		}
		prompt, err := seedDefaultAIPrompt(tx, task.ID, owner.ID)
		if err != nil {
			return err
		}
		activePromptID := task.AIPromptID
		if activePromptID == nil {
			activePromptID = &prompt.ID
		}
		task.AIPromptID = activePromptID
		if err := tx.Model(&task).Updates(map[string]any{
			"baseline_description": string(baseline),
			"status":               "published",
			"ai_review_enabled":    true,
			"human_review_enabled": true,
			"ai_prompt_id":         *activePromptID,
			"published_at":         time.Now().UTC(),
		}).Error; err != nil {
			return err
		}

		schema, err := os.ReadFile(projectFile(config.TemplatePath))
		if err != nil {
			return err
		}
		hash := sha256.Sum256(schema)
		template := model.TaskTemplate{
			TaskID:     task.ID,
			Version:    1,
			SchemaJSON: string(schema),
			SchemaHash: hex.EncodeToString(hash[:]),
			CreatedBy:  owner.ID,
		}
		var existing model.TaskTemplate
		err = tx.Where("task_id = ? AND version = ?", task.ID, 1).First(&existing).Error
		var existingPtr *model.TaskTemplate
		if err == nil {
			existingPtr = &existing
			template.ID = existing.ID
		}
		action := seedTemplateActions(task, existingPtr, template.SchemaHash)
		switch {
		case err == nil:
			if action.UpdateExisting {
				if err := tx.Model(&existing).Updates(map[string]any{
					"schema_json": template.SchemaJSON,
					"schema_hash": template.SchemaHash,
				}).Error; err != nil {
					return err
				}
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			if action.CreateTemplate {
				if err := tx.Create(&template).Error; err != nil {
					return err
				}
			}
		default:
			return err
		}
		if !action.CreateTemplate && template.ID == 0 {
			return errors.New("seed template action resolved without template id")
		}
		if err := tx.Where("task_id = ? AND user_id = ?", task.ID, reviewer.ID).
			Attrs(model.TaskReviewer{TaskID: task.ID, UserID: reviewer.ID, AssignedBy: &owner.ID, AssignedAt: time.Now().UTC()}).
			FirstOrCreate(&model.TaskReviewer{}).Error; err != nil {
			return err
		}
		// Seed 只负责补齐/更新官方 v1 模板。S2 之后 Owner 可能已经创建 v2/v3,
		// rerun seed 不能把当前模板指针回滚到 v1。
		if action.AttachToTask {
			if err := tx.Model(&task).Update("template_id", template.ID).Error; err != nil {
				return err
			}
		}

		items, err := loadSeedItems(config.DatasetPath)
		if err != nil {
			return err
		}
		for _, payload := range items {
			externalID, _ := payload["id"].(string)
			raw, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			item := model.TaskItem{
				TaskID:     task.ID,
				ExternalID: model.StringFrom(externalID),
				Payload:    string(raw),
				Status:     "available",
			}
			if err := tx.Where("task_id = ? AND external_id = ?", task.ID, externalID).Attrs(item).FirstOrCreate(&item).Error; err != nil {
				return err
			}
			if err := tx.Model(&item).Updates(map[string]any{"payload": string(raw)}).Error; err != nil {
				return err
			}
		}
		if err := seedGoldenSamples(tx, task.ID, *activePromptID, owner.ID, items, config.GoldenAnswer); err != nil {
			return err
		}

		return tx.Model(&task).Update("total_items", len(items)).Error
	})
}

func seedDefaultAIPrompt(tx *gorm.DB, taskID uint64, ownerID uint64) (model.AIPromptConfig, error) {
	prompt := model.AIPromptConfig{
		TaskID:         taskID,
		Version:        1,
		PromptTemplate: defaultAIPromptTemplate,
		Dimensions:     defaultAIPromptDimensions,
		PassThreshold:  80,
		UncertainMin:   60,
		Model:          defaultAIPromptModel,
		CreatedBy:      ownerID,
	}
	var existing model.AIPromptConfig
	err := tx.Where("task_id = ? AND version = ?", taskID, 1).First(&existing).Error
	switch {
	case err == nil:
		if err := tx.Model(&existing).Updates(map[string]any{
			"prompt_template": defaultAIPromptTemplate,
			"dimensions":      defaultAIPromptDimensions,
			"pass_threshold":  80,
			"uncertain_min":   60,
			"model":           defaultAIPromptModel,
		}).Error; err != nil {
			return model.AIPromptConfig{}, err
		}
		existing.PromptTemplate = defaultAIPromptTemplate
		existing.Dimensions = defaultAIPromptDimensions
		existing.PassThreshold = 80
		existing.UncertainMin = 60
		existing.Model = defaultAIPromptModel
		return existing, nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		if err := tx.Create(&prompt).Error; err != nil {
			return model.AIPromptConfig{}, err
		}
		return prompt, nil
	default:
		return model.AIPromptConfig{}, err
	}
}

func seedGoldenSamples(tx *gorm.DB, taskID uint64, promptID uint64, ownerID uint64, items []map[string]any, answerFor func(map[string]any) map[string]any) error {
	if answerFor == nil {
		return nil
	}
	limit := 2
	if len(items) < limit {
		limit = len(items)
	}
	for index := 0; index < limit; index++ {
		payload, err := json.Marshal(items[index])
		if err != nil {
			return err
		}
		answer, err := json.Marshal(answerFor(items[index]))
		if err != nil {
			return err
		}
		hash := sha256.Sum256(payload)
		payloadJSON := string(payload)
		answerJSON := string(answer)
		payloadHash := hex.EncodeToString(hash[:])
		notes := model.StringFrom("seed golden sample for mock AI dry-run")
		promptIDCopy := promptID
		sample := model.GoldenSample{
			TaskID:          taskID,
			AIPromptID:      &promptIDCopy,
			Payload:         payloadJSON,
			PayloadHash:     payloadHash,
			ExpectedAnswer:  answerJSON,
			ExpectedVerdict: defaultGoldenVerdict,
			Notes:           notes,
			CreatedBy:       ownerID,
		}
		if err := tx.Where("task_id = ? AND payload_hash = ?", taskID, sample.PayloadHash).Attrs(sample).FirstOrCreate(&sample).Error; err != nil {
			return err
		}
		if err := tx.Model(&sample).Updates(map[string]any{
			"ai_prompt_id":     promptID,
			"payload":          payloadJSON,
			"expected_answer":  answerJSON,
			"expected_verdict": defaultGoldenVerdict,
			"notes":            notes,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func qaQualityGoldenAnswer(payload map[string]any) map[string]any {
	prompt, _ := payload["prompt"].(string)
	return map[string]any{
		"relevance_score":     5,
		"accuracy_score":      5,
		"format_score":        5,
		"safety_score":        5,
		"issue_tags":          []string{"无明显问题"},
		"summary":             "答案整体符合验收要求",
		"comment":             "围绕题目作答，关键事实与参考答案一致，可进入人工终审。",
		"revision_suggestion": "无需修改。",
		"corrected_answer": map[string]any{
			"prompt": prompt,
		},
	}
}

func preferenceCompareGoldenAnswer(payload map[string]any) map[string]any {
	preferred, _ := payload["preferred"].(string)
	if preferred == "" {
		preferred = "A"
	}
	margin, _ := payload["margin"].(string)
	if margin == "" {
		margin = "明显优于"
	}
	dimensions, ok := payload["dimensions"].([]any)
	selectedDimensions := make([]string, 0, len(dimensions))
	if ok {
		for _, dimension := range dimensions {
			if name, ok := dimension.(string); ok {
				selectedDimensions = append(selectedDimensions, name)
			}
		}
	}
	if len(selectedDimensions) == 0 {
		selectedDimensions = []string{"准确性"}
	}
	note, _ := payload["annotator_note"].(string)
	if note == "" {
		note = "偏好结论与参考标注一致，可进入人工终审。"
	}
	return map[string]any{
		"preferred":           preferred,
		"margin":              margin,
		"safety_flag":         "否",
		"dimensions":          selectedDimensions,
		"summary":             "偏好判断符合参考答案",
		"annotator_note":      note,
		"revision_suggestion": "无需修改。",
		"structured_note": map[string]any{
			"seed": true,
		},
	}
}

type seedTemplateAction struct {
	CreateTemplate bool
	UpdateExisting bool
	AttachToTask   bool
}

func seedTemplateActions(task model.Task, existing *model.TaskTemplate, nextHash string) seedTemplateAction {
	return seedTemplateAction{
		CreateTemplate: existing == nil,
		UpdateExisting: existing != nil && existing.SchemaHash != nextHash,
		AttachToTask:   task.TemplateID == nil,
	}
}

func loadSeedItems(datasetPath string) ([]map[string]any, error) {
	raw, err := os.ReadFile(projectFile(datasetPath))
	if err != nil {
		return nil, err
	}
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func projectFile(path string) string {
	return filepath.Join("../..", path)
}
