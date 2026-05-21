package statemachine

import "fmt"

const (
	StateDraft          = "draft"
	StateSubmitted      = "submitted"
	StateAIReviewing    = "ai_reviewing"
	StateHumanReviewing = "human_reviewing"
	StateRevising       = "revising"
	StateApproved       = "approved"
	StateRejected       = "rejected"
)

const (
	EventSave      = "save"
	EventSubmit    = "submit"
	EventEnqueue   = "enqueue"
	EventSkipAI    = "skip_ai"
	EventAIDone    = "ai_done"
	EventAIFailMax = "ai_fail_max"
	EventApprove   = "approve"
	EventReject    = "reject"
	EventRevise    = "revise"
)

type Key struct {
	From  string
	Event string
}

type Transition struct {
	From  string
	Event string
	To    []string
}

var transitions = map[Key]Transition{
	{StateDraft, EventSave}:             {From: StateDraft, Event: EventSave, To: []string{StateDraft}},
	{StateDraft, EventSubmit}:           {From: StateDraft, Event: EventSubmit, To: []string{StateSubmitted}},
	{StateSubmitted, EventEnqueue}:      {From: StateSubmitted, Event: EventEnqueue, To: []string{StateAIReviewing}},
	{StateSubmitted, EventSkipAI}:       {From: StateSubmitted, Event: EventSkipAI, To: []string{StateHumanReviewing}},
	{StateAIReviewing, EventAIDone}:     {From: StateAIReviewing, Event: EventAIDone, To: []string{StateApproved, StateHumanReviewing}},
	{StateAIReviewing, EventAIFailMax}:  {From: StateAIReviewing, Event: EventAIFailMax, To: []string{StateHumanReviewing}},
	{StateHumanReviewing, EventApprove}: {From: StateHumanReviewing, Event: EventApprove, To: []string{StateApproved}},
	{StateHumanReviewing, EventReject}:  {From: StateHumanReviewing, Event: EventReject, To: []string{StateRejected}},
	{StateHumanReviewing, EventRevise}:  {From: StateHumanReviewing, Event: EventRevise, To: []string{StateRevising}},
	{StateRevising, EventSubmit}:        {From: StateRevising, Event: EventSubmit, To: []string{StateSubmitted}},
}

func Can(from string, event string, to string) bool {
	transition, ok := transitions[Key{From: from, Event: event}]
	if !ok {
		return false
	}
	for _, allowed := range transition.To {
		if allowed == to {
			return true
		}
	}
	return false
}

func Apply(from string, event string, to string) error {
	if Can(from, event, to) {
		return nil
	}
	return fmt.Errorf("invalid transition: %s --%s--> %s", from, event, to)
}

func Transitions() []Transition {
	items := make([]Transition, 0, len(transitions))
	for _, transition := range transitions {
		items = append(items, transition)
	}
	return items
}
