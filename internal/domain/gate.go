package domain

import (
	"sort"
	"strings"
)

func (b *ReviewBatch) CheckReleaseGate() []GateBlocker {
	blockers := make([]GateBlocker, 0)
	if b.Status != StatusReadyForReview {
		blockers = append(blockers, GateBlocker{Code: "invalid_status", Message: "批次尚未完成标注与裁决"})
	}
	clipIDs := make([]string, 0, len(b.Clips))
	for id := range b.Clips {
		clipIDs = append(clipIDs, id)
	}
	sort.Strings(clipIDs)
	for _, id := range clipIDs {
		clip := b.Clips[id]
		if !clip.AuthorizationConfirmed {
			blockers = append(blockers, GateBlocker{Code: "authorization_missing", ClipID: id, Message: "录音未确认采集授权"})
		}
		if clip.HumanVoiceDetected && strings.TrimSpace(clip.RedactionNote) == "" {
			blockers = append(blockers, GateBlocker{Code: "redaction_missing", ClipID: id, Message: "敏感人声录音缺少静音说明"})
		}
		if _, ok := b.EffectiveAnnotation(id); !ok {
			blockers = append(blockers, GateBlocker{Code: "annotation_incomplete", ClipID: id, Message: "录音标注或裁决不完整"})
		}
	}
	for _, id := range b.SortedConflictIDs() {
		if b.Conflicts[id].Status != ConflictResolved {
			blockers = append(blockers, GateBlocker{Code: "conflict_open", ClipID: b.Conflicts[id].ClipID, Message: "仍有未关闭冲突"})
		}
	}
	return blockers
}
