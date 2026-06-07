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
	// findOrCreateSubmission 先查 (item,labeler) 既有 submission(无)→ 再新建,避免重复键。
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions. WHERE item_id`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
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

// TestClaim_ReusesExistingDraftSubmission 回归:releaseExpiredClaims 会把过期认领的 item 放回
// available 却保留其草稿 submission;同一 labeler 再领到同一题时,领新题分支必须复用既有草稿
// (findOrCreateSubmission)而非裸 INSERT,否则撞唯一键 uk_item_labeler 报 1062 → 500。
func TestClaim_ReusesExistingDraftSubmission(t *testing.T) {
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
		WillReturnResult(sqlmock.NewResult(0, 1))
	// 既有草稿命中 → 复用、不再 INSERT(否则重复键)。
	mock.ExpectQuery(`(?is)^SELECT.+FROM .submissions. WHERE item_id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "item_id", "labeler_id", "status"}).
			AddRow(555, 1, 7, 5, statemachine.StateDraft))
	mock.ExpectCommit()

	result, err := Claim(db, ClaimInput{TaskID: 1, LabelerID: 5})
	if err != nil {
		t.Fatalf("claim errored: %v", err)
	}
	if result.Submission.ID != 555 {
		t.Fatalf("expected to reuse existing submission 555, got %+v", result.Submission)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
