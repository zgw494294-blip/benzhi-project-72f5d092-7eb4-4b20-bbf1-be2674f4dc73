package domain

import (
	"strings"
	"time"
)

func NewBatch(id, site string, start, end time.Time, authorization string, now time.Time) (*ReviewBatch, error) {
	id, site, authorization = strings.TrimSpace(id), strings.TrimSpace(site), strings.TrimSpace(authorization)
	if id == "" || site == "" || authorization == "" {
		return nil, NewError(CodeInvalid, "批次 ID、调查地点和授权声明不能为空")
	}
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return nil, NewError(CodeInvalid, "采集结束时间必须晚于开始时间")
	}
	return &ReviewBatch{
		BatchID: id, SurveySite: site, CaptureWindowStart: start, CaptureWindowEnd: end,
		AuthorizationStatement: authorization, Status: StatusDraft, Version: 1,
		CreatedAt: now.UTC(), UpdatedAt: now.UTC(), Clips: map[string]*RecordingClip{},
		Submissions: map[string][]AnnotationSubmission{}, Conflicts: map[string]*ConflictCase{},
	}, nil
}

func (b *ReviewBatch) EnsureMutable() error {
	if b.Status == StatusFrozen || b.Status == StatusReleased {
		return NewError(CodeFrozen, "批次冻结后不可修改事实")
	}
	return nil
}

func (b *ReviewBatch) AddClip(clip RecordingClip, now time.Time) error {
	_, err := b.RegisterClips([]RecordingClip{clip}, now)
	return err
}

func (b *ReviewBatch) StartAnnotation(now time.Time) error {
	if err := b.EnsureMutable(); err != nil {
		return err
	}
	if b.Status != StatusDraft {
		return NewError(CodeInvalidState, "仅 draft 批次可开始标注")
	}
	if len(b.Clips) == 0 {
		return NewError(CodeInvalidState, "空批次不能进入标注阶段")
	}
	b.Status = StatusAnnotating
	b.touch(now)
	return nil
}

func (b *ReviewBatch) UpdateClipPrivacy(clipID string, authorized, voice bool, note string, now time.Time) error {
	if err := b.EnsureMutable(); err != nil {
		return err
	}
	clip, ok := b.Clips[clipID]
	if !ok {
		return NewError(CodeNotFound, "录音 %s 不存在", clipID)
	}
	clip.AuthorizationConfirmed, clip.HumanVoiceDetected, clip.RedactionNote = authorized, voice, strings.TrimSpace(note)
	b.touch(now)
	return nil
}

func (b *ReviewBatch) touch(now time.Time) {
	b.Version++
	b.UpdatedAt = now.UTC()
}
