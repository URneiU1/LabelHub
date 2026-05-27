package main

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hibiken/asynq"
	"go.uber.org/zap"

	"labelhub.local/exporter"
)

func TestHandleExport_InvalidPayloadSkipsRetry(t *testing.T) {
	h := workerHandlers{logger: zap.NewNop()}
	task := asynq.NewTask("export", []byte("{not json"))
	err := h.handleExport(context.Background(), task)
	if err == nil {
		t.Fatal("expected error for invalid payload")
	}
	// 不可解析的 payload 重试无意义 → 必须 SkipRetry。
	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("expected SkipRetry, got %v", err)
	}
}

func TestHandleExport_TerminalErrorSkipsRetry(t *testing.T) {
	t.Setenv("EXPORT_DIR", t.TempDir())
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	// 不支持的格式 → exporter.Run 编码失败 → 标记 failed 并返回 ErrTerminal。
	mock.ExpectQuery(`SELECT format.+FROM exports WHERE id`).WithArgs(uint64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"format", "field_map", "include_reviews", "task_id", "status"}).
			AddRow("xml", nil, false, 5, "queued"))
	mock.ExpectExec(`UPDATE exports SET status='running'`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT s.id.+FROM submissions`).WithArgs(uint64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"sid", "iid", "external_id", "payload", "answer"}))
	mock.ExpectExec(`UPDATE exports SET status='failed'`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO audit_logs`).WillReturnResult(sqlmock.NewResult(1, 1))

	h := workerHandlers{db: db, logger: zap.NewNop()}
	task := asynq.NewTask("export", []byte(`{"export_id":9}`))
	gotErr := h.handleExport(context.Background(), task)
	if !errors.Is(gotErr, asynq.SkipRetry) {
		t.Fatalf("terminal error should SkipRetry, got %v", gotErr)
	}
	if !errors.Is(gotErr, exporter.ErrTerminal) {
		t.Fatalf("expected ErrTerminal in chain, got %v", gotErr)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
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
