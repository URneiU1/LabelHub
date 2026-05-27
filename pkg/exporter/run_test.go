package exporter

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func newMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, mock
}

func exportRow(format, status string, taskID uint64) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"format", "field_map", "include_reviews", "task_id", "status"}).
		AddRow(format, nil, false, taskID, status)
}

func TestRun_NonQueuedIsNoop(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`SELECT format.+FROM exports WHERE id`).
		WithArgs(uint64(7)).
		WillReturnRows(exportRow("json", "succeeded", 5))

	if err := Run(context.Background(), db, 7, t.TempDir()); err != nil {
		t.Fatalf("noop run errored: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestRun_QueuedToSucceededWritesFile(t *testing.T) {
	db, mock := newMock(t)
	dir := t.TempDir()

	mock.ExpectQuery(`SELECT format.+FROM exports WHERE id`).WithArgs(uint64(7)).
		WillReturnRows(exportRow("json", "queued", 5))
	mock.ExpectExec(`UPDATE exports SET status='running'`).WithArgs(uint64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT s.id.+FROM submissions`).WithArgs(uint64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"sid", "iid", "external_id", "payload", "answer"}).
			AddRow(1, 11, "Q1", `{}`, `{"x":1}`))
	mock.ExpectExec(`UPDATE exports SET status='succeeded'`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), 1, uint64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO audit_logs`).WillReturnResult(sqlmock.NewResult(1, 1))

	if err := Run(context.Background(), db, 7, dir); err != nil {
		t.Fatalf("run errored: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}

	files, _ := filepath.Glob(filepath.Join(dir, "5", "*.json"))
	if len(files) != 1 {
		t.Fatalf("expected 1 json file, got %v", files)
	}
	data, _ := os.ReadFile(files[0])
	var arr []map[string]any
	if err := json.Unmarshal(data, &arr); err != nil || len(arr) != 1 {
		t.Fatalf("bad output file (%v): %s", err, data)
	}
	if arr[0]["external_id"] != "Q1" {
		t.Fatalf("unexpected row: %v", arr[0])
	}
	// 确认无半截 .tmp 残留(原子落盘)
	if tmp, _ := filepath.Glob(filepath.Join(dir, "5", "*.tmp")); len(tmp) != 0 {
		t.Fatalf("leftover temp files: %v", tmp)
	}
}

func TestRun_EncodeErrorMarksFailed(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`SELECT format.+FROM exports WHERE id`).WithArgs(uint64(7)).
		WillReturnRows(exportRow("xml", "queued", 5)) // 不支持的格式 → Encode 报错
	mock.ExpectExec(`UPDATE exports SET status='running'`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT s.id.+FROM submissions`).WithArgs(uint64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"sid", "iid", "external_id", "payload", "answer"}))
	mock.ExpectExec(`UPDATE exports SET status='failed'`).
		WithArgs(sqlmock.AnyArg(), uint64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO audit_logs`).WillReturnResult(sqlmock.NewResult(1, 1))

	if err := Run(context.Background(), db, 7, t.TempDir()); err != ErrUnsupportedFormat {
		t.Fatalf("expected ErrUnsupportedFormat, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
