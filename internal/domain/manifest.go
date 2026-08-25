package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"time"
)

func (b *ReviewBatch) Freeze(now time.Time) (*DatasetManifest, error) {
	if b.Status == StatusFrozen && b.Manifest != nil {
		return b.Manifest, nil
	}
	if blockers := b.CheckReleaseGate(); len(blockers) > 0 {
		return nil, NewError(CodeGateBlocked, "发布门禁存在 %d 个阻断项", len(blockers))
	}
	entries := make([]ManifestEntry, 0, len(b.Clips))
	for clipID := range b.Clips {
		entry, ok := b.EffectiveAnnotation(clipID)
		if ok {
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ClipID < entries[j].ClipID })
	manifest := &DatasetManifest{SchemaVersion: 1, BatchID: b.BatchID, CreatedAt: now.UTC(), Entries: entries}
	payload, _ := manifestDigestPayload(manifest)
	digest := sha256.Sum256(payload)
	manifest.Digest = hex.EncodeToString(digest[:])
	b.Manifest, b.Status = manifest, StatusFrozen
	b.touch(now)
	return manifest, nil
}

func (b *ReviewBatch) AttachCredential(credential ReleaseCredential, now time.Time) error {
	if b.Status == StatusReleased && b.Credential != nil && b.Credential.CredentialDigest == credential.CredentialDigest {
		return nil
	}
	if b.Status != StatusFrozen {
		return NewError(CodeInvalidState, "仅 frozen 批次可签发凭据")
	}
	if b.Manifest == nil || credential.ManifestDigest != b.Manifest.Digest || credential.BatchID != b.BatchID {
		return NewError(CodeInvalid, "凭据与冻结清单不匹配")
	}
	b.Credential, b.Status = &credential, StatusReleased
	b.touch(now)
	return nil
}
