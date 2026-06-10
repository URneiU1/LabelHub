// Package export 封装任务结果导出。
//
// 导出走异步路径:Enqueue 单事务写 exports(queued) + outbox_events,outbox publisher
// 把它投到 asynq 的 export 队列,由 ai-worker 侧的 exporter 跑实际编码
// (JSON / JSONL / CSV / XLSX + Markdown);字段映射 / include_reviews 由 exporter 处理。
package export

import (
	"encoding/json"
	"errors"

	"gorm.io/gorm"

	"labelhub-api/internal/model"
	"labelhub-api/internal/service/audit"

	"labelhub.local/exporter"
)

const exportTopic = "export"

// ErrUnsupportedFormat: format 不在白名单内。
var ErrUnsupportedFormat = errors.New("export: unsupported format")

// EnqueueInput:异步导出入队参数。Task 已被 handler 做过 owner-check。
type EnqueueInput struct {
	Task           model.Task
	CreatedBy      uint64
	Format         string
	FieldMap       *string // 原始 JSON 字符串(可空)
	IncludeReviews bool
}

// Enqueue 单事务写 exports(queued) + outbox_events(topic=export) + audit(queued),
// 返回创建的 Export。outbox publisher 会自动把它入队到 asynq 的 export 队列。
func Enqueue(db *gorm.DB, in EnqueueInput) (model.Export, error) {
	if !exporter.SupportedFormat(in.Format) {
		return model.Export{}, ErrUnsupportedFormat
	}
	if _, err := exporter.ParseFieldMap(in.FieldMap); err != nil {
		return model.Export{}, err
	}

	record := model.Export{
		TaskID:         in.Task.ID,
		CreatedBy:      in.CreatedBy,
		Format:         in.Format,
		FieldMap:       in.FieldMap,
		IncludeReviews: in.IncludeReviews,
		Status:         "queued",
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		payload, err := json.Marshal(map[string]uint64{"export_id": record.ID})
		if err != nil {
			return err
		}
		if err := tx.Create(&model.OutboxEvent{Topic: exportTopic, Payload: string(payload), Status: "pending"}).Error; err != nil {
			return err
		}
		actorID := in.CreatedBy
		return audit.Write(tx, audit.LogEntry{
			EntityType: "export",
			EntityID:   record.ID,
			ToState:    "queued",
			ActorType:  "user",
			ActorID:    &actorID,
			Event:      "queued",
			Payload:    map[string]any{"task_id": in.Task.ID, "format": in.Format, "include_reviews": in.IncludeReviews},
		})
	})
	if err != nil {
		return model.Export{}, err
	}
	return record, nil
}

// ListByTask 返回某任务的导出历史(最新在前)。limit 非法时取 50。
func ListByTask(db *gorm.DB, taskID uint64, limit int) ([]model.Export, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var exports []model.Export
	err := db.Where("task_id = ?", taskID).Order("id DESC").Limit(limit).Find(&exports).Error
	return exports, err
}
