package main

import (
	"context"
	"encoding/json"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"

	"labelhub.local/exporter"
)

// handleExport 是 export 队列的 5 行适配器:解析 export_id → 调 exporter.Run。
// Run 已把 queued→running→succeeded/failed 的状态机和原子落盘都做完。
func (h workerHandlers) handleExport(ctx context.Context, t *asynq.Task) error {
	var p struct {
		ExportID uint64 `json:"export_id"`
	}
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		// payload 不可解析 → 不可重试
		h.logger.Warn("invalid export payload", zap.Error(err), zap.ByteString("payload", t.Payload()))
		return err
	}
	dir := envOrDefault("EXPORT_DIR", "./data/exports")
	if err := exporter.Run(ctx, h.db, p.ExportID, dir); err != nil {
		h.logger.Error("export run failed", zap.Uint64("export_id", p.ExportID), zap.Error(err))
		return err // Run 已标记 failed;返回 err 交给 asynq 决定是否重试
	}
	return nil
}
