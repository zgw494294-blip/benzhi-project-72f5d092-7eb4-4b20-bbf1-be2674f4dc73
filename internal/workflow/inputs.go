package workflow

import (
	"context"
	"strings"
	"time"

	"acoustic-annotation-release/internal/domain"
)

const (
	RoleAdministrator = "administrator"
	RoleAnnotator     = "annotator"
	RoleReviewer      = "reviewer"
)

type WriteContext struct {
	ActorID         string
	Role            string
	ExpectedVersion int64
	IdempotencyKey  string
	RequestContext  context.Context
}

func (c WriteContext) Validate(roles ...string) error {
	if strings.TrimSpace(c.ActorID) == "" {
		return domain.NewError(domain.CodeInvalid, "X-Actor-ID 不能为空")
	}
	if strings.TrimSpace(c.IdempotencyKey) == "" {
		return domain.NewError(domain.CodeInvalid, "Idempotency-Key 不能为空")
	}
	if c.ExpectedVersion < 0 {
		return domain.NewError(domain.CodeInvalid, "expectedVersion 不能为负数")
	}
	for _, allowed := range roles {
		if c.Role == allowed {
			return nil
		}
	}
	return domain.NewError(domain.CodeForbidden, "角色 %s 无权执行此操作", c.Role)
}

// CheckCanceled reports whether the request context has been canceled.
// It is used both as a pre-check before entering the repository commit
// path and as a re-check after acquiring the commit lock, so that a
// request canceled while queued on the commit lock does not persist
// events, snapshots or aggregate mutations.
func (c WriteContext) CheckCanceled() error {
	if c.RequestContext == nil {
		return nil
	}
	if err := c.RequestContext.Err(); err != nil {
		return domain.NewError(domain.CodeCanceled, "请求上下文已结束: %v", err)
	}
	return nil
}

type CreateBatchInput struct {
	BatchID                string    `json:"batchID"`
	SurveySite             string    `json:"surveySite"`
	CaptureWindowStart     time.Time `json:"captureWindowStart"`
	CaptureWindowEnd       time.Time `json:"captureWindowEnd"`
	AuthorizationStatement string    `json:"authorizationStatement"`
}

type AddClipInput struct {
	ClipID                 string    `json:"clipID"`
	SourceName             string    `json:"sourceName"`
	DurationMillis         int64     `json:"durationMillis"`
	ContentDigest          string    `json:"contentDigest"`
	CaptureTimestamp       time.Time `json:"captureTimestamp"`
	AuthorizationConfirmed bool      `json:"authorizationConfirmed"`
	HumanVoiceDetected     bool      `json:"humanVoiceDetected"`
	RedactionNote          string    `json:"redactionNote"`
}

type BulkAddClipsInput struct {
	Clips []AddClipInput `json:"clips"`
}

type AnnotationInput struct {
	ClipID       string  `json:"clipID"`
	Round        int     `json:"round"`
	SpeciesLabel string  `json:"speciesLabel"`
	StartMillis  int64   `json:"startMillis"`
	EndMillis    int64   `json:"endMillis"`
	Confidence   float64 `json:"confidence"`
	EvidenceNote string  `json:"evidenceNote"`
	Revision     int     `json:"revision"`
}

type ResolveConflictInput struct {
	Decision       string `json:"decision"`
	ResolvedLabel  string `json:"resolvedLabel"`
	ResolutionNote string `json:"resolutionNote"`
}

type BulkConflictDecisionsInput struct {
	Decisions []domain.ConflictDecisionInput `json:"decisions"`
}

type PrivacyInput struct {
	AuthorizationConfirmed bool   `json:"authorizationConfirmed"`
	HumanVoiceDetected     bool   `json:"humanVoiceDetected"`
	RedactionNote          string `json:"redactionNote"`
}

type GateRemediationsInput struct {
	Remediations []domain.GateRemediationInput `json:"remediations"`
}

type IssueInput struct {
	IssuerID string `json:"issuerID"`
}
