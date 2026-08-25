package workflow

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"acoustic-annotation-release/internal/certificate"
	"acoustic-annotation-release/internal/domain"
	"acoustic-annotation-release/internal/persistence"
)

type Service struct {
	repository   *persistence.Repository
	certificates *certificate.Service
	now          func() time.Time
	issueMu      sync.Mutex
}

func NewService(repository *persistence.Repository, certificates *certificate.Service) *Service {
	return &Service{repository: repository, certificates: certificates, now: time.Now}
}

func (s *Service) CreateBatch(input CreateBatchInput, context WriteContext) (*domain.ReviewBatch, bool, error) {
	if err := context.Validate(RoleAdministrator); err != nil {
		return nil, false, err
	}
	if context.ExpectedVersion != 0 {
		return nil, false, domain.NewError(domain.CodeConflict, "创建批次的 expectedVersion 必须为 0")
	}
	batch, err := domain.NewBatch(input.BatchID, input.SurveySite, input.CaptureWindowStart, input.CaptureWindowEnd, input.AuthorizationStatement, s.now())
	if err != nil {
		return nil, false, err
	}
	result, err := s.repository.Create(batch, "batch.created", context.IdempotencyKey, context.ActorID, context.Role, s.now())
	if err != nil {
		return nil, false, err
	}
	return result.Batch, result.Replay, nil
}

func (s *Service) AddClip(batchID string, input AddClipInput, context WriteContext) (*domain.ReviewBatch, bool, error) {
	if err := context.Validate(RoleAdministrator); err != nil {
		return nil, false, err
	}
	clip := domain.RecordingClip{ClipID: input.ClipID, SourceName: input.SourceName, DurationMillis: input.DurationMillis, ContentDigest: input.ContentDigest, CaptureTimestamp: input.CaptureTimestamp, AuthorizationConfirmed: input.AuthorizationConfirmed, HumanVoiceDetected: input.HumanVoiceDetected, RedactionNote: input.RedactionNote}
	return s.update(batchID, context, "clip.registered", func(batch *domain.ReviewBatch) error { return batch.AddClip(clip, s.now()) })
}

func (s *Service) StartAnnotation(batchID string, context WriteContext) (*domain.ReviewBatch, bool, error) {
	if err := context.Validate(RoleAdministrator); err != nil {
		return nil, false, err
	}
	return s.update(batchID, context, "annotation.started", func(batch *domain.ReviewBatch) error { return batch.StartAnnotation(s.now()) })
}

func (s *Service) UpdatePrivacy(batchID, clipID string, input PrivacyInput, context WriteContext) (*domain.ReviewBatch, bool, error) {
	if err := context.Validate(RoleAdministrator, RoleReviewer); err != nil {
		return nil, false, err
	}
	return s.update(batchID, context, "clip.privacy_updated", func(batch *domain.ReviewBatch) error {
		return batch.UpdateClipPrivacy(clipID, input.AuthorizationConfirmed, input.HumanVoiceDetected, input.RedactionNote, s.now())
	})
}

func (s *Service) SubmitAnnotation(batchID string, input AnnotationInput, context WriteContext) (*domain.ReviewBatch, *domain.ConflictCase, bool, error) {
	if err := context.Validate(RoleAnnotator); err != nil {
		return nil, nil, false, err
	}
	submission := domain.AnnotationSubmission{SubmissionID: newID("submission"), ClipID: input.ClipID, Round: input.Round, AnnotatorID: context.ActorID, SpeciesLabel: input.SpeciesLabel, StartMillis: input.StartMillis, EndMillis: input.EndMillis, Confidence: input.Confidence, EvidenceNote: input.EvidenceNote, Revision: input.Revision}
	var conflict *domain.ConflictCase
	batch, replay, err := s.update(batchID, context, "annotation.submitted", func(batch *domain.ReviewBatch) error {
		var inner error
		conflict, inner = batch.SubmitAnnotation(submission, s.now())
		return inner
	})
	if err != nil {
		return nil, nil, false, err
	}
	if replay {
		conflict = conflictForClip(batch, input.ClipID)
	}
	return batch, conflict, replay, nil
}

func (s *Service) ResolveConflict(batchID, conflictID string, input ResolveConflictInput, context WriteContext) (*domain.ReviewBatch, bool, error) {
	if err := context.Validate(RoleReviewer); err != nil {
		return nil, false, err
	}
	return s.update(batchID, context, "conflict.resolved", func(batch *domain.ReviewBatch) error {
		return batch.ResolveConflict(conflictID, input.Decision, input.ResolvedLabel, context.ActorID, input.ResolutionNote, s.now())
	})
}

func (s *Service) update(batchID string, context WriteContext, operation string, mutate func(*domain.ReviewBatch) error) (*domain.ReviewBatch, bool, error) {
	result, err := s.repository.Update(batchID, context.ExpectedVersion, operation, context.IdempotencyKey, context.ActorID, context.Role, s.now(), mutate)
	if err != nil {
		return nil, false, err
	}
	return result.Batch, result.Replay, nil
}

func newID(prefix string) string {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return prefix + "-fallback-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	return prefix + "-" + hex.EncodeToString(bytes)
}

func conflictForClip(batch *domain.ReviewBatch, clipID string) *domain.ConflictCase {
	if conflict := batch.Conflicts["conflict-"+clipID]; conflict != nil {
		copy := *conflict
		return &copy
	}
	return nil
}
