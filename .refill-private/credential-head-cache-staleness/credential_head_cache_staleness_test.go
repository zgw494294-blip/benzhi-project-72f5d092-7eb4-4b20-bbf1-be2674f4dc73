package credential_head_cache_staleness_test

import (
	"testing"
	"time"

	"acoustic-annotation-release/internal/certificate"
	"acoustic-annotation-release/internal/domain"
)

func TestCredentialHeadCacheRefreshesAfterIssue(t *testing.T) {
	service := certificate.NewService()
	if result := service.Verify(nil); !result.Valid || result.CheckedCount != 0 {
		t.Fatalf("空凭据链应通过校验: %#v", result)
	}

	issuedAt := time.Date(2026, 8, 25, 9, 30, 0, 0, time.UTC)
	first, err := service.Issue(
		"batch-cache-a",
		"reviewer-a",
		&domain.DatasetManifest{BatchID: "batch-cache-a", Digest: "manifest-cache-a"},
		nil,
		issuedAt,
	)
	if err != nil {
		t.Fatalf("签发第一份凭据: %v", err)
	}
	second, err := service.Issue(
		"batch-cache-b",
		"reviewer-b",
		&domain.DatasetManifest{BatchID: "batch-cache-b", Digest: "manifest-cache-b"},
		[]domain.ReleaseCredential{first},
		issuedAt.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("签发第二份凭据: %v", err)
	}

	verification := certificate.NewService().Verify([]domain.ReleaseCredential{first, second})
	if second.Sequence != 2 || second.PreviousDigest != first.CredentialDigest || !verification.Valid {
		t.Fatalf("校验预热后签发未延续持久化凭据链: firstSequence=%d secondSequence=%d previousMatches=%t issues=%#v",
			first.Sequence, second.Sequence, second.PreviousDigest == first.CredentialDigest, verification.Issues)
	}
}
