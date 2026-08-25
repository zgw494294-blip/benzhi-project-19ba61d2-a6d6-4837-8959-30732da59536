package workflow

import "benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/domain"

type WriteMeta struct {
	ExpectedVersion int    `json:"expectedVersion"`
	IdempotencyKey  string `json:"idempotencyKey"`
}

type CreateTrialCommand struct {
	WriteMeta
	ID                   string            `json:"id,omitempty"`
	SiteName             string            `json:"siteName"`
	WallSection          string            `json:"wallSection"`
	SubstrateCondition   string            `json:"substrateCondition"`
	Owner                string            `json:"owner"`
	AcceptanceThresholds domain.Thresholds `json:"acceptanceThresholds"`
}

type RegisterPanelCommand struct {
	WriteMeta
	FormulaID string               `json:"formulaId,omitempty"`
	Formula   domain.MortarFormula `json:"formula,omitempty"`
	Panel     domain.TestPanel     `json:"panel,omitempty"`
	Panels    []domain.TestPanel   `json:"panels,omitempty"`
	Actor     string               `json:"actor"`
}

type MeasurementRecord struct {
	PanelID     string             `json:"panelId"`
	Measurement domain.Measurement `json:"measurement"`
}

type RecordMeasurementCommand struct {
	WriteMeta
	PanelID      string              `json:"panelId,omitempty"`
	Measurement  domain.Measurement  `json:"measurement,omitempty"`
	Measurements []MeasurementRecord `json:"measurements,omitempty"`
}

type ReviewDeviationCommand struct {
	WriteMeta
	PanelID     string `json:"panelId"`
	DeviationID string `json:"deviationId"`
	Conclusion  string `json:"conclusion"`
	Reviewer    string `json:"reviewer"`
	Note        string `json:"note"`
}

type RemediationCommand struct {
	WriteMeta
	PanelID string                 `json:"panelId"`
	Plan    domain.RemediationPlan `json:"plan"`
	Actor   string                 `json:"actor"`
}

type RetestCommand struct {
	WriteMeta
	PanelID     string             `json:"panelId"`
	PlanID      string             `json:"planId"`
	Measurement domain.Measurement `json:"measurement"`
}

type FreezeCommand struct {
	WriteMeta
	FormulaID         string `json:"formulaId"`
	PanelID           string `json:"panelId"`
	ApprovedBy        string `json:"approvedBy"`
	EligibilityDigest string `json:"eligibilityDigest"`
}

type ReleaseCommand struct {
	WriteMeta
	ApprovedBy string `json:"approvedBy"`
}
