package submission

import (
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"

	"labelhub-api/internal/model"
	"labelhub-api/internal/statemachine"
)

func TestTemplateVersionForTaskDefaultsOnlyWhenTaskHasNoTemplate(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	version, err := templateVersionForTask(db, model.Task{ID: 1})
	if err != nil {
		t.Fatalf("templateVersionForTask returned error: %v", err)
	}
	if version != 1 {
		t.Fatalf("version = %d, want default 1", version)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestTemplateVersionForTaskErrorsWhenReferencedTemplateMissing(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	templateID := uint64(101)
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_templates.+id.+task_id`).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := templateVersionForTask(db, model.Task{ID: 1, TemplateID: &templateID})
	if !errors.Is(err, ErrTaskTemplate) {
		t.Fatalf("error = %v, want ErrTaskTemplate", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

// NextRevisionFromMax:无历史 → 1;有历史 max=N → N+1。PLAN §2 append-only + UK(submission_id, revision_no) 契约。
func TestNextRevisionFromMax(t *testing.T) {
	cases := []struct {
		name     string
		validMax bool
		maxValue int64
		want     int
	}{
		{"no revision yet", false, 0, 1},
		{"max=1 → 2", true, 1, 2},
		{"max=5 → 6", true, 5, 6},
		{"max=99 → 100", true, 99, 100},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NextRevisionFromMax(c.validMax, c.maxValue); got != c.want {
				t.Errorf("NextRevisionFromMax(valid=%v, max=%d) = %d; want %d", c.validMax, c.maxValue, got, c.want)
			}
		})
	}
}

// ResubmitClearedFields:revising → submit 时同事务必须把 ai_verdict / ai_score / human_verdict 三字段清空,
// 否则旧 AI 判定会污染新一轮。状态机迁移目标态 + submitted_at 也必须落上。
func TestResubmitClearedFieldsWipesVerdicts(t *testing.T) {
	now := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	fields := ResubmitClearedFields(statemachine.StateAIReviewing, now)

	if fields["status"] != statemachine.StateAIReviewing {
		t.Errorf("status not set to target: %v", fields["status"])
	}
	if fields["submitted_at"] != now {
		t.Errorf("submitted_at not stamped: %v", fields["submitted_at"])
	}
	for _, field := range []string{"ai_verdict", "ai_score", "human_verdict"} {
		value, present := fields[field]
		if !present {
			t.Errorf("%s missing from cleared-fields map", field)
		}
		if value != nil {
			t.Errorf("%s must be nil(写入 NULL),got %v", field, value)
		}
	}
}

func TestLockClaimedItemRejectsExpiredLease(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	now := time.Date(2026, 6, 1, 4, 0, 0, 0, time.UTC)
	previousNow := NowUTC
	NowUTC = func() time.Time { return now }
	defer func() { NowUTC = previousNow }()

	labelerID := uint64(5)
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "status", "claimed_by", "claimed_at"}).
			AddRow(7, 1, ItemStatusClaimed, labelerID, now.Add(-31*time.Minute)))

	_, err := lockClaimedItem(db, model.Task{ID: 1, LeaseTimeoutMinutes: 30}, 7, labelerID)
	if !errors.Is(err, ErrLeaseExpired) {
		t.Fatalf("expected ErrLeaseExpired, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestSaveRejectsFirstSubmitAtDailyLimit(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	labelerID := uint64(5)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "daily_submission_limit_per_labeler"}).
			AddRow(1, 1))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "status", "claimed_by"}).
			AddRow(7, 1, ItemStatusClaimed, labelerID))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "labeler_id", "status"}).
			AddRow(9, 1, 7, labelerID, statemachine.StateDraft))
	mock.ExpectQuery(`(?is)^SELECT count.+FROM .submissions.`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()

	_, err := Save(db, SaveInput{
		Task:      model.Task{ID: 1},
		Item:      model.TaskItem{ID: 7},
		UserID:    labelerID,
		AnswerRaw: []byte(`{"label":"x"}`),
	})
	if !errors.Is(err, ErrDailySubmissionLimitReached) {
		t.Fatalf("expected ErrDailySubmissionLimitReached, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestSaveOverlapWaitsForPeerAndReleasesItem(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	labelerID := uint64(5)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "overlap_count", "overlap_coverage_pct"}).
			AddRow(1, 2, 100))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "status", "claimed_by"}).
			AddRow(7, 1, ItemStatusClaimed, labelerID))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "labeler_id", "status"}).
			AddRow(9, 1, 7, labelerID, statemachine.StateDraft))
	mock.ExpectQuery(`(?is)^SELECT MAX.+FROM .submission_revisions.`).
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(nil))
	mock.ExpectExec(`(?is)^INSERT INTO .submission_revisions.`).
		WillReturnResult(sqlmock.NewResult(101, 1))
	mock.ExpectQuery(`(?is)^SELECT submission_revisions.answer FROM .submissions. JOIN submission_revisions`).
		WillReturnRows(sqlmock.NewRows([]string{"answer"}))
	mock.ExpectExec(`(?is)^UPDATE .submissions. SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE .task_items. SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(9, statemachine.StateSubmitted))
	mock.ExpectCommit()

	result, err := Save(db, SaveInput{
		Task:      model.Task{ID: 1},
		Item:      model.TaskItem{ID: 7},
		UserID:    labelerID,
		AnswerRaw: []byte(`{"label":"pass"}`),
	})
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if result.Status != statemachine.StateSubmitted {
		t.Fatalf("status = %s, want submitted", result.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestSaveAutoApprovesUnsampledSubmissionWithoutAI(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	labelerID := uint64(5)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "review_sampling_pct", "human_review_enabled"}).
			AddRow(1, 0, true))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "status", "claimed_by"}).
			AddRow(7, 1, ItemStatusClaimed, labelerID))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "labeler_id", "status"}).
			AddRow(9, 1, 7, labelerID, statemachine.StateDraft))
	mock.ExpectQuery(`(?is)^SELECT MAX.+FROM .submission_revisions.`).
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(nil))
	mock.ExpectExec(`(?is)^INSERT INTO .submission_revisions.`).
		WillReturnResult(sqlmock.NewResult(101, 1))
	mock.ExpectExec(`(?is)^UPDATE .submissions. SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE .task_items. SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^UPDATE .tasks. SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).
		WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(9, statemachine.StateApproved))
	mock.ExpectCommit()

	result, err := Save(db, SaveInput{
		Task:      model.Task{ID: 1},
		Item:      model.TaskItem{ID: 7},
		UserID:    labelerID,
		AnswerRaw: []byte(`{"label":"pass"}`),
	})
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if result.Status != statemachine.StateApproved {
		t.Fatalf("status = %s, want approved", result.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}
