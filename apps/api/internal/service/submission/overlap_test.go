package submission

import (
	"testing"

	"labelhub-api/internal/model"
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

func TestDecideOverlapOutcome(t *testing.T) {
	task := model.Task{OverlapCount: 2, OverlapCoveragePct: 100}
	if got := decideOverlapOutcome(task, 1, nil, []byte(`{"label":"pass"}`)); got != overlapWaiting {
		t.Fatalf("first answer outcome = %q, want %q", got, overlapWaiting)
	}
	if got := decideOverlapOutcome(task, 1, []string{`{"label":"pass"}`}, []byte(`{"label":"pass"}`)); got != overlapConsensus {
		t.Fatalf("matching answer outcome = %q, want %q", got, overlapConsensus)
	}
	if got := decideOverlapOutcome(task, 1, []string{`{"label":"pass"}`}, []byte(`{"label":"reject"}`)); got != overlapNeedsArbitration {
		t.Fatalf("conflicting answer outcome = %q, want %q", got, overlapNeedsArbitration)
	}
}
