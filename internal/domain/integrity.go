package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

func (b *ReviewBatch) ValidateIntegrity() error {
	if b == nil || b.BatchID == "" || b.Version < 1 {
		return fmt.Errorf("批次身份或版本无效")
	}
	if !b.CaptureWindowEnd.After(b.CaptureWindowStart) {
		return fmt.Errorf("批次采集时间窗无效")
	}
	if b.Clips == nil || b.Submissions == nil || b.Conflicts == nil {
		return fmt.Errorf("批次集合未初始化")
	}
	validStatus := map[BatchStatus]bool{StatusDraft: true, StatusAnnotating: true, StatusAdjudicating: true, StatusReadyForReview: true, StatusFrozen: true, StatusReleased: true}
	if !validStatus[b.Status] {
		return fmt.Errorf("未知批次状态 %q", b.Status)
	}
	digests := map[string]string{}
	for clipID, clip := range b.Clips {
		if clip == nil || clipID == "" || clip.ClipID != clipID || clip.BatchID != b.BatchID {
			return fmt.Errorf("录音 %q 的引用关系无效", clipID)
		}
		if !validIdentifier(clipID) || clip.SourceName == "" || clip.DurationMillis <= 0 || clip.ContentDigest == "" {
			return fmt.Errorf("录音 %s 的时长或摘要无效", clipID)
		}
		if clip.CaptureTimestamp.Before(b.CaptureWindowStart) || clip.CaptureTimestamp.After(b.CaptureWindowEnd) {
			return fmt.Errorf("录音 %s 的采集时间不在批次时间窗内", clipID)
		}
		digest := NormalizeContentDigest(clip.ContentDigest)
		if existing, ok := digests[digest]; ok {
			return fmt.Errorf("录音 %s 与 %s 的内容摘要重复", clipID, existing)
		}
		digests[digest] = clipID
	}
	for clipID, submissions := range b.Submissions {
		clip := b.Clips[clipID]
		if clip == nil {
			return fmt.Errorf("标注引用不存在的录音 %s", clipID)
		}
		seen := map[string]bool{}
		owners := map[int]string{}
		for _, submission := range submissions {
			key := fmt.Sprintf("%d:%d", submission.Round, submission.Revision)
			if seen[key] {
				return fmt.Errorf("录音 %s 存在重复轮次修订 %s", clipID, key)
			}
			seen[key] = true
			if submission.ClipID != clipID || submission.SubmissionID == "" || submission.AnnotatorID == "" {
				return fmt.Errorf("录音 %s 的标注身份无效", clipID)
			}
			if submission.Round < 1 || submission.Round > 2 || submission.Revision < 1 {
				return fmt.Errorf("录音 %s 的标注轮次或修订无效", clipID)
			}
			if owner := owners[submission.Round]; owner != "" && owner != submission.AnnotatorID {
				return fmt.Errorf("录音 %s 同一轮存在多个标注员", clipID)
			}
			owners[submission.Round] = submission.AnnotatorID
			if submission.StartMillis < 0 || submission.EndMillis <= submission.StartMillis || submission.EndMillis > clip.DurationMillis {
				return fmt.Errorf("录音 %s 的标注区间无效", clipID)
			}
		}
	}
	for conflictID, conflict := range b.Conflicts {
		if conflict == nil || conflict.ConflictID != conflictID || b.Clips[conflict.ClipID] == nil {
			return fmt.Errorf("冲突 %s 的引用关系无效", conflictID)
		}
		if conflict.Status == ConflictResolved && (conflict.ResolvedAt == nil || conflict.ResolvedLabel == "") {
			return fmt.Errorf("冲突 %s 的关闭事实不完整", conflictID)
		}
		if !ValidConflictStatus(string(conflict.Status)) || !ValidConflictReason(conflict.ReasonCode) {
			return fmt.Errorf("冲突 %s 的状态或原因无效", conflictID)
		}
		if conflict.ReannotationRound < 0 || conflict.ReannotationRound > 2 {
			return fmt.Errorf("冲突 %s 的重标轮次无效", conflictID)
		}
	}
	if b.Status == StatusFrozen || b.Status == StatusReleased {
		if b.Manifest == nil {
			return fmt.Errorf("冻结或发布批次缺少清单")
		}
		if err := VerifyManifest(b.Manifest); err != nil {
			return err
		}
	}
	if b.Status == StatusReleased {
		if b.Credential == nil || b.Credential.BatchID != b.BatchID || b.Credential.ManifestDigest != b.Manifest.Digest {
			return fmt.Errorf("已发布批次的凭据引用无效")
		}
	}
	return nil
}

func VerifyManifest(manifest *DatasetManifest) error {
	if manifest == nil || manifest.SchemaVersion != 1 || manifest.BatchID == "" {
		return fmt.Errorf("清单字段不完整")
	}
	for index, entry := range manifest.Entries {
		if entry.ClipID == "" || entry.ContentDigest == "" || entry.SpeciesLabel == "" || entry.EndMillis <= entry.StartMillis {
			return fmt.Errorf("清单第 %d 项无效", index)
		}
		if index > 0 && manifest.Entries[index-1].ClipID >= entry.ClipID {
			return fmt.Errorf("清单条目未按 clipID 严格排序")
		}
	}
	payload, err := manifestDigestPayload(manifest)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(payload)
	if manifest.Digest != hex.EncodeToString(digest[:]) {
		return fmt.Errorf("清单摘要不一致")
	}
	return nil
}

func manifestDigestPayload(manifest *DatasetManifest) ([]byte, error) {
	return json.Marshal(struct {
		SchemaVersion int             `json:"schemaVersion"`
		BatchID       string          `json:"batchID"`
		Entries       []ManifestEntry `json:"entries"`
	}{manifest.SchemaVersion, manifest.BatchID, manifest.Entries})
}
