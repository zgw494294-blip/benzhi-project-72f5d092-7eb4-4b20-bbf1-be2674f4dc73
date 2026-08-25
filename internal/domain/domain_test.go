package domain

import (
	"testing"
	"time"
)

func testBatch(t *testing.T) (*ReviewBatch, time.Time) {
	t.Helper()
	now := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	batch, err := NewBatch("batch-test", "测试湿地", now.Add(-time.Hour), now.Add(time.Hour), "已取得调查授权", now)
	if err != nil {
		t.Fatal(err)
	}
	err = batch.AddClip(RecordingClip{ClipID: "clip-a", SourceName: "a.wav", DurationMillis: 10000, ContentDigest: "sha256:a", CaptureTimestamp: now, AuthorizationConfirmed: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = batch.StartAnnotation(now); err != nil {
		t.Fatal(err)
	}
	return batch, now
}

func TestConflictReannotationAndFreeze(t *testing.T) {
	batch, now := testBatch(t)
	first := AnnotationSubmission{SubmissionID: "submission-1", ClipID: "clip-a", Round: 1, AnnotatorID: "ann-1", SpeciesLabel: "Anas platyrhynchos", StartMillis: 1000, EndMillis: 4000, Confidence: 0.95, EvidenceNote: "叫声证据", Revision: 1}
	if conflict, err := batch.SubmitAnnotation(first, now.Add(time.Minute)); err != nil || conflict != nil {
		t.Fatalf("第一轮提交异常: conflict=%v err=%v", conflict, err)
	}
	second := AnnotationSubmission{SubmissionID: "submission-2", ClipID: "clip-a", Round: 2, AnnotatorID: "ann-2", SpeciesLabel: "Ardea cinerea", StartMillis: 1100, EndMillis: 3900, Confidence: 0.90, EvidenceNote: "频谱证据", Revision: 1}
	conflict, err := batch.SubmitAnnotation(second, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if conflict == nil || batch.Status != StatusAdjudicating || conflict.ReasonCode != "label_mismatch" {
		t.Fatalf("未建立预期冲突: %#v status=%s", conflict, batch.Status)
	}
	if err = batch.ResolveConflict(conflict.ConflictID, "reannotate", "", "reviewer", "请复核物种", now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	revision := first
	revision.SubmissionID, revision.SpeciesLabel, revision.Revision = "submission-3", "Ardea cinerea", 2
	if _, err = batch.SubmitAnnotation(revision, now.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if conflict.Status != ConflictReview {
		t.Fatalf("退回修订后状态为 %s", conflict.Status)
	}
	if err = batch.ResolveConflict(conflict.ConflictID, "merge", "ardea cinerea", "reviewer", "修订通过", now.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if batch.Status != StatusReadyForReview {
		t.Fatalf("裁决后状态为 %s", batch.Status)
	}
	manifest, err := batch.Freeze(now.Add(6 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err = VerifyManifest(manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Entries[0].SpeciesLabel != "ardea cinerea" {
		t.Fatalf("清单物种错误: %s", manifest.Entries[0].SpeciesLabel)
	}
	if err = batch.UpdateClipPrivacy("clip-a", true, false, "", now); ErrorCodeOf(err) != CodeFrozen {
		t.Fatalf("冻结后修改应失败: %v", err)
	}
}

func TestBlindVisibilityAndGateBlockers(t *testing.T) {
	batch, now := testBatch(t)
	batch.Clips["clip-a"].AuthorizationConfirmed = false
	batch.Clips["clip-a"].HumanVoiceDetected = true
	first := AnnotationSubmission{SubmissionID: "s1", ClipID: "clip-a", Round: 1, AnnotatorID: "ann-1", SpeciesLabel: "bird", StartMillis: 0, EndMillis: 1000, Confidence: .9, EvidenceNote: "evidence", Revision: 1}
	if _, err := batch.SubmitAnnotation(first, now); err != nil {
		t.Fatal(err)
	}
	visible, err := batch.VisibleSubmissions("clip-a", "ann-2", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 0 {
		t.Fatalf("第二轮标注员提前看到了第一轮: %#v", visible)
	}
	second := first
	second.SubmissionID, second.Round, second.AnnotatorID = "s2", 2, "ann-2"
	if _, err = batch.SubmitAnnotation(second, now); err != nil {
		t.Fatal(err)
	}
	blockers := batch.CheckReleaseGate()
	if len(blockers) != 2 || blockers[0].Code != "authorization_missing" || blockers[1].Code != "redaction_missing" {
		t.Fatalf("门禁阻断不完整: %#v", blockers)
	}
}

func TestSameAnnotatorCannotTakeBothRounds(t *testing.T) {
	batch, now := testBatch(t)
	first := AnnotationSubmission{SubmissionID: "s1", ClipID: "clip-a", Round: 1, AnnotatorID: "same", SpeciesLabel: "bird", StartMillis: 0, EndMillis: 1000, Confidence: .8, EvidenceNote: "evidence", Revision: 1}
	if _, err := batch.SubmitAnnotation(first, now); err != nil {
		t.Fatal(err)
	}
	first.SubmissionID, first.Round = "s2", 2
	if _, err := batch.SubmitAnnotation(first, now); ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("预期身份隔离错误，得到 %v", err)
	}
}
