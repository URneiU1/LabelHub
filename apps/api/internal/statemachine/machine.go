package statemachine

import "fmt"

const (
	StateDraft             = "draft"
	StateSubmitted         = "submitted"
	StateAIReviewing       = "ai_reviewing"
	StateHumanReviewing    = "human_reviewing"
	// StateManualReview 是 AI 综合判定为「可疑(uncertain)」时专属的转人工复核入口态(对齐审核流程图独立分支)。
	// 它与 StateHumanReviewing 一样是初审入口,但区分来源:可疑 → manual_review,AI 通过 → human_reviewing。
	StateManualReview      = "manual_review"
	StateNeedsArbitration  = "needs_arbitration"
	StateConsensusEvidence = "consensus_evidence"
	StateRevising          = "revising"
	StateApproved          = "approved"
	StateRejected          = "rejected"
)

const (
	EventSave                 = "save"
	EventSubmit               = "submit"
	EventEnqueue              = "enqueue"
	EventSkipAI               = "skip_ai"
	EventAIDone               = "ai_done"
	EventAIAutoApproved       = "ai_auto_approved"
	// EventAIUncertain 表示 AI 综合判定为可疑,转人工复核(进入 manual_review)。
	EventAIUncertain          = "ai_uncertain"
	EventAIReject             = "ai_reject"
	EventAIFailMax            = "ai_fail_max"
	EventConsensusConflict    = "consensus_conflict"
	EventConsensusEvidence    = "consensus_evidence"
	EventSamplingAutoApproved = "sampling_auto_approved"
	EventApprove              = "approve"
	EventReject               = "reject"
	EventRevise               = "revise"
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
	{StateDraft, EventSave}:                     {From: StateDraft, Event: EventSave, To: []string{StateDraft}},
	{StateDraft, EventSubmit}:                   {From: StateDraft, Event: EventSubmit, To: []string{StateSubmitted}},
	{StateSubmitted, EventEnqueue}:              {From: StateSubmitted, Event: EventEnqueue, To: []string{StateAIReviewing}},
	{StateSubmitted, EventSkipAI}:               {From: StateSubmitted, Event: EventSkipAI, To: []string{StateHumanReviewing}},
	{StateSubmitted, EventConsensusConflict}:    {From: StateSubmitted, Event: EventConsensusConflict, To: []string{StateNeedsArbitration}},
	{StateSubmitted, EventConsensusEvidence}:    {From: StateSubmitted, Event: EventConsensusEvidence, To: []string{StateConsensusEvidence}},
	{StateSubmitted, EventSamplingAutoApproved}: {From: StateSubmitted, Event: EventSamplingAutoApproved, To: []string{StateApproved}},
	{StateAIReviewing, EventAIDone}:             {From: StateAIReviewing, Event: EventAIDone, To: []string{StateApproved, StateHumanReviewing}},
	{StateAIReviewing, EventAIAutoApproved}:     {From: StateAIReviewing, Event: EventAIAutoApproved, To: []string{StateApproved}},
	{StateAIReviewing, EventAIUncertain}:        {From: StateAIReviewing, Event: EventAIUncertain, To: []string{StateManualReview}},
	{StateAIReviewing, EventAIReject}:           {From: StateAIReviewing, Event: EventAIReject, To: []string{StateRevising}},
	{StateAIReviewing, EventAIFailMax}:          {From: StateAIReviewing, Event: EventAIFailMax, To: []string{StateHumanReviewing}},
	{StateHumanReviewing, EventApprove}:         {From: StateHumanReviewing, Event: EventApprove, To: []string{StateApproved}},
	{StateHumanReviewing, EventReject}:          {From: StateHumanReviewing, Event: EventReject, To: []string{StateRejected}},
	{StateHumanReviewing, EventRevise}:          {From: StateHumanReviewing, Event: EventRevise, To: []string{StateRevising}},
	// 转人工复核(可疑)与初审做一致的人工动作:approve 进 human_reviewing 走终审,reject/revise 与初审一致。
	{StateManualReview, EventApprove}:           {From: StateManualReview, Event: EventApprove, To: []string{StateHumanReviewing}},
	{StateManualReview, EventReject}:            {From: StateManualReview, Event: EventReject, To: []string{StateRejected}},
	{StateManualReview, EventRevise}:            {From: StateManualReview, Event: EventRevise, To: []string{StateRevising}},
	{StateNeedsArbitration, EventApprove}:       {From: StateNeedsArbitration, Event: EventApprove, To: []string{StateApproved}},
	{StateNeedsArbitration, EventReject}:        {From: StateNeedsArbitration, Event: EventReject, To: []string{StateRejected}},
	{StateRevising, EventSubmit}:                {From: StateRevising, Event: EventSubmit, To: []string{StateSubmitted}},
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
