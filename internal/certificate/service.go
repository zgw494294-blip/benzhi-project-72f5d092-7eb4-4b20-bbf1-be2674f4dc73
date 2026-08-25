package certificate

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"acoustic-annotation-release/internal/domain"
)

type Service struct{}

func NewService() *Service { return &Service{} }

func (s *Service) Issue(batchID, issuerID string, manifest *domain.DatasetManifest, existing []domain.ReleaseCredential, now time.Time) (domain.ReleaseCredential, error) {
	batchID, issuerID = strings.TrimSpace(batchID), strings.TrimSpace(issuerID)
	if batchID == "" || issuerID == "" || manifest == nil || manifest.Digest == "" {
		return domain.ReleaseCredential{}, domain.NewError(domain.CodeInvalid, "签发参数或清单摘要为空")
	}
	if manifest.BatchID != batchID {
		return domain.ReleaseCredential{}, domain.NewError(domain.CodeInvalid, "清单批次不匹配")
	}
	sort.Slice(existing, func(i, j int) bool { return existing[i].Sequence < existing[j].Sequence })
	sequence, previous := int64(1), ""
	if len(existing) > 0 {
		sequence = existing[len(existing)-1].Sequence + 1
		previous = existing[len(existing)-1].CredentialDigest
	}
	credential := domain.ReleaseCredential{BatchID: batchID, Sequence: sequence, ManifestDigest: manifest.Digest, PreviousDigest: previous, IssuerID: issuerID, IssuedAt: now.UTC()}
	credential.CredentialDigest = digestCredential(credential)
	credential.CredentialID = fmt.Sprintf("credential-%06d-%s", sequence, credential.CredentialDigest[:12])
	return credential, nil
}
