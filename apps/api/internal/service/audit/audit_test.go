package audit

import (
	"database/sql"
	"regexp"
	"strconv"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	return gormDB, mock, sqlDB
}

// 空 Payload → audit_logs.payload 字段写 NULL,不是 "null" / "{}"
// (PLAN §4 关键易忘点:不区分这三者会让前端审计时间线渲染歧义)
func TestWrite_NilPayloadWritesNull(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO .audit_logs`).
		WithArgs(
			"submission",     // entity_type
			uint64(42),       // entity_id
			nil,              // from_state(空字符串 → NULL)
			"submitted",      // to_state
			"user",           // actor_type
			uint64(7),        // actor_id
			"submit",         // event
			nil,              // payload(nil map → NULL)
			sqlmock.AnyArg(), // created_at
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	actorID := uint64(7)
	if err := Write(db, LogEntry{
		EntityType: "submission",
		EntityID:   42,
		FromState:  "",
		ToState:    "submitted",
		ActorType:  "user",
		ActorID:    &actorID,
		Event:      "submit",
		Payload:    nil,
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

// 非空 Payload → marshal 成 JSON 字符串
func TestWrite_PayloadMarshalled(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO `+"`audit_logs`")).
		WithArgs(
			"export",
			uint64(1),
			nil, // from_state 空
			"succeeded",
			"user",
			uint64(3),
			"exported",
			`{"format":"json"}`, // payload JSON
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	actorID := uint64(3)
	if err := Write(db, LogEntry{
		EntityType: "export",
		EntityID:   1,
		ToState:    "succeeded",
		ActorType:  "user",
		ActorID:    &actorID,
		Event:      "exported",
		Payload:    map[string]any{"format": "json"},
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

// FromState 非空 → 写入字符串值
func TestWrite_FromStatePopulated(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO `+"`audit_logs`")).
		WithArgs(
			"submission",
			uint64(5),
			"draft", // from_state 现在是字符串值,不是 NULL
			"submitted",
			"user",
			uint64(9),
			"submit",
			nil,
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	actorID := uint64(9)
	if err := Write(db, LogEntry{
		EntityType: "submission",
		EntityID:   5,
		FromState:  "draft",
		ToState:    "submitted",
		ActorType:  "user",
		ActorID:    &actorID,
		Event:      "submit",
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

// 防 stray strconv import error
var _ = strconv.Itoa
