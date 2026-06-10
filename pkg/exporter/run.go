package exporter

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrTerminal 标记永久失败(编码/落盘错误):导出已被写成 failed,重投递重跑没有意义。
// 瞬时错误(加载行 / 状态更新等 DB 错误)不裹这个,交给上层(worker)决定重试。
var ErrTerminal = errors.New("exporter: terminal failure")

var errTransient = errors.New("exporter: transient failure")

// RunResult 是一次成功导出的产物元数据。
type RunResult struct {
	FilePath string
	FileSize uint64
	RowCount int
}

// Run 是异步导出执行核心(worker 调用,裸 *sql.DB):
// 幂等认领 queued→running → 加载行 → 编码原子落盘 → 标记 succeeded + 审计。
// 编码/落盘失败 → 标记 failed(error_msg 脱敏)并返回裹了 ErrTerminal 的错误(永久失败,不应重试);
// 加载/状态更新等瞬时 DB 错误 → 返回普通 error,由 worker 交给 asynq 重试。
func Run(ctx context.Context, db *sql.DB, exportID uint64, baseDir string) error {
	var (
		format         string
		fieldMap       sql.NullString
		includeReviews bool
		taskID         uint64
		status         string
	)
	err := db.QueryRowContext(ctx,
		`SELECT format, field_map, include_reviews, task_id, status FROM exports WHERE id = ?`, exportID).
		Scan(&format, &fieldMap, &includeReviews, &taskID, &status)
	if err != nil {
		return fmt.Errorf("exporter: load export %d: %w", exportID, err)
	}
	// 幂等:已完结(succeeded/failed)直接 noop,重复投递不重跑。
	if status != "queued" && status != "running" {
		return nil
	}
	// 认领:queued→running,RowsAffected 防并发重复执行。
	if status == "queued" {
		res, err := db.ExecContext(ctx, `UPDATE exports SET status='running' WHERE id = ? AND status='queued'`, exportID)
		if err != nil {
			return fmt.Errorf("exporter: mark running: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil // 被别的 worker 抢先认领
		}
	}

	result, runErr := encodeToFile(ctx, db, taskID, format, nullStrPtr(fieldMap), includeReviews, baseDir)
	if runErr != nil {
		if errors.Is(runErr, errTransient) {
			return runErr
		}
		if markErr := markExportFailed(ctx, db, exportID, runErr); markErr != nil {
			// Couldn't record the failure (transient DB issue). Return a retryable error so the
			// worker retries instead of SkipRetry-ing on ErrTerminal, which would strand the
			// export in 'running' forever with no download link and no error message.
			return fmt.Errorf("exporter: mark export %d failed: %w", exportID, markErr)
		}
		return fmt.Errorf("%w: %w", ErrTerminal, runErr)
	}

	if _, err := db.ExecContext(ctx,
		`UPDATE exports SET status='succeeded', file_path=?, file_size=?, row_count=?, finished_at=NOW() WHERE id = ?`,
		result.FilePath, result.FileSize, result.RowCount, exportID); err != nil {
		return fmt.Errorf("exporter: mark succeeded: %w", err)
	}
	writeExportAudit(ctx, db, exportID, "succeeded", "exported",
		map[string]any{"task_id": taskID, "format": format, "row_count": result.RowCount})
	return nil
}

func encodeToFile(ctx context.Context, db *sql.DB, taskID uint64, format string, fieldMap *string, includeReviews bool, baseDir string) (RunResult, error) {
	fm, err := ParseFieldMap(fieldMap)
	if err != nil {
		return RunResult{}, err
	}
	rows, err := LoadApprovedRows(ctx, db, taskID, fm.IncludeReviews || includeReviews)
	if err != nil {
		return RunResult{}, fmt.Errorf("%w: load approved rows: %w", errTransient, err)
	}
	var sample Row
	if len(rows) > 0 {
		sample = rows[0]
	}
	cols := fm.ColumnsOrDefault(sample)

	dir := filepath.Join(baseDir, fmt.Sprintf("%d", taskID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return RunResult{}, fmt.Errorf("exporter: mkdir: %w", err)
	}
	finalPath := filepath.Join(dir, randName()+FileExtension(format))
	tmpPath := finalPath + ".tmp"

	f, err := os.Create(tmpPath)
	if err != nil {
		return RunResult{}, fmt.Errorf("exporter: create temp: %w", err)
	}
	rowCount, encErr := Encode(format, f, cols, rows)
	if encErr == nil {
		encErr = f.Sync()
	}
	if cerr := f.Close(); encErr == nil {
		encErr = cerr
	}
	if encErr != nil {
		_ = os.Remove(tmpPath)
		return RunResult{}, encErr
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return RunResult{}, fmt.Errorf("exporter: rename: %w", err)
	}
	info, err := os.Stat(finalPath)
	if err != nil {
		return RunResult{}, err
	}
	// 存绝对路径:worker 与 api 进程 CWD 不同,相对路径会导致 api 下载找不到文件。
	absPath, err := filepath.Abs(finalPath)
	if err != nil {
		absPath = finalPath
	}
	return RunResult{FilePath: absPath, FileSize: uint64(info.Size()), RowCount: rowCount}, nil
}

// markExportFailed marks the export 'failed' (sanitized error). Returns an error if the status
// UPDATE itself fails so the caller can fall back to a retryable error instead of stranding the
// export in 'running'. The audit write stays best-effort.
func markExportFailed(ctx context.Context, db *sql.DB, exportID uint64, cause error) error {
	if _, err := db.ExecContext(ctx,
		`UPDATE exports SET status='failed', error_msg=?, finished_at=NOW() WHERE id = ?`,
		sanitizeExportError(cause), exportID); err != nil {
		return err
	}
	writeExportAudit(ctx, db, exportID, "failed", "failed", map[string]any{"error": sanitizeExportError(cause)})
	return nil
}

// writeExportAudit 尽力写一条 audit_logs(失败不影响导出结果,best-effort)。
func writeExportAudit(ctx context.Context, db *sql.DB, exportID uint64, toState, event string, payload map[string]any) {
	var payloadJSON any
	if payload != nil {
		if raw, err := json.Marshal(payload); err == nil {
			payloadJSON = string(raw)
		}
	}
	_, _ = db.ExecContext(ctx,
		`INSERT INTO audit_logs (entity_type, entity_id, to_state, actor_type, actor_id, event, payload)
		 VALUES ('export', ?, ?, 'system', NULL, ?, ?)`,
		exportID, toState, event, payloadJSON)
}

// sanitizeExportError 截断错误信息,避免把过长/底层细节灌进 error_msg(S3 教训)。
func sanitizeExportError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	const max = 480
	if len(msg) > max {
		return msg[:max]
	}
	return msg
}

func nullStrPtr(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	return &n.String
}

func randName() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "export"
	}
	return hex.EncodeToString(b[:])
}
