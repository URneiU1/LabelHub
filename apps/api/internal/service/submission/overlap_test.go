package submission

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"

	"labelhub-api/internal/model"
	"labelhub-api/internal/statemachine"
)

func TestRequiredOverlapForItemUsesCoverageBucket(t *testing.T) {
	task := model.Task{OverlapCount: 3, OverlapCoveragePct: 25}
	if got := requiredOverlapForItem(task, 24); got != 3 {
		t.Fatalf("item 24 overlap = %d, want 3", got)
	}
	if got := requiredOverlapForItem(task, 25); got != 1 {
		t.Fatalf("item 25 overlap = %d, want 1", got)
	}
	if got := requiredOverlapForItem(model.Task{OverlapCount: 1, OverlapCoveragePct: 100}, 1); got != 1 {
		t.Fatalf("single-label task overlap = %d, want 1", got)
	}
}

func TestTransitionConsensusPeersMarksEarlierSubmissionAsEvidenceWithAudit(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT .id. FROM .submissions. WHERE item_id = .+ AND id <> .+ AND status = .+ ORDER BY id ASC`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(41))
	mock.ExpectExec(`(?is)^UPDATE .submissions. SET .+status.+ WHERE id = .+ AND status = .+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).
		WillReturnResult(sqlmock.NewResult(71, 1))
	mock.ExpectCommit()

	if err := db.Transaction(func(tx *gorm.DB) error {
		return transitionConsensusPeers(tx, 11, 42)
	}); err != nil {
		t.Fatalf("transitionConsensusPeers returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
	if !statemachine.Can(statemachine.StateSubmitted, statemachine.EventConsensusEvidence, statemachine.StateConsensusEvidence) {
		t.Fatal("consensus evidence transition must remain registered")
	}
}

func TestTransitionArbitrationPeersMarksEarlierSubmissionWithAudit(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT .id. FROM .submissions. WHERE item_id = .+ AND id <> .+ AND status = .+ ORDER BY id ASC`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(41))
	mock.ExpectExec(`(?is)^UPDATE .submissions. SET .+status.+ WHERE id = .+ AND status = .+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?is)^INSERT INTO .audit_logs.`).
		WillReturnResult(sqlmock.NewResult(71, 1))
	mock.ExpectCommit()

	if err := db.Transaction(func(tx *gorm.DB) error {
		return transitionArbitrationPeers(tx, 11, 42)
	}); err != nil {
		t.Fatalf("transitionArbitrationPeers returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
	if !statemachine.Can(statemachine.StateSubmitted, statemachine.EventConsensusConflict, statemachine.StateNeedsArbitration) {
		t.Fatal("consensus conflict transition must remain registered")
	}
}

func TestTransitionArbitrationPeersDetectsLostUpdate(t *testing.T) {
	db, mock, sqlDB := newSubmissionMockDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?is)^SELECT .id. FROM .submissions. WHERE item_id = .+ AND id <> .+ AND status = .+ ORDER BY id ASC`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(41))
	mock.ExpectExec(`(?is)^UPDATE .submissions. SET .+status.+ WHERE id = .+ AND status = .+`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	err := db.Transaction(func(tx *gorm.DB) error {
		return transitionArbitrationPeers(tx, 11, 42)
	})
	if err != ErrClaimRaceLost {
		t.Fatalf("err = %v, want ErrClaimRaceLost", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestDecideOverlapOutcome(t *testing.T) {
	task := model.Task{OverlapCount: 2, OverlapCoveragePct: 100}
	if got := decideOverlapOutcome(task, 1, nil, []byte(`{"label":"pass"}`), nil); got != overlapWaiting {
		t.Fatalf("first answer outcome = %q, want %q", got, overlapWaiting)
	}
	if got := decideOverlapOutcome(task, 1, []string{`{"label":"pass"}`}, []byte(`{"label":"pass"}`), nil); got != overlapConsensus {
		t.Fatalf("matching answer outcome = %q, want %q", got, overlapConsensus)
	}
	if got := decideOverlapOutcome(task, 1, []string{`{"label":"pass"}`}, []byte(`{"label":"reject"}`), nil); got != overlapNeedsArbitration {
		t.Fatalf("conflicting answer outcome = %q, want %q", got, overlapNeedsArbitration)
	}
	if got := decideOverlapOutcome(task, 1,
		[]string{`{"label":"pass","evidence":"upload-a"}`},
		[]byte(`{"evidence":"upload-b","label":"pass"}`),
		[]string{"evidence"},
	); got != overlapConsensus {
		t.Fatalf("upload-only difference outcome = %q, want %q", got, overlapConsensus)
	}
}
