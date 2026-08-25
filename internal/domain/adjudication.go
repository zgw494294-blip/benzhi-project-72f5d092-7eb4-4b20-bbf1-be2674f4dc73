package domain

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

type ConflictDecisionInput struct {
	ConflictID        string `json:"conflictID"`
	Decision          string `json:"decision"`
	ResolvedLabel     string `json:"resolvedLabel,omitempty"`
	ResolutionNote    string `json:"resolutionNote"`
	ReannotationRound int    `json:"reannotationRound,omitempty"`
}

type ConflictDecisionResult struct {
	ConflictID string         `json:"conflictID"`
	ClipID     string         `json:"clipID"`
	Decision   string         `json:"decision"`
	Status     ConflictStatus `json:"status"`
}

type preparedConflictDecision struct {
	conflict          *ConflictCase
	decision          string
	label             string
	reviewer          string
	note              string
	reannotationRound int
}

func (b *ReviewBatch) AdjudicateConflicts(inputs []ConflictDecisionInput, reviewer string, now time.Time) ([]ConflictDecisionResult, error) {
	if err := b.EnsureMutable(); err != nil {
		return nil, err
	}
	if b.Status != StatusAdjudicating {
		return nil, NewError(CodeInvalidState, "仅 adjudicating 状态可批量裁决")
	}
	if len(inputs) == 0 || len(inputs) > MaxBulkItems {
		return nil, NewError(CodeInvalid, "裁决数组数量必须在 1 到 %d 之间", MaxBulkItems)
	}
	seen := make(map[string]int, len(inputs))
	prepared := make([]preparedConflictDecision, 0, len(inputs))
	issues := make([]ItemError, 0)
	for index, input := range inputs {
		input.ConflictID = strings.TrimSpace(input.ConflictID)
		if previous, exists := seen[input.ConflictID]; exists {
			issues = append(issues, ItemError{Index: index, ConflictID: input.ConflictID, Field: "conflictID", ReasonCode: "duplicate_conflict_id", Message: "与请求第 " + strconv.Itoa(previous) + " 项冲突标识重复"})
			continue
		}
		seen[input.ConflictID] = index
		decision := strings.TrimSpace(input.Decision)
		if strings.TrimSpace(input.ResolutionNote) == "" {
			issues = append(issues, ItemError{Index: index, ConflictID: input.ConflictID, Field: "resolutionNote", ReasonCode: "required", Message: "裁决说明不能为空"})
			continue
		}
		if decision == "merge" && NormalizeLabel(input.ResolvedLabel) == "" {
			issues = append(issues, ItemError{Index: index, ConflictID: input.ConflictID, Field: "resolvedLabel", ReasonCode: "required", Message: "合并裁决必须提供标签"})
			continue
		}
		if decision == "reannotate" && input.ReannotationRound == 0 {
			issues = append(issues, ItemError{Index: index, ConflictID: input.ConflictID, Field: "reannotationRound", ReasonCode: "required", Message: "退回重标必须指定 reannotationRound"})
			continue
		}
		if decision == "reannotate" && input.ReannotationRound != 1 && input.ReannotationRound != 2 {
			issues = append(issues, ItemError{Index: index, ConflictID: input.ConflictID, Field: "reannotationRound", ReasonCode: "invalid_round", Message: "reannotationRound 必须为 1 或 2"})
			continue
		}
		if decision != "accept_round_1" && decision != "accept_round_2" && decision != "merge" && decision != "reannotate" {
			issues = append(issues, ItemError{Index: index, ConflictID: input.ConflictID, Field: "decision", ReasonCode: "invalid_decision", Message: "不支持的裁决决定"})
			continue
		}
		item, err := b.validateConflictDecision(input.ConflictID, input.Decision, input.ResolvedLabel, reviewer, input.ResolutionNote, input.ReannotationRound)
		if err != nil {
			typed, _ := err.(*Error)
			reason := "invalid_decision"
			message := err.Error()
			if typed != nil {
				message = typed.Message
				if typed.Code == CodeNotFound {
					reason = "not_found"
				} else if typed.Code == CodeInvalidState {
					reason = "invalid_state"
				}
			}
			issues = append(issues, ItemError{Index: index, ConflictID: input.ConflictID, ReasonCode: reason, Message: message})
			continue
		}
		prepared = append(prepared, item)
	}
	if len(issues) > 0 {
		return nil, NewDetailedError(CodeInvalid, map[string]any{"items": issues}, "批量裁决预检失败")
	}
	sort.Slice(prepared, func(i, j int) bool { return prepared[i].conflict.ConflictID < prepared[j].conflict.ConflictID })
	results := make([]ConflictDecisionResult, 0, len(prepared))
	for _, item := range prepared {
		b.applyConflictDecision(item, now)
		results = append(results, ConflictDecisionResult{ConflictID: item.conflict.ConflictID, ClipID: item.conflict.ClipID, Decision: item.decision, Status: item.conflict.Status})
	}
	b.recalculateStatus()
	b.touch(now)
	return results, nil
}

func (b *ReviewBatch) validateConflictDecision(conflictID, decision, label, reviewer, note string, reannotationRound int) (preparedConflictDecision, error) {
	conflict := b.Conflicts[strings.TrimSpace(conflictID)]
	if conflict == nil {
		return preparedConflictDecision{}, NewError(CodeNotFound, "冲突项不存在")
	}
	decision, reviewer, note = strings.TrimSpace(decision), strings.TrimSpace(reviewer), strings.TrimSpace(note)
	if reviewer == "" || note == "" {
		return preparedConflictDecision{}, NewError(CodeInvalid, "复核人和裁决说明不能为空")
	}
	if conflict.Status == ConflictResolved {
		return preparedConflictDecision{}, NewError(CodeInvalidState, "冲突项已关闭")
	}
	if conflict.Status == ConflictReannotate {
		return preparedConflictDecision{}, NewError(CodeInvalidState, "退回后必须先提交新修订")
	}
	prepared := preparedConflictDecision{conflict: conflict, decision: decision, reviewer: reviewer, note: note, reannotationRound: reannotationRound}
	switch decision {
	case "accept_round_1", "accept_round_2":
		round := 1
		if decision == "accept_round_2" {
			round = 2
		}
		for _, submission := range latestRounds(b.Submissions[conflict.ClipID]) {
			if submission.Round == round {
				prepared.label = submission.SpeciesLabel
			}
		}
		if prepared.label == "" {
			return preparedConflictDecision{}, NewError(CodeInvalid, "采用轮次缺少最新提交")
		}
	case "merge":
		prepared.label = NormalizeLabel(label)
		if prepared.label == "" {
			return preparedConflictDecision{}, NewError(CodeInvalid, "合并裁决必须提供标签")
		}
	case "reannotate":
		if reannotationRound != 0 && reannotationRound != 1 && reannotationRound != 2 {
			return preparedConflictDecision{}, NewError(CodeInvalid, "reannotationRound 必须为 1 或 2")
		}
	default:
		return preparedConflictDecision{}, NewError(CodeInvalid, "不支持的裁决决定")
	}
	return prepared, nil
}

func (b *ReviewBatch) applyConflictDecision(input preparedConflictDecision, now time.Time) {
	conflict := input.conflict
	decision := ConflictDecision{Decision: input.decision, ReviewerID: input.reviewer, ResolutionNote: input.note, ResolvedLabel: NormalizeLabel(input.label), ReannotationRound: input.reannotationRound, At: now.UTC()}
	conflict.DecisionTrail = append(conflict.DecisionTrail, decision)
	conflict.Decision, conflict.ReviewerID, conflict.ResolutionNote = input.decision, input.reviewer, input.note
	if input.decision == "reannotate" {
		conflict.Status, conflict.ReannotationRound = ConflictReannotate, input.reannotationRound
		conflict.ResolvedLabel, conflict.ResolvedAt = "", nil
		return
	}
	resolvedAt := now.UTC()
	conflict.ResolvedLabel = NormalizeLabel(input.label)
	conflict.Status, conflict.ResolvedAt, conflict.ReannotationRound = ConflictResolved, &resolvedAt, 0
}
