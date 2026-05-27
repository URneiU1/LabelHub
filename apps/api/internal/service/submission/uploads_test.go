package submission

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"labelhub-api/internal/model"
)

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
