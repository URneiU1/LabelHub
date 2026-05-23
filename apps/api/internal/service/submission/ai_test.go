package submission

import (
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"labelhub-api/internal/model"
)

func TestBuildAIReviewPlanAnchorsPayloadAndKey(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	promptID := uint64(7)
	mock.ExpectQuery(`(?is)^SELECT.+FROM .ai_prompt_configs.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "version"}).AddRow(promptID, 1, 3))

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
