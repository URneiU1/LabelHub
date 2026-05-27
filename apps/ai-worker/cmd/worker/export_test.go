package main

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hibiken/asynq"
	"go.uber.org/zap"
)

func TestHandleExport_InvalidPayloadReturnsError(t *testing.T) {
	h := workerHandlers{logger: zap.NewNop()}
	task := asynq.NewTask("export", []byte("{not json"))
	if err := h.handleExport(context.Background(), task); err == nil {
		t.Fatal("expected error for invalid payload")
	}
}

func TestHandleExport_RunsExport(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	// 已完结的导出 → exporter.Run 走 noop 分支(仅 SELECT),验证适配器正确把 export_id 透传给 Run。
	mock.ExpectQuery(`SELECT format.+FROM exports WHERE id`).
		WithArgs(uint64(55)).
		WillReturnRows(sqlmock.NewRows([]string{"format", "field_map", "include_reviews", "task_id", "status"}).
			AddRow("csv", nil, false, 5, "succeeded"))

	h := workerHandlers{db: db, logger: zap.NewNop()}
	task := asynq.NewTask("export", []byte(`{"export_id":55}`))
	if err := h.handleExport(context.Background(), task); err != nil {
		t.Fatalf("handleExport errored: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
