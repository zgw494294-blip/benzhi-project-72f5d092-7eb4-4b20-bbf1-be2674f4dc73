package persistence

import (
	"encoding/json"
	"time"

	"acoustic-annotation-release/internal/domain"
)

const schemaVersion = 1

type CommitResult struct {
	Batch           *domain.ReviewBatch `json:"batch"`
	Audit           domain.AuditEvent   `json:"audit"`
	OperationResult json.RawMessage     `json:"operationResult,omitempty"`
	Replay          bool                `json:"replay"`
}

type MutationMetadata struct {
	Result       any
	AuditDetails map[string]any
}

type IdempotentRecord struct {
	Key         string          `json:"key"`
	BatchID     string          `json:"batchID"`
	Operation   string          `json:"operation"`
	Method      string          `json:"method,omitempty"`
	Resource    string          `json:"resource,omitempty"`
	ActorID     string          `json:"actorID,omitempty"`
	ActorRole   string          `json:"actorRole,omitempty"`
	RequestHash string          `json:"requestHash,omitempty"`
	Result      json.RawMessage `json:"result"`
	CreatedAt   time.Time       `json:"createdAt"`
}

type logPayload struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Batch         *domain.ReviewBatch `json:"batch"`
	Audit         domain.AuditEvent   `json:"audit"`
	Idempotency   IdempotentRecord    `json:"idempotency"`
}

type logFrame struct {
	SchemaVersion  int             `json:"schemaVersion"`
	Sequence       int64           `json:"sequence"`
	PreviousDigest string          `json:"previousDigest"`
	Payload        json.RawMessage `json:"payload"`
	Checksum       string          `json:"checksum"`
	Digest         string          `json:"digest"`
}

type snapshot struct {
	SchemaVersion int                            `json:"schemaVersion"`
	LastSequence  int64                          `json:"lastSequence"`
	LastDigest    string                         `json:"lastDigest"`
	Batches       map[string]*domain.ReviewBatch `json:"batches"`
	Idempotency   map[string]IdempotentRecord    `json:"idempotency"`
	Audit         []domain.AuditEvent            `json:"audit"`
}
