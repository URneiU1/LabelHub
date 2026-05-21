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
		{StateAIReviewing, EventAIDone, StateApproved},
		{StateAIReviewing, EventAIDone, StateHumanReviewing},
		{StateAIReviewing, EventAIFailMax, StateHumanReviewing},
		{StateHumanReviewing, EventApprove, StateApproved},
		{StateHumanReviewing, EventReject, StateRejected},
		{StateHumanReviewing, EventRevise, StateRevising},
		{StateRevising, EventSubmit, StateSubmitted},
	}

	for _, tt := range tests {
		if err := Apply(tt.from, tt.event, tt.to); err != nil {
			t.Fatalf("Apply(%q, %q, %q) error = %v", tt.from, tt.event, tt.to, err)
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
	}

	for _, tt := range tests {
		if err := Apply(tt.from, tt.event, tt.to); err == nil {
			t.Fatalf("Apply(%q, %q, %q) expected error", tt.from, tt.event, tt.to)
		}
	}
}

func TestTransitionTableCoversPlan(t *testing.T) {
	if got := len(Transitions()); got != 10 {
		t.Fatalf("Transitions() len = %d, want 10", got)
	}
}
