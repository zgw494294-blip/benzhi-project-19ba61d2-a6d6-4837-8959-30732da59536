package domain

// TaskStatus 是受控试配任务的持久化状态。
type TaskStatus string

const (
	StatusDraft           TaskStatus = "draft"
	StatusCuring          TaskStatus = "curing"
	StatusUnderReview     TaskStatus = "under_review"
	StatusRemediation     TaskStatus = "remediation"
	StatusPendingApproval TaskStatus = "pending_approval"
	StatusFrozen          TaskStatus = "frozen"
	StatusReleased        TaskStatus = "released"
)
