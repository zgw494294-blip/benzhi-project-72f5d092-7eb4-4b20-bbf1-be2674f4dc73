package certificate

import (
	"testing"
	"time"

	"acoustic-annotation-release/internal/domain"
)

func TestIssueAndVerifyCredentialChain(t *testing.T) {
	service := NewService()
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	first, err := service.Issue("batch-1", "reviewer-1", &domain.DatasetManifest{BatchID: "batch-1", Digest: "manifest-1"}, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Issue("batch-2", "reviewer-2", &domain.DatasetManifest{BatchID: "batch-2", Digest: "manifest-2"}, []domain.ReleaseCredential{first}, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	result := service.Verify([]domain.ReleaseCredential{second, first})
	if !result.Valid || result.CheckedCount != 2 || result.HeadDigest != second.CredentialDigest {
		t.Fatalf("合法凭据链校验失败: %#v", result)
	}
	second.ManifestDigest = "tampered"
	result = service.Verify([]domain.ReleaseCredential{first, second})
	if result.Valid || len(result.Issues) == 0 {
		t.Fatalf("篡改未被发现: %#v", result)
	}
}
