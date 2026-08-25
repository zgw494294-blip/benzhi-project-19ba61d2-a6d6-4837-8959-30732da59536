package domain

import "time"

type BatchSnapshot struct {
	TaskID            string             `json:"taskId"`
	SiteName          string             `json:"siteName"`
	WallSection       string             `json:"wallSection"`
	Thresholds        Thresholds         `json:"thresholds"`
	Formula           MortarFormula      `json:"formula"`
	Panel             TestPanel          `json:"panel"`
	ApprovedBy        string             `json:"approvedBy"`
	ApprovedAt        time.Time          `json:"approvedAt"`
	TaskVersion       int                `json:"taskVersion"`
	EligibilityDigest string             `json:"eligibilityDigest"`
	EligibilityChecks []EligibilityCheck `json:"eligibilityChecks"`
}

type EligibilityCheck struct {
	Code        string `json:"code"`
	Passed      bool   `json:"passed"`
	Message     string `json:"message"`
	EvidenceRef string `json:"evidenceRef,omitempty"`
}

type ReleaseCredential struct {
	CredentialNo   string        `json:"credentialNo"`
	TaskID         string        `json:"taskId"`
	BatchSnapshot  BatchSnapshot `json:"batchSnapshot"`
	ApprovedBy     string        `json:"approvedBy"`
	IssuedAt       time.Time     `json:"issuedAt"`
	Sequence       uint64        `json:"sequence"`
	PreviousDigest string        `json:"previousDigest"`
	ContentDigest  string        `json:"contentDigest"`
}
