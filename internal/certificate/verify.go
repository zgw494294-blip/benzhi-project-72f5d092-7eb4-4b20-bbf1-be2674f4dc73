package certificate

import (
	"fmt"
	"sort"
	"strings"

	"acoustic-annotation-release/internal/domain"
)

type VerificationIssue struct {
	Sequence int64  `json:"sequence"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

type VerificationResult struct {
	Valid        bool                `json:"valid"`
	CheckedCount int                 `json:"checkedCount"`
	HeadDigest   string              `json:"headDigest,omitempty"`
	Issues       []VerificationIssue `json:"issues"`
}

func (s *Service) Verify(chain []domain.ReleaseCredential) VerificationResult {
	items := append([]domain.ReleaseCredential(nil), chain...)
	sort.Slice(items, func(i, j int) bool { return items[i].Sequence < items[j].Sequence })
	result := VerificationResult{Valid: true, CheckedCount: len(items), Issues: []VerificationIssue{}}
	previous := ""
	for index, credential := range items {
		expectedSequence := int64(index + 1)
		if credential.Sequence != expectedSequence {
			result.add(credential.Sequence, "sequence_gap", fmt.Sprintf("期望序号 %d", expectedSequence))
		}
		if credential.BatchID == "" || credential.ManifestDigest == "" || credential.IssuerID == "" || credential.IssuedAt.IsZero() {
			result.add(credential.Sequence, "missing_field", "凭据必填字段不完整")
		}
		if credential.PreviousDigest != previous {
			result.add(credential.Sequence, "previous_digest_mismatch", "前序摘要不一致")
		}
		if credential.CredentialDigest != digestCredential(credential) {
			result.add(credential.Sequence, "credential_digest_mismatch", "当前凭据摘要不一致")
		}
		if !strings.HasPrefix(credential.CredentialID, fmt.Sprintf("credential-%06d-", credential.Sequence)) {
			result.add(credential.Sequence, "credential_id_invalid", "凭据标识与序号不一致")
		}
		previous = credential.CredentialDigest
	}
	result.HeadDigest = previous
	return result
}

func (r *VerificationResult) add(sequence int64, code, message string) {
	r.Valid = false
	r.Issues = append(r.Issues, VerificationIssue{Sequence: sequence, Code: code, Message: message})
}
