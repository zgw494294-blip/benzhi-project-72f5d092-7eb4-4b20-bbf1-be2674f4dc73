package domain

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

const MaxBulkItems = 100

func NormalizeContentDigest(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), ""))
}

func (b *ReviewBatch) RegisterClips(inputs []RecordingClip, now time.Time) ([]RecordingClip, error) {
	if err := b.EnsureMutable(); err != nil {
		return nil, err
	}
	if b.Status != StatusDraft {
		return nil, NewError(CodeInvalidState, "仅 draft 批次可登记录音")
	}
	if len(inputs) == 0 || len(inputs) > MaxBulkItems {
		return nil, NewError(CodeInvalid, "录音数组数量必须在 1 到 %d 之间", MaxBulkItems)
	}

	existingDigests := make(map[string]string, len(b.Clips))
	for clipID, clip := range b.Clips {
		existingDigests[NormalizeContentDigest(clip.ContentDigest)] = clipID
	}
	seenIDs := make(map[string]int, len(inputs))
	seenDigests := make(map[string]int, len(inputs))
	normalized := make([]RecordingClip, len(inputs))
	issues := make([]ItemError, 0)
	for index, input := range inputs {
		input.ClipID = strings.TrimSpace(input.ClipID)
		input.SourceName = strings.TrimSpace(input.SourceName)
		input.ContentDigest = NormalizeContentDigest(input.ContentDigest)
		input.RedactionNote = strings.TrimSpace(input.RedactionNote)
		input.BatchID = b.BatchID
		normalized[index] = input

		addIssue := func(field, reason, message string) {
			issues = append(issues, ItemError{Index: index, ClipID: input.ClipID, Field: field, ReasonCode: reason, Message: message})
		}
		if !validIdentifier(input.ClipID) {
			addIssue("clipID", "invalid_clip_id", "录音标识无效")
		} else if _, exists := b.Clips[input.ClipID]; exists {
			addIssue("clipID", "duplicate_clip_id", "批次内已存在相同录音标识")
		} else if previous, exists := seenIDs[input.ClipID]; exists {
			addIssue("clipID", "duplicate_clip_id", "与请求第 "+strconv.Itoa(previous)+" 项录音标识重复")
		} else {
			seenIDs[input.ClipID] = index
		}
		if input.SourceName == "" || len(input.SourceName) > 512 {
			addIssue("sourceName", "invalid_source_name", "来源名称不能为空且不能超过 512 个字符")
		}
		if input.DurationMillis <= 0 {
			addIssue("durationMillis", "invalid_duration", "录音时长必须为正数")
		}
		if input.CaptureTimestamp.IsZero() || input.CaptureTimestamp.Before(b.CaptureWindowStart) || input.CaptureTimestamp.After(b.CaptureWindowEnd) {
			addIssue("captureTimestamp", "outside_capture_window", "录音采集时间不在批次时间窗内")
		}
		if input.ContentDigest == "" || len(input.ContentDigest) > 512 {
			addIssue("contentDigest", "invalid_content_digest", "内容摘要不能为空且不能超过 512 个字符")
		} else if existingID, exists := existingDigests[input.ContentDigest]; exists {
			addIssue("contentDigest", "duplicate_content_digest", "与批次已有录音 "+existingID+" 的内容摘要重复")
		} else if previous, exists := seenDigests[input.ContentDigest]; exists {
			addIssue("contentDigest", "duplicate_content_digest", "与请求第 "+strconv.Itoa(previous)+" 项内容摘要重复")
		} else {
			seenDigests[input.ContentDigest] = index
		}
	}
	if len(issues) > 0 {
		return nil, NewDetailedError(CodeInvalid, map[string]any{"items": issues}, "批量录音登记预检失败")
	}
	for index := range normalized {
		clip := normalized[index]
		b.Clips[clip.ClipID] = &clip
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].ClipID < normalized[j].ClipID })
	b.touch(now)
	return normalized, nil
}

func validIdentifier(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' || character == '_' || character == '.') {
			return false
		}
	}
	return true
}
