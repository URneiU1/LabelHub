package submission

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"labelhub-api/internal/statemachine"
)

// TestClaim_FirstComeServesNextAvailableItem 验证 first_come 分发:无既有认领时,
// 按 id 顺序用 FOR UPDATE SKIP LOCKED 领到下一个 available item 并建 draft submission。
func TestClaim_FirstComeServesNextAvailableItem(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "template_id"}).AddRow(1, "published", nil))
	// 无既有认领 → 空行 → ErrRecordNotFound,转入领新题分支。
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+claimed_by`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	// SKIP LOCKED 领到下一个 available(id=7)。
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+SKIP LOCKED`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "status"}).AddRow(7, 1, "available"))
	mock.ExpectExec(`(?is)^UPDATE .task_items. SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .submissions.`).
		WillReturnResult(sqlmock.NewResult(901, 1))
	mock.ExpectCommit()

	result, err := Claim(db, ClaimInput{TaskID: 1, LabelerID: 5})
	if err != nil {
		t.Fatalf("claim errored: %v", err)
	}
	if result.Item.ID != 7 || result.Item.Status != ItemStatusClaimed {
		t.Fatalf("unexpected item: %+v", result.Item)
	}
	if result.Submission.Status != statemachine.StateDraft {
		t.Fatalf("submission status = %s, want draft", result.Submission.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// TestClaim_RaceLostWhenItemTakenConcurrently 验证并发安全:当乐观 UPDATE 影响 0 行
// (item 已被另一并发 labeler 抢走)时返回 ErrClaimRaceLost,绝不重复发同一题。
func TestClaim_RaceLostWhenItemTakenConcurrently(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.+FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "template_id"}).AddRow(1, "published", nil))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+claimed_by`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .task_items.+SKIP LOCKED`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "status"}).AddRow(7, 1, "available"))
	mock.ExpectExec(`(?is)^UPDATE .task_items. SET`).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 行 → 已被抢走
	mock.ExpectRollback()

	_, err := Claim(db, ClaimInput{TaskID: 1, LabelerID: 5})
	if !errors.Is(err, ErrClaimRaceLost) {
		t.Fatalf("expected ErrClaimRaceLost, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
