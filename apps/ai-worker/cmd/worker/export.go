package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"

	"labelhub.local/exporter"
)

// handleExport 是 export 队列的适配器:解析 export_id → 调 exporter.Run。
// Run 已把 queued→running→succeeded/failed 的状态机和原子落盘都做完。
// 重试策略:payload 不可解析、或编码/落盘永久失败(ErrTerminal,导出已写 failed)→ SkipRetry;
// 瞬时 DB 错误 → 返回 err 交给 asynq 重试。
func (h workerHandlers) handleExport(ctx context.Context, t *asynq.Task) error {
	var p struct {
		ExportID uint64 `json:"export_id"`
	}
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		// payload 不可解析,重试也不会变好 → 跳过重试
		h.logger.Warn("invalid export payload", zap.Error(err), zap.ByteString("payload", t.Payload()))
		return fmt.Errorf("%w: %w", asynq.SkipRetry, err)
	}
	dir := os.Getenv("EXPORT_DIR") // 启动 mustAbsExportDir 已校验为绝对路径
	if err := exporter.Run(ctx, h.db, p.ExportID, dir); err != nil {
		h.logger.Error("export run failed", zap.Uint64("export_id", p.ExportID), zap.Error(err))
		if errors.Is(err, exporter.ErrTerminal) {
			return fmt.Errorf("%w: %w", asynq.SkipRetry, err) // 已标记 failed,重试无意义
		}
		return err // 瞬时错误,交给 asynq 重试
	}
	return nil
}
