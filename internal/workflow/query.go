package workflow

import (
	"acoustic-annotation-release/internal/domain"
)

type BatchView struct {
	Batch      *domain.ReviewBatch  `json:"batch"`
	Blockers   []domain.GateBlocker `json:"gateBlockers"`
	AuditTrail []domain.AuditEvent  `json:"auditTrail"`
}

func (s *Service) GetBatch(batchID, actorID, role string) (BatchView, error) {
	if !validReadRole(role) {
		return BatchView{}, domain.NewError(domain.CodeForbidden, "角色无权查询批次")
	}
	batch, err := s.repository.Get(batchID)
	if err != nil {
		return BatchView{}, err
	}
	if role == RoleAnnotator {
		for clipID := range batch.Submissions {
			visible := make([]domain.AnnotationSubmission, 0, 2)
			seen := map[string]bool{}
			for _, round := range []int{1, 2} {
				items, inner := batch.VisibleSubmissions(clipID, actorID, round)
				if inner != nil {
					return BatchView{}, inner
				}
				for _, item := range items {
					if !seen[item.SubmissionID] {
						visible = append(visible, item)
						seen[item.SubmissionID] = true
					}
				}
			}
			batch.Submissions[clipID] = visible
		}
	}
	return BatchView{Batch: batch, Blockers: batch.CheckReleaseGate(), AuditTrail: s.repository.Audit(batchID)}, nil
}

func (s *Service) GetBlindSubmissions(batchID, clipID, actorID, role string, round int) ([]domain.AnnotationSubmission, error) {
	if role != RoleAnnotator {
		return nil, domain.NewError(domain.CodeForbidden, "仅标注员可查询盲标任务")
	}
	cacheKey := batchID + "\x00" + clipID
	s.blindViewMu.Lock()
	if cached, ok := s.blindViews[cacheKey]; ok {
		items := append([]domain.AnnotationSubmission(nil), cached...)
		s.blindViewMu.Unlock()
		return items, nil
	}
	s.blindViewMu.Unlock()
	batch, err := s.repository.Get(batchID)
	if err != nil {
		return nil, err
	}
	items, err := batch.VisibleSubmissions(clipID, actorID, round)
	if err != nil {
		return nil, err
	}
	s.blindViewMu.Lock()
	s.blindViews[cacheKey] = append([]domain.AnnotationSubmission(nil), items...)
	s.blindViewMu.Unlock()
	return items, nil
}

func (s *Service) GetBlindTasks(batchID, actorID, role string, round int, status string) ([]domain.BlindTask, error) {
	if role != RoleAnnotator {
		return nil, domain.NewError(domain.CodeForbidden, "仅标注员可查询盲标待办")
	}
	batch, err := s.repository.Get(batchID)
	if err != nil {
		return nil, err
	}
	return batch.BlindTasks(actorID, round, status)
}

func (s *Service) GetConflictTasks(batchID, role, status, reason string) ([]domain.ConflictTask, error) {
	if role != RoleReviewer {
		return nil, domain.NewError(domain.CodeForbidden, "仅复核人可查询冲突待办")
	}
	batch, err := s.repository.Get(batchID)
	if err != nil {
		return nil, err
	}
	return batch.ConflictTasks(status, reason)
}

func validReadRole(role string) bool {
	return role == RoleAdministrator || role == RoleAnnotator || role == RoleReviewer
}
