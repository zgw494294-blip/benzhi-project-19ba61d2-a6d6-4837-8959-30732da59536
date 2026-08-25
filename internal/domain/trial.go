package domain

import "time"

type AuditEntry struct {
	Sequence   int       `json:"sequence"`
	Version    int       `json:"version"`
	Type       string    `json:"type"`
	Actor      string    `json:"actor"`
	Summary    string    `json:"summary"`
	OccurredAt time.Time `json:"occurredAt"`
}

type TrialTask struct {
	ID                   string             `json:"id"`
	SiteName             string             `json:"siteName"`
	WallSection          string             `json:"wallSection"`
	SubstrateCondition   string             `json:"substrateCondition"`
	Owner                string             `json:"owner"`
	AcceptanceThresholds Thresholds         `json:"acceptanceThresholds"`
	Status               TaskStatus         `json:"status"`
	Version              int                `json:"version"`
	CreatedAt            time.Time          `json:"createdAt"`
	UpdatedAt            time.Time          `json:"updatedAt"`
	Formulas             []MortarFormula    `json:"formulas"`
	Panels               []TestPanel        `json:"panels"`
	FrozenBatch          *BatchSnapshot     `json:"frozenBatch,omitempty"`
	Credential           *ReleaseCredential `json:"credential,omitempty"`
	Audit                []AuditEntry       `json:"audit"`
}

type Event struct {
	Type       string     `json:"type"`
	TaskID     string     `json:"taskId"`
	Version    int        `json:"version"`
	Actor      string     `json:"actor"`
	OccurredAt time.Time  `json:"occurredAt"`
	Snapshot   *TrialTask `json:"snapshot,omitempty"`
}
