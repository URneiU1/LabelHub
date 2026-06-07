package statemachine

import "testing"

func TestAllPlannedTransitions(t *testing.T) {
	tests := []struct {
		from  string
		event string
		to    string
	}{
		{StateDraft, EventSave, StateDraft},
		{StateDraft, EventSubmit, StateSubmitted},
		{StateSubmitted, EventEnqueue, StateAIReviewing},
		{StateSubmitted, EventSkipAI, StateHumanReviewing},
		{StateAIReviewing, EventAIDone, StateHumanReviewing},
		{StateAIReviewing, EventAIUncertain, StateManualReview},
		{StateAIReviewing, EventAIReject, StateRevising},
		{StateAIReviewing, EventAIFailMax, StateHumanReviewing},
		{StateHumanReviewing, EventApprove, StateApproved},
		{StateHumanReviewing, EventReject, StateRejected},
		{StateHumanReviewing, EventRevise, StateRevising},
		{StateManualReview, EventApprove, StateHumanReviewing},
		{StateManualReview, EventReject, StateRejected},
		{StateManualReview, EventRevise, StateRevising},
		{StateRevising, EventSubmit, StateSubmitted},
		{StateSubmitted, EventConsensusConflict, StateNeedsArbitration},
		{StateSubmitted, EventConsensusEvidence, StateConsensusEvidence},
		{StateNeedsArbitration, EventApprove, StateApproved},
		{StateNeedsArbitration, EventReject, StateRejected},
		{StateSubmitted, EventSamplingAutoApproved, StateApproved},
	}

	for _, tt := range tests {
		if err := Apply(tt.from, tt.event, tt.to); err != nil {
			t.Fatalf("Apply(%q, %q, %q) error = %v", tt.from, tt.event, tt.to, err)
		}
	}
}

func TestTransitionsExposesEveryAllowedEdge(t *testing.T) {
	seen := map[Key]map[string]bool{}
	for _, transition := range Transitions() {
		if transition.From == "" || transition.Event == "" || len(transition.To) == 0 {
			t.Fatalf("invalid transition entry: %+v", transition)
		}
		key := Key{From: transition.From, Event: transition.Event}
		if seen[key] == nil {
			seen[key] = map[string]bool{}
		}
		for _, to := range transition.To {
			seen[key][to] = true
			if !Can(transition.From, transition.Event, to) {
				t.Fatalf("Can(%q,%q,%q) = false for exported transition", transition.From, transition.Event, to)
			}
		}
	}
	for _, tt := range plannedTransitions() {
		if !seen[Key{From: tt.from, Event: tt.event}][tt.to] {
			t.Fatalf("Transitions() missing %s --%s--> %s", tt.from, tt.event, tt.to)
		}
	}
}

func TestInvalidTransitions(t *testing.T) {
	tests := []struct {
		from  string
		event string
		to    string
	}{
		{StateDraft, EventApprove, StateApproved},
		{StateSubmitted, EventApprove, StateApproved},
		{StateHumanReviewing, EventSubmit, StateSubmitted},
		{StateApproved, EventRevise, StateRevising},
		{StateRejected, EventSubmit, StateSubmitted},
		{StateAIReviewing, EventAIDone, StateApproved},
	}

	for _, tt := range tests {
		if err := Apply(tt.from, tt.event, tt.to); err == nil {
			t.Fatalf("Apply(%q, %q, %q) expected error", tt.from, tt.event, tt.to)
		}
	}
}

func TestEveryStateRejectsAtLeastOneInvalidEvent(t *testing.T) {
	states := []string{
		StateDraft,
		StateSubmitted,
		StateAIReviewing,
		StateHumanReviewing,
		StateManualReview,
		StateNeedsArbitration,
		StateConsensusEvidence,
		StateApproved,
		StateRejected,
		StateRevising,
	}
	events := []string{
		EventSave,
		EventSubmit,
		EventEnqueue,
		EventSkipAI,
		EventAIDone,
		EventAIUncertain,
		EventAIFailMax,
		EventConsensusConflict,
		EventConsensusEvidence,
		EventSamplingAutoApproved,
		EventApprove,
		EventReject,
		EventRevise,
	}
	for _, state := range states {
		foundInvalid := false
		for _, event := range events {
			if Can(state, event, StateApproved) {
				continue
			}
			foundInvalid = true
			if err := Apply(state, event, StateApproved); err == nil {
				t.Fatalf("Apply(%q,%q,%q) unexpectedly succeeded", state, event, StateApproved)
			}
			break
		}
		if !foundInvalid {
			t.Fatalf("state %q did not expose an invalid event in test set", state)
		}
	}
}

func TestTransitionTableCoversPlan(t *testing.T) {
	if got := len(Transitions()); got != 20 {
		t.Fatalf("Transitions() len = %d, want 20", got)
	}
}

type plannedTransition struct {
	from  string
	event string
	to    string
}

func plannedTransitions() []plannedTransition {
	return []plannedTransition{
		{StateDraft, EventSave, StateDraft},
		{StateDraft, EventSubmit, StateSubmitted},
		{StateSubmitted, EventEnqueue, StateAIReviewing},
		{StateSubmitted, EventSkipAI, StateHumanReviewing},
		{StateAIReviewing, EventAIDone, StateHumanReviewing},
		{StateAIReviewing, EventAIUncertain, StateManualReview},
		{StateAIReviewing, EventAIReject, StateRevising},
		{StateAIReviewing, EventAIFailMax, StateHumanReviewing},
		{StateHumanReviewing, EventApprove, StateApproved},
		{StateHumanReviewing, EventReject, StateRejected},
		{StateHumanReviewing, EventRevise, StateRevising},
		{StateManualReview, EventApprove, StateHumanReviewing},
		{StateManualReview, EventReject, StateRejected},
		{StateManualReview, EventRevise, StateRevising},
		{StateRevising, EventSubmit, StateSubmitted},
		{StateSubmitted, EventConsensusConflict, StateNeedsArbitration},
		{StateSubmitted, EventConsensusEvidence, StateConsensusEvidence},
		{StateNeedsArbitration, EventApprove, StateApproved},
		{StateNeedsArbitration, EventReject, StateRejected},
		{StateSubmitted, EventSamplingAutoApproved, StateApproved},
	}
}
