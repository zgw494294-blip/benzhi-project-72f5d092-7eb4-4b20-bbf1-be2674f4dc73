package domain

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const minimumConfidence = 0.60

func NormalizeLabel(label string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(label)), " "))
}

func IntervalOverlap(aStart, aEnd, bStart, bEnd int64) float64 {
	intersection := min64(aEnd, bEnd) - max64(aStart, bStart)
	if intersection <= 0 {
		return 0
	}
	union := max64(aEnd, bEnd) - min64(aStart, bStart)
	if union <= 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func (b *ReviewBatch) SubmitAnnotation(input AnnotationSubmission, now time.Time) (*ConflictCase, error) {
	if err := b.EnsureMutable(); err != nil {
		return nil, err
	}
	if b.Status != StatusAnnotating && b.Status != StatusAdjudicating {
		return nil, NewError(CodeInvalidState, "当前状态不可提交标注")
	}
	clip, ok := b.Clips[input.ClipID]
	if !ok {
		return nil, NewError(CodeNotFound, "录音 %s 不存在", input.ClipID)
	}
	input.SpeciesLabel = NormalizeLabel(input.SpeciesLabel)
	input.EvidenceNote, input.AnnotatorID = strings.TrimSpace(input.EvidenceNote), strings.TrimSpace(input.AnnotatorID)
	if input.Round != 1 && input.Round != 2 {
		return nil, NewError(CodeInvalid, "标注轮次只能是 1 或 2")
	}
	if input.AnnotatorID == "" || input.SpeciesLabel == "" || input.EvidenceNote == "" {
		return nil, NewError(CodeInvalid, "标注者、物种标签和证据说明不能为空")
	}
	if input.StartMillis < 0 || input.EndMillis <= input.StartMillis || input.EndMillis > clip.DurationMillis {
		return nil, NewError(CodeInvalid, "标注区间超出录音范围")
	}
	if math.IsNaN(input.Confidence) || input.Confidence < 0 || input.Confidence > 1 {
		return nil, NewError(CodeInvalid, "置信度必须在 0 到 1 之间")
	}
	if input.Revision <= 0 {
		input.Revision = 1
	}
	existing := b.Submissions[input.ClipID]
	latestRevision := 0
	for _, submission := range existing {
		if submission.Round != input.Round && submission.AnnotatorID == input.AnnotatorID {
			return nil, NewError(CodeForbidden, "同一标注员不能承担同一录音的两轮盲标")
		}
		if submission.Round == input.Round {
			if submission.AnnotatorID != input.AnnotatorID {
				return nil, NewError(CodeForbidden, "该轮盲标已由其他标注员承担")
			}
			if submission.Revision > latestRevision {
				latestRevision = submission.Revision
			}
		}
	}
	if input.Revision != latestRevision+1 {
		return nil, NewError(CodeInvalid, "修订号必须为下一修订号 %d", latestRevision+1)
	}
	input.SubmittedAt = now.UTC()
	b.Submissions[input.ClipID] = append(existing, input)
	conflict := b.evaluateClip(input.ClipID, input.Round, now)
	b.recalculateStatus()
	b.touch(now)
	return conflict, nil
}

func (b *ReviewBatch) VisibleSubmissions(clipID, annotatorID string, round int) ([]AnnotationSubmission, error) {
	if _, ok := b.Clips[clipID]; !ok {
		return nil, NewError(CodeNotFound, "录音不存在")
	}
	items := latestRounds(b.Submissions[clipID])
	if len(items) < 2 {
		visible := make([]AnnotationSubmission, 0, 1)
		for _, item := range items {
			if item.Round == round && item.AnnotatorID == annotatorID {
				visible = append(visible, item)
			}
		}
		return visible, nil
	}
	return items, nil
}

func (b *ReviewBatch) evaluateClip(clipID string, submittedRound int, now time.Time) *ConflictCase {
	latest := latestRounds(b.Submissions[clipID])
	if len(latest) != 2 {
		return nil
	}
	a, c := latest[0], latest[1]
	overlap := IntervalOverlap(a.StartMillis, a.EndMillis, c.StartMillis, c.EndMillis)
	reason := ""
	if NormalizeLabel(a.SpeciesLabel) != NormalizeLabel(c.SpeciesLabel) {
		reason = "label_mismatch"
	} else if overlap < 0.80 {
		reason = "interval_divergence"
	} else if a.Confidence < minimumConfidence || c.Confidence < minimumConfidence {
		reason = "low_confidence"
	}
	conflictID := "conflict-" + clipID
	current := b.Conflicts[conflictID]
	if current != nil && current.Status == ConflictReannotate {
		current.OverlapRatio = overlap
		if reason != "" {
			current.ReasonCode = reason
		}
		if current.ReannotationRound == 0 || current.ReannotationRound == submittedRound {
			current.Status = ConflictReview
		}
		return current
	}
	if current != nil && current.Status == ConflictReview {
		current.OverlapRatio = overlap
		if reason != "" {
			current.ReasonCode = reason
		}
		return current
	}
	if reason == "" {
		delete(b.Conflicts, conflictID)
		return nil
	}
	if current != nil && current.Status == ConflictResolved {
		return current
	}
	current = &ConflictCase{ConflictID: conflictID, ClipID: clipID, ReasonCode: reason, OverlapRatio: overlap, Status: ConflictOpen}
	b.Conflicts[conflictID] = current
	return current
}

func latestRounds(all []AnnotationSubmission) []AnnotationSubmission {
	byRound := map[int]AnnotationSubmission{}
	for _, item := range all {
		if previous, ok := byRound[item.Round]; !ok || item.Revision > previous.Revision {
			byRound[item.Round] = item
		}
	}
	result := make([]AnnotationSubmission, 0, 2)
	for _, round := range []int{1, 2} {
		if item, ok := byRound[round]; ok {
			result = append(result, item)
		}
	}
	return result
}

func (b *ReviewBatch) ResolveConflict(conflictID, decision, label, reviewer, note string, now time.Time) error {
	if err := b.EnsureMutable(); err != nil {
		return err
	}
	if b.Status != StatusAdjudicating {
		return NewError(CodeInvalidState, "仅 adjudicating 状态可裁决")
	}
	prepared, err := b.validateConflictDecision(conflictID, decision, label, reviewer, note, 0)
	if err != nil {
		return err
	}
	b.applyConflictDecision(prepared, now)
	b.recalculateStatus()
	b.touch(now)
	return nil
}

func (b *ReviewBatch) recalculateStatus() {
	complete := len(b.Clips) > 0
	for clipID := range b.Clips {
		if len(latestRounds(b.Submissions[clipID])) != 2 {
			complete = false
		}
	}
	if !complete {
		b.Status = StatusAnnotating
		return
	}
	for _, conflict := range b.Conflicts {
		if conflict.Status != ConflictResolved {
			b.Status = StatusAdjudicating
			return
		}
	}
	b.Status = StatusReadyForReview
}

func (b *ReviewBatch) EffectiveAnnotation(clipID string) (ManifestEntry, bool) {
	latest := latestRounds(b.Submissions[clipID])
	if len(latest) != 2 {
		return ManifestEntry{}, false
	}
	entry := ManifestEntry{ClipID: clipID, ContentDigest: b.Clips[clipID].ContentDigest, SpeciesLabel: latest[0].SpeciesLabel, StartMillis: min64(latest[0].StartMillis, latest[1].StartMillis), EndMillis: max64(latest[0].EndMillis, latest[1].EndMillis), RedactionNote: b.Clips[clipID].RedactionNote}
	if conflict := b.Conflicts["conflict-"+clipID]; conflict != nil {
		if conflict.Status != ConflictResolved {
			return ManifestEntry{}, false
		}
		entry.SpeciesLabel = conflict.ResolvedLabel
		if conflict.Decision == "accept_round_2" {
			entry.StartMillis, entry.EndMillis = latest[1].StartMillis, latest[1].EndMillis
		}
		if conflict.Decision == "accept_round_1" {
			entry.StartMillis, entry.EndMillis = latest[0].StartMillis, latest[0].EndMillis
		}
	}
	return entry, true
}

func (b *ReviewBatch) SortedConflictIDs() []string {
	ids := make([]string, 0, len(b.Conflicts))
	for id := range b.Conflicts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
func conflictSummary(c *ConflictCase) string { return fmt.Sprintf("%s:%s", c.ClipID, c.ReasonCode) }
