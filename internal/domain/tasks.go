package domain

import (
	"sort"
	"strings"
)

type BlindTaskStatus string

const (
	BlindTaskPending              BlindTaskStatus = "pending"
	BlindTaskSubmitted            BlindTaskStatus = "submitted"
	BlindTaskAwaitingReannotation BlindTaskStatus = "awaiting_reannotation"
	BlindTaskCompleted            BlindTaskStatus = "completed"
	BlindTaskIneligible           BlindTaskStatus = "ineligible"
)

type BlindTask struct {
	ClipID              string                `json:"clipID"`
	Round               int                   `json:"round"`
	Status              BlindTaskStatus       `json:"status"`
	NextRevision        int                   `json:"nextRevision"`
	Eligible            bool                  `json:"eligible"`
	IneligibilityReason string                `json:"ineligibilityReason,omitempty"`
	OwnSubmission       *AnnotationSubmission `json:"ownSubmission,omitempty"`
}

func ValidBlindTaskStatus(value string) bool {
	switch BlindTaskStatus(value) {
	case BlindTaskPending, BlindTaskSubmitted, BlindTaskAwaitingReannotation, BlindTaskCompleted, BlindTaskIneligible:
		return true
	default:
		return false
	}
}

func (b *ReviewBatch) BlindTasks(actorID string, round int, status string) ([]BlindTask, error) {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return nil, NewError(CodeInvalid, "标注员身份不能为空")
	}
	if round != 1 && round != 2 {
		return nil, NewError(CodeInvalid, "round 必须为 1 或 2")
	}
	if status != "" && !ValidBlindTaskStatus(status) {
		return nil, NewError(CodeInvalid, "未知待办状态 %s", status)
	}
	if b.Status == StatusDraft {
		return nil, NewError(CodeInvalidState, "批次尚未进入 annotating")
	}
	clipIDs := make([]string, 0, len(b.Clips))
	for clipID := range b.Clips {
		clipIDs = append(clipIDs, clipID)
	}
	sort.Strings(clipIDs)
	result := make([]BlindTask, 0, len(clipIDs))
	for _, clipID := range clipIDs {
		task := b.blindTask(clipID, actorID, round)
		if status == "" || string(task.Status) == status {
			result = append(result, task)
		}
	}
	return result, nil
}

func (b *ReviewBatch) blindTask(clipID, actorID string, round int) BlindTask {
	task := BlindTask{ClipID: clipID, Round: round, Status: BlindTaskPending, NextRevision: 1, Eligible: true}
	latest := latestRounds(b.Submissions[clipID])
	var requested *AnnotationSubmission
	otherOwnedByActor := false
	requestedOwnedByOther := false
	for index := range latest {
		submission := latest[index]
		if submission.Round == round {
			if submission.AnnotatorID == actorID {
				copy := submission
				requested = &copy
			} else {
				requestedOwnedByOther = true
			}
		} else if submission.AnnotatorID == actorID {
			otherOwnedByActor = true
		}
	}
	if otherOwnedByActor {
		task.Status, task.Eligible, task.NextRevision = BlindTaskIneligible, false, 0
		task.IneligibilityReason = "same_annotator_other_round"
		return task
	}
	if requestedOwnedByOther {
		task.Status, task.Eligible, task.NextRevision = BlindTaskCompleted, false, 0
		task.IneligibilityReason = "round_assigned_to_other"
		return task
	}
	if requested == nil {
		return task
	}
	task.OwnSubmission = requested
	task.NextRevision = requested.Revision + 1
	conflict := b.Conflicts["conflict-"+clipID]
	if conflict != nil && conflict.Status == ConflictReannotate && (conflict.ReannotationRound == 0 || conflict.ReannotationRound == round) {
		task.Status = BlindTaskAwaitingReannotation
		return task
	}
	if len(latest) == 2 && (conflict == nil || conflict.Status == ConflictResolved) {
		task.Status = BlindTaskCompleted
		return task
	}
	task.Status = BlindTaskSubmitted
	return task
}

type ConflictTask struct {
	Conflict          ConflictCase           `json:"conflict"`
	LatestSubmissions []AnnotationSubmission `json:"latestSubmissions"`
}

func ValidConflictStatus(value string) bool {
	switch ConflictStatus(value) {
	case ConflictOpen, ConflictReannotate, ConflictReview, ConflictResolved:
		return true
	default:
		return false
	}
}

func ValidConflictReason(value string) bool {
	return value == "label_mismatch" || value == "interval_divergence" || value == "low_confidence"
}

func (b *ReviewBatch) ConflictTasks(status, reason string) ([]ConflictTask, error) {
	if status != "" && !ValidConflictStatus(status) {
		return nil, NewError(CodeInvalid, "未知冲突状态 %s", status)
	}
	if reason != "" && !ValidConflictReason(reason) {
		return nil, NewError(CodeInvalid, "未知冲突原因 %s", reason)
	}
	if b.Status == StatusDraft {
		return nil, NewError(CodeInvalidState, "批次尚未进入 annotating")
	}
	result := make([]ConflictTask, 0, len(b.Conflicts))
	for _, conflict := range b.Conflicts {
		if status != "" && string(conflict.Status) != status || reason != "" && conflict.ReasonCode != reason {
			continue
		}
		copy := *conflict
		copy.DecisionTrail = append([]ConflictDecision(nil), conflict.DecisionTrail...)
		result = append(result, ConflictTask{Conflict: copy, LatestSubmissions: latestRounds(b.Submissions[conflict.ClipID])})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Conflict.ClipID == result[j].Conflict.ClipID {
			return result[i].Conflict.ConflictID < result[j].Conflict.ConflictID
		}
		return result[i].Conflict.ClipID < result[j].Conflict.ClipID
	})
	return result, nil
}
