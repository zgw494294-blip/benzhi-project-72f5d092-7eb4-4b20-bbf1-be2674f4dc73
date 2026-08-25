package domain

import (
	"strconv"
	"strings"
	"time"
)

type GateRemediationInput struct {
	ClipID                 string  `json:"clipID"`
	AuthorizationConfirmed *bool   `json:"authorizationConfirmed,omitempty"`
	HumanVoiceDetected     *bool   `json:"humanVoiceDetected,omitempty"`
	RedactionNote          *string `json:"redactionNote,omitempty"`
}

type GateRemediationOutcome struct {
	Passed            bool          `json:"passed"`
	Status            BatchStatus   `json:"status"`
	ResolvedBlockers  []GateBlocker `json:"resolvedBlockers"`
	RemainingBlockers []GateBlocker `json:"remainingBlockers"`
}

func (b *ReviewBatch) RemediateGate(inputs []GateRemediationInput, now time.Time) (GateRemediationOutcome, error) {
	if err := b.EnsureMutable(); err != nil {
		return GateRemediationOutcome{}, err
	}
	if len(inputs) == 0 || len(inputs) > MaxBulkItems {
		return GateRemediationOutcome{}, NewError(CodeInvalid, "整改数组数量必须在 1 到 %d 之间", MaxBulkItems)
	}
	before := b.CheckReleaseGate()
	current := make(map[string]map[string]bool)
	for _, blocker := range before {
		if blocker.Code == "authorization_missing" || blocker.Code == "redaction_missing" {
			if current[blocker.ClipID] == nil {
				current[blocker.ClipID] = map[string]bool{}
			}
			current[blocker.ClipID][blocker.Code] = true
		}
	}
	type preparedRemediation struct {
		clip       *RecordingClip
		authorized bool
		voice      bool
		note       string
	}
	prepared := make([]preparedRemediation, 0, len(inputs))
	issues := make([]ItemError, 0)
	seen := make(map[string]int, len(inputs))
	for index, input := range inputs {
		input.ClipID = strings.TrimSpace(input.ClipID)
		addIssue := func(field, reason, message string) {
			issues = append(issues, ItemError{Index: index, ClipID: input.ClipID, Field: field, ReasonCode: reason, Message: message})
		}
		if previous, exists := seen[input.ClipID]; exists {
			addIssue("clipID", "duplicate_clip_id", "与请求第 "+strconv.Itoa(previous)+" 项录音标识重复")
			continue
		}
		seen[input.ClipID] = index
		clip := b.Clips[input.ClipID]
		if clip == nil {
			addIssue("clipID", "not_found", "录音不存在")
			continue
		}
		blockers := current[input.ClipID]
		if len(blockers) == 0 {
			addIssue("clipID", "not_current_blocker", "录音没有可由本接口整改的当前阻断项")
			continue
		}
		value := preparedRemediation{clip: clip, authorized: clip.AuthorizationConfirmed, voice: clip.HumanVoiceDetected, note: clip.RedactionNote}
		addressed := false
		if blockers["authorization_missing"] {
			if input.AuthorizationConfirmed == nil {
				addIssue("authorizationConfirmed", "required", "必须明确提供授权确认值")
			} else if !*input.AuthorizationConfirmed {
				addIssue("authorizationConfirmed", "authorization_not_confirmed", "授权确认必须为 true 才能完成整改")
			} else {
				value.authorized, addressed = true, true
			}
		} else if input.AuthorizationConfirmed != nil {
			addIssue("authorizationConfirmed", "unrelated_field", "当前不存在授权缺失阻断")
		}
		if blockers["redaction_missing"] {
			if input.HumanVoiceDetected == nil && input.RedactionNote == nil {
				addIssue("humanVoiceDetected", "required", "必须提供人声检测结果或静音说明")
			} else {
				if input.HumanVoiceDetected != nil {
					value.voice = *input.HumanVoiceDetected
				}
				if input.RedactionNote != nil {
					value.note = strings.TrimSpace(*input.RedactionNote)
				}
				if value.voice && value.note == "" {
					addIssue("redactionNote", "redaction_note_required", "敏感人声为 true 时静音说明不能为空")
				} else {
					addressed = true
				}
			}
		} else if input.HumanVoiceDetected != nil || input.RedactionNote != nil {
			addIssue("humanVoiceDetected", "unrelated_field", "当前不存在静音说明缺失阻断")
		}
		if !addressed {
			addIssue("clipID", "empty_remediation", "整改项未解决任何当前阻断")
		}
		prepared = append(prepared, value)
	}
	if len(issues) > 0 {
		return GateRemediationOutcome{}, NewDetailedError(CodeInvalid, map[string]any{"items": issues}, "发布门禁批量整改预检失败")
	}
	for _, item := range prepared {
		item.clip.AuthorizationConfirmed = item.authorized
		item.clip.HumanVoiceDetected = item.voice
		item.clip.RedactionNote = item.note
	}
	b.touch(now)
	after := b.CheckReleaseGate()
	afterIndex := make(map[string]bool, len(after))
	for _, blocker := range after {
		afterIndex[gateBlockerKey(blocker)] = true
	}
	resolved := make([]GateBlocker, 0)
	for _, blocker := range before {
		if !afterIndex[gateBlockerKey(blocker)] {
			resolved = append(resolved, blocker)
		}
	}
	return GateRemediationOutcome{Passed: len(after) == 0, Status: b.Status, ResolvedBlockers: resolved, RemainingBlockers: after}, nil
}

func gateBlockerKey(blocker GateBlocker) string {
	return blocker.Code + "\x00" + blocker.ClipID
}
