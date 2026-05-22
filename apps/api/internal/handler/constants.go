package handler

// itemStatus* 是 task_items.status 列的字面值。handler 包内目前只用 available / claimed
// (task.go ImportItems 设 available;labeler.go ClaimItem 改 claimed)。
// finished 由 service/review 拥有,它在自己包内复制一份;handler 不需要 finished。
const (
	itemStatusAvailable = "available"
	itemStatusClaimed   = "claimed"
)
