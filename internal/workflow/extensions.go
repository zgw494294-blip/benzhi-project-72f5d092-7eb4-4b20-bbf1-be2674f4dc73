package workflow

import (
	"encoding/json"
	"sort"
	"strings"

	"acoustic-annotation-release/internal/domain"
	"acoustic-annotation-release/internal/persistence"
)

type BulkClipResult struct {
	Batch            *domain.ReviewBatch    `json:"batch"`
	Clips            []domain.RecordingClip `json:"clips"`
	IdempotentReplay bool                   `json:"idempotentReplay"`
}

func (s *Service) AddClips(batchID string, input BulkAddClipsInput, context WriteContext) (BulkClipResult, error) {
	if err := context.Validate(RoleAdministrator); err != nil {
		return BulkClipResult{}, err
	}
	clips := make([]domain.RecordingClip, len(input.Clips))
	for index, item := range input.Clips {
		clips[index] = domain.RecordingClip{ClipID: item.ClipID, SourceName: item.SourceName, DurationMillis: item.DurationMillis, ContentDigest: item.ContentDigest, CaptureTimestamp: item.CaptureTimestamp, AuthorizationConfirmed: item.AuthorizationConfirmed, HumanVoiceDetected: item.HumanVoiceDetected, RedactionNote: item.RedactionNote}
	}
	result, err := s.repository.UpdateAtomicWithContext(context.RequestContext, batchID, context.ExpectedVersion, "clips.bulk_registered", context.IdempotencyKey, context.ActorID, context.Role, s.now(), func(batch *domain.ReviewBatch) (persistence.MutationMetadata, error) {
		registered, inner := batch.RegisterClips(clips, s.now())
		if inner != nil {
			return persistence.MutationMetadata{}, inner
		}
		clipIDs := make([]string, len(registered))
		for index := range registered {
			clipIDs[index] = registered[index].ClipID
		}
		return persistence.MutationMetadata{Result: registered, AuditDetails: map[string]any{"count": len(registered), "clipIDs": clipIDs}}, nil
	})
	if err != nil {
		return BulkClipResult{}, err
	}
	var registered []domain.RecordingClip
	if err = json.Unmarshal(result.OperationResult, &registered); err != nil {
		return BulkClipResult{}, err
	}
	return BulkClipResult{Batch: result.Batch, Clips: registered, IdempotentReplay: result.Replay}, nil
}

type BulkAdjudicationResult struct {
	Batch            *domain.ReviewBatch             `json:"batch"`
	Decisions        []domain.ConflictDecisionResult `json:"decisions"`
	IdempotentReplay bool                            `json:"idempotentReplay"`
}

func (s *Service) AdjudicateConflicts(batchID string, input BulkConflictDecisionsInput, context WriteContext) (BulkAdjudicationResult, error) {
	if err := context.Validate(RoleReviewer); err != nil {
		return BulkAdjudicationResult{}, err
	}
	result, err := s.repository.UpdateAtomicWithContext(context.RequestContext, batchID, context.ExpectedVersion, "conflicts.bulk_adjudicated", context.IdempotencyKey, context.ActorID, context.Role, s.now(), func(batch *domain.ReviewBatch) (persistence.MutationMetadata, error) {
		decisions, inner := batch.AdjudicateConflicts(input.Decisions, context.ActorID, s.now())
		if inner != nil {
			return persistence.MutationMetadata{}, inner
		}
		summaries := make([]map[string]any, len(decisions))
		for index, item := range decisions {
			summaries[index] = map[string]any{"conflictID": item.ConflictID, "clipID": item.ClipID, "decision": item.Decision, "status": item.Status}
		}
		return persistence.MutationMetadata{Result: decisions, AuditDetails: map[string]any{"count": len(decisions), "decisions": summaries}}, nil
	})
	if err != nil {
		return BulkAdjudicationResult{}, err
	}
	var decisions []domain.ConflictDecisionResult
	if err = json.Unmarshal(result.OperationResult, &decisions); err != nil {
		return BulkAdjudicationResult{}, err
	}
	return BulkAdjudicationResult{Batch: result.Batch, Decisions: decisions, IdempotentReplay: result.Replay}, nil
}

type GateRemediationResult struct {
	Batch             *domain.ReviewBatch  `json:"batch"`
	Passed            bool                 `json:"passed"`
	Status            domain.BatchStatus   `json:"status"`
	ResolvedBlockers  []domain.GateBlocker `json:"resolvedBlockers"`
	RemainingBlockers []domain.GateBlocker `json:"remainingBlockers"`
	IdempotentReplay  bool                 `json:"idempotentReplay"`
}

func (s *Service) RemediateGate(batchID string, input GateRemediationsInput, context WriteContext) (GateRemediationResult, error) {
	if err := context.Validate(RoleAdministrator, RoleReviewer); err != nil {
		return GateRemediationResult{}, err
	}
	result, err := s.repository.UpdateAtomicWithContext(context.RequestContext, batchID, context.ExpectedVersion, "release_gate.bulk_remediated", context.IdempotencyKey, context.ActorID, context.Role, s.now(), func(batch *domain.ReviewBatch) (persistence.MutationMetadata, error) {
		before := batch.CheckReleaseGate()
		outcome, inner := batch.RemediateGate(input.Remediations, s.now())
		if inner != nil {
			return persistence.MutationMetadata{}, inner
		}
		codesByClip := make(map[string][]string)
		for _, blocker := range before {
			if blocker.Code == "authorization_missing" || blocker.Code == "redaction_missing" {
				codesByClip[blocker.ClipID] = append(codesByClip[blocker.ClipID], blocker.Code)
			}
		}
		clipIDs := make([]string, 0, len(input.Remediations))
		for _, item := range input.Remediations {
			clipIDs = append(clipIDs, strings.TrimSpace(item.ClipID))
		}
		sort.Strings(clipIDs)
		auditItems := make([]map[string]any, 0, len(clipIDs))
		for _, clipID := range clipIDs {
			auditItems = append(auditItems, map[string]any{"clipID": clipID, "reasonCodes": codesByClip[clipID]})
		}
		return persistence.MutationMetadata{Result: outcome, AuditDetails: map[string]any{"count": len(clipIDs), "remediations": auditItems}}, nil
	})
	if err != nil {
		return GateRemediationResult{}, err
	}
	var outcome domain.GateRemediationOutcome
	if err = json.Unmarshal(result.OperationResult, &outcome); err != nil {
		return GateRemediationResult{}, err
	}
	return GateRemediationResult{Batch: result.Batch, Passed: outcome.Passed, Status: outcome.Status, ResolvedBlockers: outcome.ResolvedBlockers, RemainingBlockers: outcome.RemainingBlockers, IdempotentReplay: result.Replay}, nil
}
