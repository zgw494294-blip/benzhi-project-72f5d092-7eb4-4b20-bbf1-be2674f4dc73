package workflow

import (
	"acoustic-annotation-release/internal/certificate"
	"acoustic-annotation-release/internal/domain"
)

type GateResult struct {
	Passed   bool                 `json:"passed"`
	Status   domain.BatchStatus   `json:"status"`
	Blockers []domain.GateBlocker `json:"blockers"`
}

func (s *Service) CheckGate(batchID, role string) (GateResult, error) {
	if role != RoleReviewer && role != RoleAdministrator {
		return GateResult{}, domain.NewError(domain.CodeForbidden, "仅管理员或复核人可检查门禁")
	}
	s.gateMu.RLock()
	cached, ok := s.gateCache[batchID]
	s.gateMu.RUnlock()
	if ok {
		return cloneGateResult(cached), nil
	}
	batch, err := s.repository.Get(batchID)
	if err != nil {
		return GateResult{}, err
	}
	blockers := batch.CheckReleaseGate()
	result := GateResult{Passed: len(blockers) == 0, Status: batch.Status, Blockers: blockers}
	s.gateMu.Lock()
	s.gateCache[batchID] = cloneGateResult(result)
	s.gateMu.Unlock()
	return result, nil
}

func cloneGateResult(result GateResult) GateResult {
	result.Blockers = append([]domain.GateBlocker(nil), result.Blockers...)
	return result
}

func (s *Service) Freeze(batchID string, context WriteContext) (*domain.ReviewBatch, bool, error) {
	if err := context.Validate(RoleReviewer); err != nil {
		return nil, false, err
	}
	return s.update(batchID, context, "batch.frozen", func(batch *domain.ReviewBatch) error { _, err := batch.Freeze(s.now()); return err })
}

func (s *Service) Issue(batchID string, input IssueInput, context WriteContext) (*domain.ReviewBatch, bool, error) {
	if err := context.Validate(RoleReviewer); err != nil {
		return nil, false, err
	}
	if input.IssuerID == "" || input.IssuerID != context.ActorID {
		return nil, false, domain.NewError(domain.CodeForbidden, "issuerID 必须与操作者一致")
	}
	s.issueMu.Lock()
	defer s.issueMu.Unlock()
	credentials := s.repository.Credentials()
	return s.update(batchID, context, "credential.issued", func(batch *domain.ReviewBatch) error {
		if batch.Status == domain.StatusReleased && batch.Credential != nil {
			return nil
		}
		credential, err := s.certificates.Issue(batch.BatchID, input.IssuerID, batch.Manifest, credentials, s.now())
		if err != nil {
			return err
		}
		return batch.AttachCredential(credential, s.now())
	})
}

func (s *Service) VerifyCredentials() certificate.VerificationResult {
	return s.certificates.Verify(s.repository.Credentials())
}
