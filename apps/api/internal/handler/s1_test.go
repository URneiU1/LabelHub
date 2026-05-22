package handler

import (
	"encoding/json"
	"testing"
	"time"

	"labelhub-api/internal/statemachine"
)

// 业务决策:reviewer 的 verdict 映射到状态机 event + 目标态 + human_verdict 字段
func TestReviewDecisionMapping(t *testing.T) {
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
		{"approved", "", "", "", false}, // 单数 vs 复数:必须严格匹配
		{"", "", "", "", false},
		{"YES", "", "", "", false},
	}

	for _, tt := range tests {
		event, to, verdict, ok := reviewDecision(tt.verdict)
		if ok != tt.wantOK || event != tt.wantEvent || to != tt.wantTo || verdict != tt.wantVerdict {
			t.Errorf("reviewDecision(%q) = (%q, %q, %q, %v); want (%q, %q, %q, %v)",
				tt.verdict, event, to, verdict, ok, tt.wantEvent, tt.wantTo, tt.wantVerdict, tt.wantOK)
		}
	}
}

// 导出器 / 审计 payload 解析:坏 JSON 应该原样回退而不是 panic
func TestMustJSONFallback(t *testing.T) {
	if got := mustJSON(`{"a":1}`); !mapsEqual(got, map[string]any{"a": float64(1)}) {
		t.Errorf("valid JSON mismatch: %v", got)
	}
	if got := mustJSON(`not json`); got != "not json" {
		t.Errorf("invalid JSON should fallback to raw, got %v", got)
	}
	if got := mustJSON(`[1,2,3]`); !sliceLen(got, 3) {
		t.Errorf("array JSON unparsed: %v", got)
	}
}

// storageKey 必须能去重:相邻调用产出不同 key,因为含纳秒时间戳
func TestStorageKeyUnique(t *testing.T) {
	a := storageKey("photo.png")
	b := storageKey("photo.png")
	if a == b {
		t.Fatalf("storageKey collision on consecutive calls: %q == %q", a, b)
	}
	if len(a) != 64 {
		t.Errorf("storageKey should be 64-char hex (sha256), got len=%d", len(a))
	}
}

// MIME 白名单边界:PLAN §3.1 规定 png/jpeg/webp/pdf/txt/json
func TestUploadMIMEWhitelist(t *testing.T) {
	good := []string{"image/png", "image/jpeg", "image/webp", "application/pdf", "text/plain", "application/json"}
	bad := []string{"text/html", "application/octet-stream", "image/gif", ""}

	for _, mime := range good {
		if _, ok := allowedUploadMIME[mime]; !ok {
			t.Errorf("MIME %q should be allowed but is not", mime)
		}
	}
	for _, mime := range bad {
		if _, ok := allowedUploadMIME[mime]; ok {
			t.Errorf("MIME %q should NOT be allowed but is", mime)
		}
	}

	if _, ok := imageMIMEs["image/png"]; !ok {
		t.Error("image/png must be classified as image to enforce 5MB limit")
	}
	if _, ok := imageMIMEs["application/pdf"]; ok {
		t.Error("application/pdf must NOT be image (5MB image cap should not apply to PDF)")
	}
}

// nextRevisionNo 在没有 revision 时应该返回 1,有的时候 max + 1
// 这是 PLAN §2 append-only + UK(submission_id, revision_no) 的核心契约
func TestNextRevisionNoSemantics(t *testing.T) {
	// 纯逻辑层面通过样本验证语义:
	// 这里测试我们对 sql.NullInt64 的处理,真正 DB 路径要 integration 测
	cases := []struct {
		validMax bool
		maxValue int64
		want     int
	}{
		{validMax: false, want: 1}, // 没 revision
		{validMax: true, maxValue: 1, want: 2},
		{validMax: true, maxValue: 5, want: 6},
		{validMax: true, maxValue: 99, want: 100},
	}
	for _, c := range cases {
		got := nextRevisionFromMax(c.validMax, c.maxValue)
		if got != c.want {
			t.Errorf("nextRevisionFromMax(valid=%v, max=%d) = %d; want %d", c.validMax, c.maxValue, got, c.want)
		}
	}
}

// PLAN §4.3 关键业务点:revising → submit 必须把 ai_verdict / ai_score / human_verdict 三字段同时清空
// 答辩讲"修订重走 AI 审核"时,这条规则是核心 — 漏清任何一个,旧 AI 判定会污染新一轮
func TestResubmitClearedFieldsWipesVerdicts(t *testing.T) {
	now := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	fields := resubmitClearedFields(statemachine.StateAIReviewing, now)

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

// PLAN §4.3 关键业务点:approve 必须同时写 status=approved + approved_at;reject/revise 不写 approved_at
// 防止后续 dashboard "approved_at 排序" 拿到 NULL 时序
func TestReviewUpdatesApprovedAtSemantics(t *testing.T) {
	now := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)

	approve := reviewUpdates(statemachine.StateApproved, "approve", now)
	if approve["status"] != statemachine.StateApproved || approve["human_verdict"] != "approve" {
		t.Errorf("approve missing status/verdict: %+v", approve)
	}
	if approve["approved_at"] != now {
		t.Errorf("approve must stamp approved_at, got %v", approve["approved_at"])
	}

	reject := reviewUpdates(statemachine.StateRejected, "reject", now)
	if _, present := reject["approved_at"]; present {
		t.Errorf("reject must NOT touch approved_at, got %+v", reject)
	}

	revise := reviewUpdates(statemachine.StateRevising, "revise", now)
	if _, present := revise["approved_at"]; present {
		t.Errorf("revise must NOT touch approved_at, got %+v", revise)
	}
}

func mapsEqual(got any, want map[string]any) bool {
	gotMap, ok := got.(map[string]any)
	if !ok {
		return false
	}
	a, _ := json.Marshal(gotMap)
	b, _ := json.Marshal(want)
	return string(a) == string(b)
}

func sliceLen(value any, want int) bool {
	arr, ok := value.([]any)
	if !ok {
		return false
	}
	return len(arr) == want
}
