package workflow

import (
	"time"

	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/domain"
)

type ActionResult struct {
	Task           *domain.TrialTask         `json:"task"`
	Credential     *domain.ReleaseCredential `json:"credential,omitempty"`
	NextCheckpoint *int                      `json:"nextCheckpoint,omitempty"`
	RetestOutcome  *RetestOutcome            `json:"retestOutcome,omitempty"`
}

type RetestOutcome struct {
	Passed              bool               `json:"passed"`
	RequiresRemediation bool               `json:"requiresRemediation"`
	RetestID            string             `json:"retestId"`
	Round               int                `json:"round"`
	NewDeviations       []domain.Deviation `json:"newDeviations"`
}

type CredentialView struct {
	Credential       *domain.ReleaseCredential `json:"credential"`
	DigestValid      bool                      `json:"digestValid"`
	RecomputedDigest string                    `json:"recomputedDigest"`
	TaskStatus       domain.TaskStatus         `json:"taskStatus"`
	Audit            []domain.AuditEntry       `json:"audit"`
	LedgerSequence   uint64                    `json:"ledgerSequence"`
	LedgerDigest     string                    `json:"ledgerDigest"`
	CheckedAt        time.Time                 `json:"checkedAt"`
}
