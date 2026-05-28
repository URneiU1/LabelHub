package submission

import (
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"

	"labelhub-api/internal/model"
)

func TestBuildAIReviewPlanAnchorsPayloadAndKey(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	promptID := uint64(7)
	mock.ExpectQuery(`(?is)^SELECT.+FROM .ai_prompt_configs.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version", "model"}).AddRow(promptID, 1, 3, "mock-model"))

	plan, err := buildAIReviewPlan(db,
		model.Task{ID: 1, AIReviewEnabled: true, AIPromptID: &promptID},
		model.Submission{ID: 42},
		model.SubmissionRevision{ID: 901},
	)
	if err != nil {
		t.Fatalf("buildAIReviewPlan returned error: %v", err)
	}
	if !plan.Enabled {
		t.Fatal("AI review plan should be enabled")
	}
	if plan.IdempotencyKey == "" || len(plan.IdempotencyKey) != 64 {
		t.Fatalf("bad idempotency key: %q", plan.IdempotencyKey)
	}
	var payload aiReviewTaskPayload
	if err := json.Unmarshal([]byte(plan.Payload), &payload); err != nil {
		t.Fatalf("payload should be JSON: %v", err)
	}
	if payload.SubmissionID != 42 || payload.RevisionID != 901 || payload.PromptConfigID != promptID || payload.PromptVersion != 3 {
		t.Fatalf("payload not anchored: %+v", payload)
	}
	if payload.IdempotencyKey != plan.IdempotencyKey {
		t.Fatalf("payload key mismatch: %q vs %q", payload.IdempotencyKey, plan.IdempotencyKey)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestBuildAIReviewPlanSkipsWhenTaskAIReviewDisabled(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	promptID := uint64(7)
	plan, err := buildAIReviewPlan(db,
		model.Task{ID: 1, AIReviewEnabled: false, AIPromptID: &promptID},
		model.Submission{ID: 42},
		model.SubmissionRevision{ID: 901},
	)
	if err != nil {
		t.Fatalf("buildAIReviewPlan returned error: %v", err)
	}
	if plan.Enabled {
		t.Fatal("AI review plan should be disabled")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestBuildAIReviewPlanRejectsEnabledTaskWithoutActivePrompt(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	_, err := buildAIReviewPlan(db,
		model.Task{ID: 1, AIReviewEnabled: true},
		model.Submission{ID: 42},
		model.SubmissionRevision{ID: 901},
	)
	if err != ErrInvalidAIPrompt {
		t.Fatalf("expected ErrInvalidAIPrompt, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestBuildAIReviewPlanRejectsEnabledTaskWithMissingPrompt(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	promptID := uint64(7)
	mock.ExpectQuery(`(?is)^SELECT.+FROM .ai_prompt_configs.`).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := buildAIReviewPlan(db,
		model.Task{ID: 1, AIReviewEnabled: true, AIPromptID: &promptID},
		model.Submission{ID: 42},
		model.SubmissionRevision{ID: 901},
	)
	if err != ErrInvalidAIPrompt {
		t.Fatalf("expected ErrInvalidAIPrompt, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestSaveReloadsTaskBeforeAIPlanAndRejectsInvalidActivePrompt(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	activePromptID := uint64(7)
	claimedBy := uint64(7)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ai_review_enabled", "ai_prompt_id"}).
			AddRow(1, true, activePromptID))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "claimed_by", "status"}).
			AddRow(11, 1, claimedBy, ItemStatusClaimed))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "labeler_id", "status", "template_version"}).
			AddRow(42, 1, 11, claimedBy, "draft", 1))
	mock.ExpectQuery(`(?is)^SELECT MAX.+FROM .submission_revisions.`).
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(0))
	mock.ExpectExec(`(?is)^INSERT INTO .submission_revisions.`).
		WillReturnResult(sqlmock.NewResult(901, 1))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .ai_prompt_configs.`).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectRollback()

	_, err := Save(db, SaveInput{
		Task:      model.Task{ID: 1, AIReviewEnabled: false},
		Item:      model.TaskItem{ID: 11},
		AnswerRaw: []byte(`{"summary":"ok"}`),
		UserID:    claimedBy,
		Draft:     false,
	})
	if err != ErrInvalidAIPrompt {
		t.Fatalf("expected ErrInvalidAIPrompt from locked task snapshot, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}
