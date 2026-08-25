package domain

import (
	"testing"
	"time"
)

func TestBulkClipRegistrationIsAtomicAndStable(t *testing.T) {
	now := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	batch, err := NewBatch("bulk", "湿地", now.Add(-time.Hour), now.Add(time.Hour), "授权", now)
	if err != nil {
		t.Fatal(err)
	}
	clips := []RecordingClip{
		{ClipID: "clip-z", SourceName: "z.wav", DurationMillis: 1000, ContentDigest: " SHA256:Z ", CaptureTimestamp: now},
		{ClipID: "clip-a", SourceName: "a.wav", DurationMillis: 1000, ContentDigest: "SHA256:A", CaptureTimestamp: now},
	}
	registered, err := batch.RegisterClips(clips, now)
	if err != nil {
		t.Fatal(err)
	}
	if batch.Version != 2 || registered[0].ClipID != "clip-a" || registered[1].ClipID != "clip-z" {
		t.Fatalf("批量登记没有一次提交或稳定排序: version=%d clips=%#v", batch.Version, registered)
	}
	version := batch.Version
	_, err = batch.RegisterClips([]RecordingClip{
		{ClipID: "clip-b", SourceName: "b.wav", DurationMillis: 1000, ContentDigest: "sha256:b", CaptureTimestamp: now},
		{ClipID: "clip-c", SourceName: "c.wav", DurationMillis: 1000, ContentDigest: " sha256:a ", CaptureTimestamp: now},
	}, now)
	if ErrorCodeOf(err) != CodeInvalid || batch.Version != version || len(batch.Clips) != 2 {
		t.Fatalf("重复摘要批次应全部回滚: err=%v version=%d clips=%d", err, batch.Version, len(batch.Clips))
	}
	typed := err.(*Error)
	if typed.Details == nil {
		t.Fatal("批量错误缺少结构化项目明细")
	}
}

func TestBlindTasksProtectRoundsAndGuideReannotation(t *testing.T) {
	batch, now := testBatch(t)
	tasks, err := batch.BlindTasks("ann-1", 1, "pending")
	if err != nil || len(tasks) != 1 || tasks[0].NextRevision != 1 {
		t.Fatalf("初始待办错误: %#v err=%v", tasks, err)
	}
	first := AnnotationSubmission{SubmissionID: "s1", ClipID: "clip-a", Round: 1, AnnotatorID: "ann-1", SpeciesLabel: "bird-a", StartMillis: 0, EndMillis: 1000, Confidence: .9, EvidenceNote: "证据", Revision: 1}
	if _, err = batch.SubmitAnnotation(first, now); err != nil {
		t.Fatal(err)
	}
	tasks, err = batch.BlindTasks("ann-1", 1, "")
	if err != nil || tasks[0].Status != BlindTaskSubmitted || tasks[0].OwnSubmission == nil || tasks[0].NextRevision != 2 {
		t.Fatalf("自有提交待办错误: %#v err=%v", tasks, err)
	}
	ineligible, err := batch.BlindTasks("ann-1", 2, "")
	if err != nil || ineligible[0].Status != BlindTaskIneligible {
		t.Fatalf("跨轮身份未隔离: %#v err=%v", ineligible, err)
	}
	second := first
	second.SubmissionID, second.Round, second.AnnotatorID, second.SpeciesLabel = "s2", 2, "ann-2", "bird-b"
	if _, err = batch.SubmitAnnotation(second, now); err != nil {
		t.Fatal(err)
	}
	decisions := []ConflictDecisionInput{{ConflictID: "conflict-clip-a", Decision: "reannotate", ResolutionNote: "重做第一轮", ReannotationRound: 1}}
	if _, err = batch.AdjudicateConflicts(decisions, "reviewer", now); err != nil {
		t.Fatal(err)
	}
	tasks, err = batch.BlindTasks("ann-1", 1, "")
	if err != nil || tasks[0].Status != BlindTaskAwaitingReannotation || tasks[0].NextRevision != 2 {
		t.Fatalf("退回修订指引错误: %#v err=%v", tasks, err)
	}
	first.SubmissionID, first.SpeciesLabel, first.Revision = "s3", "bird-b", 2
	if _, err = batch.SubmitAnnotation(first, now); err != nil {
		t.Fatal(err)
	}
	if batch.Conflicts["conflict-clip-a"].Status != ConflictReview {
		t.Fatalf("新修订后冲突状态为 %s", batch.Conflicts["conflict-clip-a"].Status)
	}
}

func TestBulkAdjudicationAndGateRemediationAreAllOrNothing(t *testing.T) {
	now := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	batch, err := NewBatch("decisions", "湿地", now.Add(-time.Hour), now.Add(time.Hour), "授权", now)
	if err != nil {
		t.Fatal(err)
	}
	clips := []RecordingClip{
		{ClipID: "clip-a", SourceName: "a.wav", DurationMillis: 5000, ContentDigest: "a", CaptureTimestamp: now, HumanVoiceDetected: true},
		{ClipID: "clip-b", SourceName: "b.wav", DurationMillis: 5000, ContentDigest: "b", CaptureTimestamp: now, AuthorizationConfirmed: true},
	}
	if _, err = batch.RegisterClips(clips, now); err != nil {
		t.Fatal(err)
	}
	if err = batch.StartAnnotation(now); err != nil {
		t.Fatal(err)
	}
	for _, clipID := range []string{"clip-a", "clip-b"} {
		first := AnnotationSubmission{SubmissionID: "s1-" + clipID, ClipID: clipID, Round: 1, AnnotatorID: "ann-1", SpeciesLabel: "bird-a", StartMillis: 0, EndMillis: 1000, Confidence: .9, EvidenceNote: "证据", Revision: 1}
		second := first
		second.SubmissionID, second.Round, second.AnnotatorID, second.SpeciesLabel = "s2-"+clipID, 2, "ann-2", "bird-b"
		if _, err = batch.SubmitAnnotation(first, now); err != nil {
			t.Fatal(err)
		}
		if _, err = batch.SubmitAnnotation(second, now); err != nil {
			t.Fatal(err)
		}
	}
	version := batch.Version
	bad := []ConflictDecisionInput{
		{ConflictID: "conflict-clip-a", Decision: "accept_round_1", ResolutionNote: "采用第一轮"},
		{ConflictID: "conflict-clip-b", Decision: "merge", ResolutionNote: "缺少标签"},
	}
	if _, err = batch.AdjudicateConflicts(bad, "reviewer", now); ErrorCodeOf(err) != CodeInvalid {
		t.Fatalf("预期批量裁决失败: %v", err)
	}
	if batch.Version != version || batch.Conflicts["conflict-clip-a"].Status != ConflictOpen {
		t.Fatal("失败的批量裁决产生了部分修改")
	}
	good := []ConflictDecisionInput{
		{ConflictID: "conflict-clip-a", Decision: "accept_round_1", ResolutionNote: "采用第一轮"},
		{ConflictID: "conflict-clip-b", Decision: "merge", ResolvedLabel: "bird-b", ResolutionNote: "合并"},
	}
	if _, err = batch.AdjudicateConflicts(good, "reviewer", now); err != nil {
		t.Fatal(err)
	}
	if batch.Version != version+1 || batch.Status != StatusReadyForReview {
		t.Fatalf("裁决未以单版本完成: version=%d status=%s", batch.Version, batch.Status)
	}
	version = batch.Version
	confirmed := true
	voice := true
	note := "已静音"
	invalid := []GateRemediationInput{
		{ClipID: "clip-a", AuthorizationConfirmed: &confirmed, HumanVoiceDetected: &voice, RedactionNote: &note},
		{ClipID: "missing", AuthorizationConfirmed: &confirmed},
	}
	if _, err = batch.RemediateGate(invalid, now); ErrorCodeOf(err) != CodeInvalid {
		t.Fatalf("预期整改失败: %v", err)
	}
	if batch.Version != version || batch.Clips["clip-a"].AuthorizationConfirmed {
		t.Fatal("失败的批量整改产生了部分修改")
	}
	outcome, err := batch.RemediateGate([]GateRemediationInput{{ClipID: "clip-a", AuthorizationConfirmed: &confirmed, HumanVoiceDetected: &voice, RedactionNote: &note}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.Passed || len(outcome.ResolvedBlockers) != 2 || len(outcome.RemainingBlockers) != 0 {
		t.Fatalf("整改复检差异错误: %#v", outcome)
	}
}
