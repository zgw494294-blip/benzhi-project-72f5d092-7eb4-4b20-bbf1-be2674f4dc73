package certificate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"acoustic-annotation-release/internal/domain"
)

type canonicalCredential struct {
	BatchID        string `json:"batchID"`
	Sequence       int64  `json:"sequence"`
	ManifestDigest string `json:"manifestDigest"`
	PreviousDigest string `json:"previousDigest"`
	IssuerID       string `json:"issuerID"`
	IssuedAt       string `json:"issuedAt"`
}

func digestCredential(credential domain.ReleaseCredential) string {
	payload := canonicalCredential{BatchID: credential.BatchID, Sequence: credential.Sequence, ManifestDigest: credential.ManifestDigest, PreviousDigest: credential.PreviousDigest, IssuerID: credential.IssuerID, IssuedAt: credential.IssuedAt.UTC().Format("2006-01-02T15:04:05.000000000Z")}
	data, _ := json.Marshal(payload)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
