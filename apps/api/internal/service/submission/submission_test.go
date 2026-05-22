package submission

import (
	"testing"
	"time"

	"labelhub-api/internal/statemachine"
)

// NextRevisionFromMax:无历史 → 1;有历史 max=N → N+1。PLAN §2 append-only + UK(submission_id, revision_no) 契约。
func TestNextRevisionFromMax(t *testing.T) {
	cases := []struct {
		name     string
		validMax bool
		maxValue int64
		want     int
	}{
		{"no revision yet", false, 0, 1},
		{"max=1 → 2", true, 1, 2},
		{"max=5 → 6", true, 5, 6},
		{"max=99 → 100", true, 99, 100},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NextRevisionFromMax(c.validMax, c.maxValue); got != c.want {
				t.Errorf("NextRevisionFromMax(valid=%v, max=%d) = %d; want %d", c.validMax, c.maxValue, got, c.want)
			}
		})
	}
}

// ResubmitClearedFields:revising → submit 时同事务必须把 ai_verdict / ai_score / human_verdict 三字段清空,
// 否则旧 AI 判定会污染新一轮。状态机迁移目标态 + submitted_at 也必须落上。
func TestResubmitClearedFieldsWipesVerdicts(t *testing.T) {
	now := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	fields := ResubmitClearedFields(statemachine.StateAIReviewing, now)

	if fields["status"] != statemachine.StateAIReviewing {
		t.Errorf("status not set to target: %v", fields["status"])
	}
	if fields["submitted_at"] != now {
		t.Errorf("submitted_at not stamped: %v", fields["submitted_at"])
	}
	for _, field := range []string{"ai_verdict", "ai_score", "human_verdict"} {
		value, present := fields[field]
		if !present {
			t.Errorf("%s missing from cleared-fields map", field)
		}
		if value != nil {
			t.Errorf("%s must be nil(写入 NULL),got %v", field, value)
		}
	}
}
