package statemachine

import "testing"

// TestTaskTransitions 验证任务生命周期状态机的合法与非法迁移。
func TestTaskTransitions(t *testing.T) {
	legal := []struct {
		from  string
		event string
		to    string
	}{
		{TaskDraft, TaskEventPublish, TaskPublished},
		{TaskPublished, TaskEventPause, TaskPaused},
		{TaskPaused, TaskEventResume, TaskPublished},
		{TaskPublished, TaskEventEnd, TaskEnded},
		{TaskPaused, TaskEventEnd, TaskEnded},
	}
	for _, tc := range legal {
		if !CanTask(tc.from, tc.event, tc.to) {
			t.Errorf("expected legal: %s --%s--> %s", tc.from, tc.event, tc.to)
		}
		if err := ApplyTask(tc.from, tc.event, tc.to); err != nil {
			t.Errorf("ApplyTask(%s,%s,%s) errored: %v", tc.from, tc.event, tc.to, err)
		}
		target, ok := TaskTargetFor(tc.from, tc.event)
		if !ok || target != tc.to {
			t.Errorf("TaskTargetFor(%s,%s) = %q,%v want %q", tc.from, tc.event, target, ok, tc.to)
		}
	}

	illegal := []struct {
		from  string
		event string
		to    string
	}{
		{TaskDraft, TaskEventPause, TaskPaused},      // 草稿不能直接暂停
		{TaskDraft, TaskEventEnd, TaskEnded},         // 草稿不能直接结束
		{TaskEnded, TaskEventResume, TaskPublished},  // 已结束是终态
		{TaskEnded, TaskEventPublish, TaskPublished}, // 已结束不能再发布
		{TaskPublished, TaskEventResume, TaskPublished},
		{TaskPaused, TaskEventPause, TaskPaused},
	}
	for _, tc := range illegal {
		if CanTask(tc.from, tc.event, tc.to) {
			t.Errorf("expected illegal: %s --%s--> %s", tc.from, tc.event, tc.to)
		}
		if err := ApplyTask(tc.from, tc.event, tc.to); err == nil {
			t.Errorf("ApplyTask(%s,%s,%s) should error", tc.from, tc.event, tc.to)
		}
	}

	if _, ok := TaskTargetFor(TaskEnded, TaskEventEnd); ok {
		t.Error("TaskTargetFor on terminal/no-transition should be false")
	}
}
