package statemachine

import "fmt"

// 任务生命周期状态机,与提交(submission)状态机相互独立。
// 草稿 → 发布中 → 已暂停 ↔ 发布中 → 已结束。
//
// Task lifecycle state machine, separate from the submission state machine.
const (
	TaskDraft     = "draft"
	TaskPublished = "published"
	TaskPaused    = "paused"
	TaskEnded     = "ended"
)

const (
	TaskEventPublish = "publish"
	TaskEventPause   = "pause"
	TaskEventResume  = "resume"
	TaskEventEnd     = "end"
)

var taskTransitions = map[Key]Transition{
	{TaskDraft, TaskEventPublish}:   {From: TaskDraft, Event: TaskEventPublish, To: []string{TaskPublished}},
	{TaskPublished, TaskEventPause}: {From: TaskPublished, Event: TaskEventPause, To: []string{TaskPaused}},
	{TaskPaused, TaskEventResume}:   {From: TaskPaused, Event: TaskEventResume, To: []string{TaskPublished}},
	{TaskPublished, TaskEventEnd}:   {From: TaskPublished, Event: TaskEventEnd, To: []string{TaskEnded}},
	{TaskPaused, TaskEventEnd}:      {From: TaskPaused, Event: TaskEventEnd, To: []string{TaskEnded}},
}

// CanTask 判定任务状态机的迁移是否合法。
func CanTask(from string, event string, to string) bool {
	transition, ok := taskTransitions[Key{From: from, Event: event}]
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

// TaskTargetFor 返回某 (from, event) 对应的唯一目标状态;不存在则返回 ok=false。
// 任务状态机每个事件目标唯一,故无需调用方再传 to。
func TaskTargetFor(from string, event string) (string, bool) {
	transition, ok := taskTransitions[Key{From: from, Event: event}]
	if !ok || len(transition.To) != 1 {
		return "", false
	}
	return transition.To[0], true
}

// ApplyTask 校验任务状态迁移,非法返回 error。
func ApplyTask(from string, event string, to string) error {
	if CanTask(from, event, to) {
		return nil
	}
	return fmt.Errorf("invalid task transition: %s --%s--> %s", from, event, to)
}

// TaskTransitions 暴露任务状态机的全部合法迁移(供测试与文档使用)。
func TaskTransitions() []Transition {
	items := make([]Transition, 0, len(taskTransitions))
	for _, transition := range taskTransitions {
		items = append(items, transition)
	}
	return items
}

// TaskPoliciesFrozen:任务一旦发布,影响标注口径的配置(分发 / 配额 / 重叠 / 抽检率)必须冻结。
// paused / ended 仍属于已发布生命周期,不能绕过冻结重新改口径。
func TaskPoliciesFrozen(status string) bool {
	return status != TaskDraft
}

// TaskTemplateFrozen:模板 schema 比口径字段宽松——draft 和 paused 都允许调整。
// 暂停时没有标注员在作答,改模板会生成新版本、恢复后才生效,不会污染进行中的提交;
// 发布中(published)与已结束(ended)仍冻结,避免标注员作答途中 schema 变化。
func TaskTemplateFrozen(status string) bool {
	return status != TaskDraft && status != TaskPaused
}
