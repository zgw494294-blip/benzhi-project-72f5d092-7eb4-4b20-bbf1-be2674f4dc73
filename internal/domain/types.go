package domain

import "time"

type BatchStatus string

const (
	StatusDraft          BatchStatus = "draft"
	StatusAnnotating     BatchStatus = "annotating"
	StatusAdjudicating   BatchStatus = "adjudicating"
	StatusReadyForReview BatchStatus = "ready_for_review"
	StatusFrozen         BatchStatus = "frozen"
	StatusReleased       BatchStatus = "released"
)

type ReviewBatch struct {
	BatchID                string                            `json:"batchID"`
	SurveySite             string                            `json:"surveySite"`
	CaptureWindowStart     time.Time                         `json:"captureWindowStart"`
	CaptureWindowEnd       time.Time                         `json:"captureWindowEnd"`
	AuthorizationStatement string                            `json:"authorizationStatement"`
	Status                 BatchStatus                       `json:"status"`
	Version                int64                             `json:"version"`
	CreatedAt              time.Time                         `json:"createdAt"`
	UpdatedAt              time.Time                         `json:"updatedAt"`
	Clips                  map[string]*RecordingClip         `json:"clips"`
	Submissions            map[string][]AnnotationSubmission `json:"submissions"`
	Conflicts              map[string]*ConflictCase          `json:"conflicts"`
	Manifest               *DatasetManifest                  `json:"manifest,omitempty"`
	Credential             *ReleaseCredential                `json:"credential,omitempty"`
}

type RecordingClip struct {
	ClipID                 string    `json:"clipID"`
	BatchID                string    `json:"batchID"`
	SourceName             string    `json:"sourceName"`
	DurationMillis         int64     `json:"durationMillis"`
	ContentDigest          string    `json:"contentDigest"`
	CaptureTimestamp       time.Time `json:"captureTimestamp"`
	AuthorizationConfirmed bool      `json:"authorizationConfirmed"`
	HumanVoiceDetected     bool      `json:"humanVoiceDetected"`
	RedactionNote          string    `json:"redactionNote,omitempty"`
}

type AnnotationSubmission struct {
	SubmissionID string    `json:"submissionID"`
	ClipID       string    `json:"clipID"`
	Round        int       `json:"round"`
	AnnotatorID  string    `json:"annotatorID"`
	SpeciesLabel string    `json:"speciesLabel"`
	StartMillis  int64     `json:"startMillis"`
	EndMillis    int64     `json:"endMillis"`
	Confidence   float64   `json:"confidence"`
	EvidenceNote string    `json:"evidenceNote"`
	Revision     int       `json:"revision"`
	SubmittedAt  time.Time `json:"submittedAt"`
}

type ConflictStatus string

const (
	ConflictOpen       ConflictStatus = "open"
	ConflictReannotate ConflictStatus = "awaiting_reannotation"
	ConflictReview     ConflictStatus = "awaiting_review"
	ConflictResolved   ConflictStatus = "resolved"
)

type ConflictCase struct {
	ConflictID        string             `json:"conflictID"`
	ClipID            string             `json:"clipID"`
	ReasonCode        string             `json:"reasonCode"`
	OverlapRatio      float64            `json:"overlapRatio"`
	Status            ConflictStatus     `json:"status"`
	Decision          string             `json:"decision,omitempty"`
	ResolvedLabel     string             `json:"resolvedLabel,omitempty"`
	ReviewerID        string             `json:"reviewerID,omitempty"`
	ResolutionNote    string             `json:"resolutionNote,omitempty"`
	ResolvedAt        *time.Time         `json:"resolvedAt,omitempty"`
	ReannotationRound int                `json:"reannotationRound,omitempty"`
	DecisionTrail     []ConflictDecision `json:"decisionTrail,omitempty"`
}

type ConflictDecision struct {
	Decision          string    `json:"decision"`
	ReviewerID        string    `json:"reviewerID"`
	ResolutionNote    string    `json:"resolutionNote"`
	ResolvedLabel     string    `json:"resolvedLabel,omitempty"`
	ReannotationRound int       `json:"reannotationRound,omitempty"`
	At                time.Time `json:"at"`
}

type ManifestEntry struct {
	ClipID        string `json:"clipID"`
	ContentDigest string `json:"contentDigest"`
	SpeciesLabel  string `json:"speciesLabel"`
	StartMillis   int64  `json:"startMillis"`
	EndMillis     int64  `json:"endMillis"`
	RedactionNote string `json:"redactionNote,omitempty"`
}

type DatasetManifest struct {
	SchemaVersion int             `json:"schemaVersion"`
	BatchID       string          `json:"batchID"`
	CreatedAt     time.Time       `json:"createdAt"`
	Entries       []ManifestEntry `json:"entries"`
	Digest        string          `json:"digest"`
}

type ReleaseCredential struct {
	CredentialID     string    `json:"credentialID"`
	BatchID          string    `json:"batchID"`
	Sequence         int64     `json:"sequence"`
	ManifestDigest   string    `json:"manifestDigest"`
	PreviousDigest   string    `json:"previousDigest"`
	CredentialDigest string    `json:"credentialDigest"`
	IssuerID         string    `json:"issuerID"`
	IssuedAt         time.Time `json:"issuedAt"`
}

type AuditEvent struct {
	Sequence     int64          `json:"sequence"`
	BatchID      string         `json:"batchID"`
	Action       string         `json:"action"`
	ActorID      string         `json:"actorID"`
	ActorRole    string         `json:"actorRole"`
	At           time.Time      `json:"at"`
	BatchVersion int64          `json:"batchVersion"`
	Details      map[string]any `json:"details,omitempty"`
}

type GateBlocker struct {
	Code    string `json:"code"`
	ClipID  string `json:"clipID,omitempty"`
	Message string `json:"message"`
}

type ItemError struct {
	Index      int    `json:"index"`
	ClipID     string `json:"clipID,omitempty"`
	ConflictID string `json:"conflictID,omitempty"`
	Field      string `json:"field,omitempty"`
	ReasonCode string `json:"reasonCode"`
	Message    string `json:"message"`
}
