package model

import (
	"time"
)

// ============================================================
// 1. users
// ============================================================
type User struct {
	ID           uint64     `gorm:"primaryKey" json:"id"`
	Username     string     `gorm:"uniqueIndex;size:64" json:"username"`
	PasswordHash string     `gorm:"size:255" json:"-"`
	DisplayName  string     `gorm:"size:64" json:"displayName"`
	Email        string     `gorm:"size:128" json:"email"`
	Status       string     `gorm:"default:active" json:"status"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	Roles        []UserRole `gorm:"foreignKey:UserID" json:"roles"`
}

// ============================================================
// 2. user_roles
// ============================================================
type UserRole struct {
	ID     uint64 `gorm:"primaryKey" json:"id"`
	UserID uint64 `gorm:"uniqueIndex:uk_user_role" json:"userId"`
	Role   string `gorm:"uniqueIndex:uk_user_role" json:"role"`
}

func (UserRole) TableName() string { return "user_roles" }

// ============================================================
// 3. tasks
// ============================================================
type Task struct {
	ID                  uint64     `gorm:"primaryKey" json:"id"`
	OwnerID             uint64     `json:"ownerId"`
	Title               string     `gorm:"size:200" json:"title"`
	Description         NullString `json:"description"`
	RichDescription     *string    `gorm:"type:json" json:"richDescription"`
	Tags                *string    `gorm:"type:json" json:"tags"`
	RewardConfig        *string    `gorm:"type:json" json:"rewardConfig"`
	BaselineDescription NullString `json:"baselineDescription"`
	Status              string     `gorm:"default:draft" json:"status"`
	TemplateID          *uint64    `json:"templateId"`
	Distribution        string     `gorm:"default:first_come" json:"distribution"`
	QuotaPerUser        int        `json:"quotaPerUser"`
	AIReviewEnabled     bool       `gorm:"default:true" json:"aiReviewEnabled"`
	HumanReviewEnabled  bool       `gorm:"default:true" json:"humanReviewEnabled"`
	AIPromptID          *uint64    `gorm:"column:ai_prompt_id" json:"aiPromptId"`
	TotalItems          int        `json:"totalItems"`
	FinishedItems       int        `json:"finishedItems"`
	Deadline            NullTime   `json:"deadline"`
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
	PublishedAt         NullTime   `json:"publishedAt"`
}

// ============================================================
// 4. task_templates
// ============================================================
type TaskTemplate struct {
	ID         uint64    `gorm:"primaryKey" json:"id"`
	TaskID     uint64    `gorm:"uniqueIndex:uk_task_version" json:"taskId"`
	Version    int       `gorm:"uniqueIndex:uk_task_version;default:1" json:"version"`
	SchemaJSON string    `gorm:"type:json" json:"schemaJson"`
	SchemaHash string    `gorm:"size:64" json:"schemaHash"`
	FieldMap   *string   `gorm:"type:json" json:"fieldMap"`
	CreatedBy  uint64    `json:"createdBy"`
	CreatedAt  time.Time `json:"createdAt"`
}

func (TaskTemplate) TableName() string { return "task_templates" }

// ============================================================
// 5. task_items
// ============================================================
type TaskItem struct {
	ID         uint64     `gorm:"primaryKey" json:"id"`
	TaskID     uint64     `json:"taskId"`
	ExternalID NullString `gorm:"size:128" json:"externalId"`
	Payload    string     `gorm:"type:json" json:"payload"`
	Status     string     `gorm:"default:available" json:"status"`
	ClaimedBy  *uint64    `json:"claimedBy"`
	ClaimedAt  NullTime   `json:"claimedAt"`
	FinishedAt NullTime   `json:"finishedAt"`
	Priority   int        `json:"priority"`
	CreatedAt  time.Time  `json:"createdAt"`
}

func (TaskItem) TableName() string { return "task_items" }

// ============================================================
// 6. task_assignees
// ============================================================
type TaskAssignee struct {
	ID         uint64   `gorm:"primaryKey" json:"id"`
	TaskID     uint64   `gorm:"uniqueIndex:uk_task_user_item" json:"taskId"`
	UserID     uint64   `gorm:"uniqueIndex:uk_task_user_item" json:"userId"`
	ItemID     *uint64  `gorm:"uniqueIndex:uk_task_user_item" json:"itemId"`
	AssignedAt NullTime `json:"assignedAt"`
}

func (TaskAssignee) TableName() string { return "task_assignees" }

// ============================================================
// 7. submissions
// ============================================================
type Submission struct {
	ID                uint64    `gorm:"primaryKey" json:"id"`
	TaskID            uint64    `json:"taskId"`
	ItemID            uint64    `gorm:"uniqueIndex:uk_item" json:"itemId"`
	TemplateVersion   int       `json:"templateVersion"`
	LabelerID         uint64    `json:"labelerId"`
	CurrentRevisionID *uint64   `json:"currentRevisionId"`
	Status            string    `gorm:"default:draft" json:"status"`
	AIVerdict         *string   `gorm:"column:ai_verdict" json:"aiVerdict"`
	AIScore           *float64  `gorm:"column:ai_score" json:"aiScore"`
	HumanVerdict      *string   `json:"humanVerdict"`
	SubmittedAt       NullTime  `json:"submittedAt"`
	ApprovedAt        NullTime  `json:"approvedAt"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// ============================================================
// 8. submission_revisions
// ============================================================
type SubmissionRevision struct {
	ID           uint64    `gorm:"primaryKey" json:"id"`
	SubmissionID uint64    `gorm:"uniqueIndex:uk_sub_rev" json:"submissionId"`
	RevisionNo   int       `gorm:"uniqueIndex:uk_sub_rev" json:"revisionNo"`
	Answer       string    `gorm:"type:json" json:"answer"`
	Draft        bool      `json:"draft"`
	CreatedBy    uint64    `json:"createdBy"`
	CreatedAt    time.Time `json:"createdAt"`
}

func (SubmissionRevision) TableName() string { return "submission_revisions" }

// ============================================================
// 9. ai_reviews
// ============================================================
type AIReview struct {
	ID             uint64     `gorm:"primaryKey" json:"id"`
	SubmissionID   uint64     `json:"submissionId"`
	RevisionID     uint64     `json:"revisionId"`
	IdempotencyKey string     `gorm:"uniqueIndex;size:64" json:"idempotencyKey"`
	PromptVersion  int        `json:"promptVersion"`
	Verdict        string     `json:"verdict"`
	OverallScore   *float64   `json:"overallScore"`
	Dimensions     *string    `gorm:"type:json" json:"dimensions"`
	Reason         NullString `json:"reason"`
	RawResponse    *string    `gorm:"type:json" json:"rawResponse"`
	TokensInput    int        `json:"tokensInput"`
	TokensOutput   int        `json:"tokensOutput"`
	LatencyMS      int        `json:"latencyMs"`
	Status         string     `gorm:"default:pending" json:"status"`
	RetryCount     int        `json:"retryCount"`
	ErrorMsg       NullString `json:"errorMsg"`
	CreatedAt      time.Time  `json:"createdAt"`
	FinishedAt     NullTime   `json:"finishedAt"`
}

func (AIReview) TableName() string { return "ai_reviews" }

// ============================================================
// 10. human_reviews
// ============================================================
type HumanReview struct {
	ID           uint64     `gorm:"primaryKey" json:"id"`
	SubmissionID uint64     `json:"submissionId"`
	RevisionID   uint64     `json:"revisionId"`
	ReviewerID   uint64     `json:"reviewerId"`
	Stage        string     `gorm:"default:first" json:"stage"`
	Verdict      string     `json:"verdict"`
	Reason       NullString `json:"reason"`
	Patch        *string    `gorm:"type:json" json:"patch"`
	CreatedAt    time.Time  `json:"createdAt"`
}

func (HumanReview) TableName() string { return "human_reviews" }

// ============================================================
// 11. audit_logs
// ============================================================
type AuditLog struct {
	ID         uint64     `gorm:"primaryKey" json:"id"`
	EntityType string     `json:"entityType"`
	EntityID   uint64     `json:"entityId"`
	FromState  NullString `gorm:"size:32" json:"fromState"`
	ToState    string     `gorm:"size:32" json:"toState"`
	ActorType  string     `json:"actorType"`
	ActorID    *uint64    `json:"actorId"`
	Event      string     `gorm:"size:64" json:"event"`
	Payload    *string    `gorm:"type:json" json:"payload"`
	CreatedAt  time.Time  `json:"createdAt"`
}

func (AuditLog) TableName() string { return "audit_logs" }

// ============================================================
// 12. uploaded_files
// ============================================================
type UploadedFile struct {
	ID                   uint64    `gorm:"primaryKey" json:"id"`
	TaskID               uint64    `json:"taskId"`
	SubmissionRevisionID *uint64   `json:"submissionRevisionId"`
	StorageKey           string    `gorm:"uniqueIndex:uk_storage_key;size:255" json:"storageKey"`
	OriginalName         string    `gorm:"size:255" json:"originalName"`
	MimeType             string    `gorm:"size:128" json:"mimeType"`
	SizeBytes            uint64    `json:"sizeBytes"`
	Status               string    `gorm:"default:temp" json:"status"`
	CreatedBy            uint64    `json:"createdBy"`
	CreatedAt            time.Time `json:"createdAt"`
	AttachedAt           NullTime  `json:"attachedAt"`
}

func (UploadedFile) TableName() string { return "uploaded_files" }

// ============================================================
// 13. exports
// ============================================================
type Export struct {
	ID             uint64     `gorm:"primaryKey" json:"id"`
	TaskID         uint64     `json:"taskId"`
	CreatedBy      uint64     `json:"createdBy"`
	Format         string     `json:"format"`
	Filter         *string    `gorm:"type:json" json:"filter"`
	FieldMap       *string    `gorm:"type:json" json:"fieldMap"`
	IncludeReviews bool       `json:"includeReviews"`
	Status         string     `gorm:"default:queued" json:"status"`
	FilePath       NullString `gorm:"size:512" json:"filePath"`
	FileSize       *uint64    `json:"fileSize"`
	RowCount       *int       `json:"rowCount"`
	ErrorMsg       NullString `json:"errorMsg"`
	CreatedAt      time.Time  `json:"createdAt"`
	FinishedAt     NullTime   `json:"finishedAt"`
}

func (Export) TableName() string { return "exports" }

// ============================================================
// 14. ai_prompt_configs
// ============================================================
type AIPromptConfig struct {
	ID             uint64    `gorm:"primaryKey" json:"id"`
	TaskID         uint64    `gorm:"uniqueIndex:uk_task_version" json:"taskId"`
	Version        int       `gorm:"uniqueIndex:uk_task_version;default:1" json:"version"`
	PromptTemplate string    `gorm:"type:text" json:"promptTemplate"`
	Dimensions     string    `gorm:"type:json" json:"dimensions"`
	PassThreshold  float64   `gorm:"default:80.0" json:"passThreshold"`
	UncertainMin   float64   `gorm:"default:60.0" json:"uncertainMin"`
	Model          string    `gorm:"default:doubao-seed-2.0-lite;size:64" json:"model"`
	CreatedBy      uint64    `json:"createdBy"`
	CreatedAt      time.Time `json:"createdAt"`
}

func (AIPromptConfig) TableName() string { return "ai_prompt_configs" }

// ============================================================
// 15. golden_samples
// ============================================================
type GoldenSample struct {
	ID              uint64     `gorm:"primaryKey" json:"id"`
	TaskID          uint64     `json:"taskId"`
	AIPromptID      *uint64    `gorm:"column:ai_prompt_id" json:"aiPromptId"`
	Payload         string     `gorm:"type:json" json:"payload"`
	PayloadHash     string     `gorm:"uniqueIndex:uk_task_payload_hash;size:64" json:"payloadHash"`
	ExpectedAnswer  string     `gorm:"type:json" json:"expectedAnswer"`
	ExpectedVerdict string     `json:"expectedVerdict"`
	Notes           NullString `json:"notes"`
	CreatedBy       uint64     `json:"createdBy"`
	CreatedAt       time.Time  `json:"createdAt"`
}

func (GoldenSample) TableName() string { return "golden_samples" }

// ============================================================
// 16. ai_dry_runs
// ============================================================
type AIDryRun struct {
	ID         uint64     `gorm:"primaryKey" json:"id"`
	TaskID     uint64     `json:"taskId"`
	AIPromptID uint64     `gorm:"column:ai_prompt_id" json:"aiPromptId"`
	Status     string     `gorm:"default:queued" json:"status"`
	Result     *string    `gorm:"type:json" json:"result"`
	ErrorMsg   NullString `json:"errorMsg"`
	CreatedBy  uint64     `json:"createdBy"`
	CreatedAt  time.Time  `json:"createdAt"`
	FinishedAt NullTime   `json:"finishedAt"`
}

func (AIDryRun) TableName() string { return "ai_dry_runs" }

// ============================================================
// 17. outbox_events
// ============================================================
type OutboxEvent struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	Topic       string    `gorm:"size:64" json:"topic"`
	Payload     string    `gorm:"type:json" json:"payload"`
	Status      string    `gorm:"default:pending" json:"status"`
	RetryCount  int       `json:"retryCount"`
	CreatedAt   time.Time `json:"createdAt"`
	PublishedAt NullTime  `json:"publishedAt"`
}

func (OutboxEvent) TableName() string { return "outbox_events" }
