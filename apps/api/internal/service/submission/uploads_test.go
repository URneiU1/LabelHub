package submission

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"labelhub-api/internal/model"
)

// H-03:附件归属扫描必须递归进入 Group / Tabs,收集所有上传叶子字段名,
// 并跳过非上传字段。
func TestCollectFileUploadFieldNamesRecursesGroupsAndTabs(t *testing.T) {
	schema := fileUploadTemplateSchema{
		Fields: []fileUploadTemplateField{
			{Name: "topFile", Widget: "FileUpload"},
			{Name: "heroImage", Widget: "ImageUpload"},
			{Name: "note", Widget: "TextInput"},
			{Name: "g", Widget: "Group", Fields: []fileUploadTemplateField{
				{Name: "groupFile", Widget: "FileUpload"},
				{Name: "deep", Widget: "Group", Fields: []fileUploadTemplateField{
					{Name: "deepImage", Widget: "ImageUpload"},
				}},
			}},
			{Name: "t", Widget: "Tabs", Tabs: []fileUploadTemplateTab{
				{Fields: []fileUploadTemplateField{
					{Name: "tabFile", Widget: "FileUpload"},
					{Name: "tabNote", Widget: "TextInput"},
				}},
			}},
		},
	}
	got := collectFileUploadFieldNames(schema.Fields)
	want := []string{"topFile", "heroImage", "groupFile", "deepImage", "tabFile"}
	if len(got) != len(want) {
		t.Fatalf("collected %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("collected[%d] = %q, want %q (full %v)", i, got[i], want[i], got)
		}
	}
}

// H-03:嵌在 Group 里的 FileUpload 提交后必须绑定到 revision(标记 attached),
// 不能因为只扫根级字段而被漏掉、随后被临时文件清理器删除。
func TestAttachUploadedFilesBindsNestedGroupFileUpload(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	restoreNow := NowUTC
	NowUTC = func() time.Time { return time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC) }
	defer func() { NowUTC = restoreNow }()

	key := strings.Repeat("e", 64)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_templates.+task_id.+version`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "schema_json"}).
			AddRow(101, 1, 2, `{"fields":[{"name":"g","widget":"Group","fields":[{"name":"evidence","widget":"FileUpload"}]}]}`))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .uploaded_files.+storage_key.+task_id.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "storage_key", "status", "created_by"}).
			AddRow(301, 1, key, "temp", 7))
	mock.ExpectExec(`(?is)^UPDATE .uploaded_files. SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := db.Transaction(func(tx *gorm.DB) error {
		return attachUploadedFiles(tx,
			model.Task{ID: 1},
			model.Submission{ID: 42, TaskID: 1, TemplateVersion: 2},
			model.SubmissionRevision{ID: 901, SubmissionID: 42},
			[]byte(`{"evidence":["`+key+`"]}`),
			7,
		)
	})
	if err != nil {
		t.Fatalf("attachUploadedFiles for nested FileUpload returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestAttachUploadedFilesBindsImageUploadWhenMIMEIsImage(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	restoreNow := NowUTC
	NowUTC = func() time.Time { return time.Date(2026, 6, 9, 9, 0, 0, 0, time.UTC) }
	defer func() { NowUTC = restoreNow }()

	key := strings.Repeat("f", 64)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_templates.+task_id.+version`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "schema_json"}).
			AddRow(101, 1, 2, `{"fields":[{"name":"hero","widget":"ImageUpload"}]}`))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .uploaded_files.+storage_key.+task_id.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "storage_key", "status", "created_by", "mime_type"}).
			AddRow(301, 1, key, "temp", 7, "image/png"))
	mock.ExpectExec(`(?is)^UPDATE .uploaded_files. SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := db.Transaction(func(tx *gorm.DB) error {
		return attachUploadedFiles(tx,
			model.Task{ID: 1},
			model.Submission{ID: 42, TaskID: 1, TemplateVersion: 2},
			model.SubmissionRevision{ID: 901, SubmissionID: 42},
			[]byte(`{"hero":["`+key+`"]}`),
			7,
		)
	})
	if err != nil {
		t.Fatalf("attachUploadedFiles for ImageUpload returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestAttachUploadedFilesRejectsNonImageMIMEForImageUpload(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	key := strings.Repeat("9", 64)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_templates.+task_id.+version`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "schema_json"}).
			AddRow(101, 1, 2, `{"fields":[{"name":"hero","widget":"ImageUpload"}]}`))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .uploaded_files.+storage_key.+task_id.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "storage_key", "status", "created_by", "mime_type"}).
			AddRow(301, 1, key, "temp", 7, "application/pdf"))
	mock.ExpectRollback()

	err := db.Transaction(func(tx *gorm.DB) error {
		return attachUploadedFiles(tx,
			model.Task{ID: 1},
			model.Submission{ID: 42, TaskID: 1, TemplateVersion: 2},
			model.SubmissionRevision{ID: 901, SubmissionID: 42},
			[]byte(`{"hero":["`+key+`"]}`),
			7,
		)
	})
	if !errors.Is(err, ErrInvalidUploadedFile) {
		t.Fatalf("expected ErrInvalidUploadedFile, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func newSubmissionMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *sql.DB) {
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

func TestAttachUploadedFilesDedupesAndMarksAttached(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	restoreNow := NowUTC
	NowUTC = func() time.Time { return time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC) }
	defer func() { NowUTC = restoreNow }()

	key := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_templates.+task_id.+version`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "schema_json"}).
			AddRow(101, 1, 2, `{"fields":[{"name":"evidence","widget":"FileUpload"}]}`))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .uploaded_files.+storage_key.+task_id.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "storage_key", "status", "created_by"}).
			AddRow(301, 1, key, "temp", 7))
	mock.ExpectExec(`(?is)^UPDATE .uploaded_files. SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := db.Transaction(func(tx *gorm.DB) error {
		return attachUploadedFiles(tx,
			model.Task{ID: 1},
			model.Submission{ID: 42, TaskID: 1, TemplateVersion: 2},
			model.SubmissionRevision{ID: 901, SubmissionID: 42},
			[]byte(`{"evidence":["`+key+`","`+key+`"]}`),
			7,
		)
	})
	if err != nil {
		t.Fatalf("attachUploadedFiles returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestAttachUploadedFilesRejectsOtherOwnerAndCrossTask(t *testing.T) {
	cases := []struct {
		name      string
		fileTask  uint64
		createdBy uint64
	}{
		{name: "other owner", fileTask: 1, createdBy: 8},
		{name: "cross task", fileTask: 2, createdBy: 7},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, sqlDB := newSubmissionMockDB(t)
			defer sqlDB.Close()

			key := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
			mock.ExpectBegin()
			mock.ExpectQuery(`(?is)^SELECT.+FROM .task_templates.+task_id.+version`).
				WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "schema_json"}).
					AddRow(101, 1, 2, `{"fields":[{"name":"evidence","widget":"FileUpload"}]}`))
			mock.ExpectQuery(`(?is)^SELECT.+FROM .uploaded_files.+storage_key.+task_id.+FOR UPDATE`).
				WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "storage_key", "status", "created_by"}).
					AddRow(301, tc.fileTask, key, "temp", tc.createdBy))
			mock.ExpectRollback()

			err := db.Transaction(func(tx *gorm.DB) error {
				return attachUploadedFiles(tx,
					model.Task{ID: 1},
					model.Submission{ID: 42, TaskID: 1, TemplateVersion: 2},
					model.SubmissionRevision{ID: 901, SubmissionID: 42},
					[]byte(`{"evidence":["`+key+`"]}`),
					7,
				)
			})
			if !errors.Is(err, ErrInvalidUploadedFile) {
				t.Fatalf("expected ErrInvalidUploadedFile, got %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("expectations not met: %v", err)
			}
		})
	}
}

func TestAttachUploadedFilesAllowsAttachedFileFromSameSubmissionHistory(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	key := "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	oldRevisionID := uint64(801)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_templates.+task_id.+version`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "schema_json"}).
			AddRow(101, 1, 2, `{"fields":[{"name":"evidence","widget":"FileUpload"}]}`))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .uploaded_files.+storage_key.+task_id.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "storage_key", "status", "created_by", "submission_revision_id"}).
			AddRow(301, 1, key, "attached", 7, oldRevisionID))
	mock.ExpectQuery(`(?is)^SELECT .id. FROM .submission_revisions.`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(oldRevisionID))
	mock.ExpectCommit()

	err := db.Transaction(func(tx *gorm.DB) error {
		return attachUploadedFiles(tx,
			model.Task{ID: 1},
			model.Submission{ID: 42, TaskID: 1, TemplateVersion: 2},
			model.SubmissionRevision{ID: 901, SubmissionID: 42},
			[]byte(`{"evidence":["`+key+`"]}`),
			7,
		)
	})
	if err != nil {
		t.Fatalf("attached file from same submission history should be reusable, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestAttachUploadedFilesRejectsAttachedFileFromOtherSubmission(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	key := "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	oldRevisionID := uint64(801)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_templates.+task_id.+version`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "schema_json"}).
			AddRow(101, 1, 2, `{"fields":[{"name":"evidence","widget":"FileUpload"}]}`))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .uploaded_files.+storage_key.+task_id.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "storage_key", "status", "created_by", "submission_revision_id"}).
			AddRow(301, 1, key, "attached", 7, oldRevisionID))
	mock.ExpectQuery(`(?is)^SELECT .id. FROM .submission_revisions.`).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectRollback()

	err := db.Transaction(func(tx *gorm.DB) error {
		return attachUploadedFiles(tx,
			model.Task{ID: 1},
			model.Submission{ID: 42, TaskID: 1, TemplateVersion: 2},
			model.SubmissionRevision{ID: 901, SubmissionID: 42},
			[]byte(`{"evidence":["`+key+`"]}`),
			7,
		)
	})
	if !errors.Is(err, ErrInvalidUploadedFile) {
		t.Fatalf("expected ErrInvalidUploadedFile, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}
