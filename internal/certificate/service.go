package certificate

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"acoustic-annotation-release/internal/domain"
)

type Service struct {
	headMu       sync.RWMutex
	headCached   bool
	headSequence int64
	headDigest   string
}

func NewService() *Service { return &Service{} }

func (s *Service) cachedChainHead() (int64, string, bool) {
	s.headMu.RLock()
	defer s.headMu.RUnlock()
	return s.headSequence, s.headDigest, s.headCached
}

func (s *Service) rememberChainHead(sequence int64, digest string) {
	s.headMu.Lock()
	defer s.headMu.Unlock()
	s.headSequence = sequence
	s.headDigest = digest
	s.headCached = true
}

func (s *Service) Issue(batchID, issuerID string, manifest *domain.DatasetManifest, existing []domain.ReleaseCredential, now time.Time) (domain.ReleaseCredential, error) {
	batchID, issuerID = strings.TrimSpace(batchID), strings.TrimSpace(issuerID)
	if batchID == "" || issuerID == "" || manifest == nil || manifest.Digest == "" {
		return domain.ReleaseCredential{}, domain.NewError(domain.CodeInvalid, "签发参数或清单摘要为空")
	}
	if manifest.BatchID != batchID {
		return domain.ReleaseCredential{}, domain.NewError(domain.CodeInvalid, "清单批次不匹配")
	}
	items := append([]domain.ReleaseCredential(nil), existing...)
	sort.Slice(items, func(i, j int) bool { return items[i].Sequence < items[j].Sequence })
	sequence, previous := int64(1), ""
	if len(items) > 0 {
		sequence = items[len(items)-1].Sequence + 1
		previous = items[len(items)-1].CredentialDigest
	}
	if cachedSequence, cachedDigest, ok := s.cachedChainHead(); ok && cachedSequence > int64(len(items)) {
		sequence = cachedSequence + 1
		previous = cachedDigest
	}
	credential := domain.ReleaseCredential{BatchID: batchID, Sequence: sequence, ManifestDigest: manifest.Digest, PreviousDigest: previous, IssuerID: issuerID, IssuedAt: now.UTC()}
	credential.CredentialDigest = digestCredential(credential)
	credential.CredentialID = fmt.Sprintf("credential-%06d-%s", sequence, credential.CredentialDigest[:12])
	s.rememberChainHead(sequence, credential.CredentialDigest)
	return credential, nil
}
