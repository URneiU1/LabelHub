package review

import (
	"testing"
	"time"

	"labelhub-api/internal/statemachine"
)

// Decode:reviewer 的 verdict 字符串映射到状态机 event + 目标态 + human_verdict 字段。
// 单/复数严格匹配:"approved" 不等于 "approve"。
func TestDecodeMapping(t *testing.T) {
	tests := []struct {
		verdict     string
		wantEvent   string
		wantTo      string
		wantVerdict string
		wantOK      bool
	}{
		{"approve", statemachine.EventApprove, statemachine.StateApproved, "approve", true},
		{"reject", statemachine.EventReject, statemachine.StateRejected, "reject", true},
		{"revise", statemachine.EventRevise, statemachine.StateRevising, "revise", true},
		{"approved", "", "", "", false},
		{"", "", "", "", false},
		{"YES", "", "", "", false},
	}

	for _, tt := range tests {
		event, to, verdict, ok := Decode(tt.verdict)
		if ok != tt.wantOK || event != tt.wantEvent || to != tt.wantTo || verdict != tt.wantVerdict {
			t.Errorf("Decode(%q) = (%q, %q, %q, %v); want (%q, %q, %q, %v)",
				tt.verdict, event, to, verdict, ok, tt.wantEvent, tt.wantTo, tt.wantVerdict, tt.wantOK)
		}
	}
}

// UpdatesFor:approve 必须同时写 status=approved + approved_at;reject/revise 不写 approved_at。
// 防止 dashboard "approved_at 排序" 拿到 NULL 时序混进 reject/revise 行。
func TestUpdatesForApprovedAtSemantics(t *testing.T) {
	now := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)

	approve := UpdatesFor(statemachine.StateApproved, "approve", now)
	if approve["status"] != statemachine.StateApproved || approve["human_verdict"] != "approve" {
		t.Errorf("approve missing status/verdict: %+v", approve)
	}
	if approve["approved_at"] != now {
		t.Errorf("approve must stamp approved_at, got %v", approve["approved_at"])
	}

	reject := UpdatesFor(statemachine.StateRejected, "reject", now)
	if _, present := reject["approved_at"]; present {
		t.Errorf("reject must NOT touch approved_at, got %+v", reject)
	}

	revise := UpdatesFor(statemachine.StateRevising, "revise", now)
	if _, present := revise["approved_at"]; present {
		t.Errorf("revise must NOT touch approved_at, got %+v", revise)
	}
}
