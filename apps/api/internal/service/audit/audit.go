// Package audit 提供 audit_logs 表的统一写入入口。
//
// 设计原则:
//   - 调用方传入开放的 *gorm.DB(可以是 outer db,也可以是 tx),让 audit 写入跟业务变更同事务
//   - 不接 gin / httpx,worker 进程也能直接调用(S3 ai-worker 写 AI 审核 audit 走这里)
//   - LogEntry 用值类型,避免到处 *string / *map 的零值歧义
package audit

import (
	"encoding/json"

	"gorm.io/gorm"

	"labelhub-api/internal/model"
)

// LogEntry 描述一次状态迁移或业务事件的审计快照。
// FromState 为空字符串表示"无 from"(例如 export 这种 one-shot 事件),会写入 NULL。
// Payload 为 nil 时不写 JSON 字段,留 NULL;非 nil 一律 marshal 成 JSON 字符串。
type LogEntry struct {
	EntityType string
	EntityID   uint64
	FromState  string
	ToState    string
	ActorType  string
	ActorID    *uint64
	Event      string
	Payload    map[string]any
}

// Write 把 entry 落到 audit_logs 表。出错返回 wrap 后的 error,
// 调用方通常在事务里直接 return 让 tx rollback。
func Write(db *gorm.DB, entry LogEntry) error {
	var payloadValue *string
	if entry.Payload != nil {
		raw, err := json.Marshal(entry.Payload)
		if err != nil {
			return err
		}
		value := string(raw)
		payloadValue = &value
	}

	var fromState model.NullString
	if entry.FromState != "" {
		fromState = model.StringFrom(entry.FromState)
	}

	log := model.AuditLog{
		EntityType: entry.EntityType,
		EntityID:   entry.EntityID,
		FromState:  fromState,
		ToState:    entry.ToState,
		ActorType:  entry.ActorType,
		ActorID:    entry.ActorID,
		Event:      entry.Event,
		Payload:    payloadValue,
	}
	return db.Create(&log).Error
}
