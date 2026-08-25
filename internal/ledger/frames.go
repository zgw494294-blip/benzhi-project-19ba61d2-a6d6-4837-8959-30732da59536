package ledger

import (
	"encoding/json"

	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/domain"
)

// frame 是事件日志中带摘要链的单个持久化事实。
type frame struct {
	SchemaVersion  int             `json:"schemaVersion"`
	Sequence       uint64          `json:"sequence"`
	PreviousDigest string          `json:"previousDigest"`
	Digest         string          `json:"digest"`
	Event          domain.Event    `json:"event"`
	IdempotencyKey string          `json:"idempotencyKey"`
	RequestDigest  string          `json:"requestDigest"`
	Response       json.RawMessage `json:"response"`
}

type idempotencyRecord struct {
	RequestDigest string          `json:"requestDigest"`
	Response      json.RawMessage `json:"response"`
}

// projection 是从事件帧恢复的当前查询投影。
type projection struct {
	SchemaVersion int                          `json:"schemaVersion"`
	LastSequence  uint64                       `json:"lastSequence"`
	LastDigest    string                       `json:"lastDigest"`
	Tasks         map[string]*domain.TrialTask `json:"tasks"`
	Idempotency   map[string]idempotencyRecord `json:"idempotency"`
}
