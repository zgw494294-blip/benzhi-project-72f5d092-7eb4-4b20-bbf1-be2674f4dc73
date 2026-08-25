package persistence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"acoustic-annotation-release/internal/domain"
)

func repositoryBatch(t *testing.T) (*domain.ReviewBatch, time.Time) {
	t.Helper()
	now := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	batch, err := domain.NewBatch("repo-batch", "湖区", now.Add(-time.Hour), now.Add(time.Hour), "授权", now)
	if err != nil {
		t.Fatal(err)
	}
	return batch, now
}

func TestRepositoryIdempotencyVersionAndRecovery(t *testing.T) {
	directory := t.TempDir()
	repository, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	batch, now := repositoryBatch(t)
	created, err := repository.Create(batch, "batch.created", "key-create", "admin", "administrator", now)
	if err != nil {
		t.Fatal(err)
	}
	if created.Replay || created.Batch.Version != 1 {
		t.Fatalf("创建结果错误: %#v", created)
	}
	replayed, err := repository.Create(batch, "batch.created", "key-create", "admin", "administrator", now)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replay || replayed.Batch.Version != 1 {
		t.Fatalf("幂等重放错误: %#v", replayed)
	}
	clip := domain.RecordingClip{ClipID: "clip", SourceName: "clip.wav", DurationMillis: 2000, ContentDigest: "digest", CaptureTimestamp: now, AuthorizationConfirmed: true}
	updated, err := repository.Update(batch.BatchID, 1, "clip.registered", "key-clip", "admin", "administrator", now, func(value *domain.ReviewBatch) error { return value.AddClip(clip, now) })
	if err != nil {
		t.Fatal(err)
	}
	if updated.Batch.Version != 2 {
		t.Fatalf("更新版本为 %d", updated.Batch.Version)
	}
	_, err = repository.Update(batch.BatchID, 1, "annotation.started", "key-stale", "admin", "administrator", now, func(value *domain.ReviewBatch) error { return value.StartAnnotation(now) })
	if domain.ErrorCodeOf(err) != domain.CodeConflict {
		t.Fatalf("过期版本未冲突: %v", err)
	}
	if err = repository.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restored, err := reopened.Get(batch.BatchID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Version != 2 || len(restored.Clips) != 1 || len(reopened.Audit(batch.BatchID)) != 2 {
		t.Fatalf("恢复结果错误: version=%d clips=%d audit=%d", restored.Version, len(restored.Clips), len(reopened.Audit(batch.BatchID)))
	}
}

func TestRepositoryReplaysAtomicOperationResultAndAuditDetails(t *testing.T) {
	repository, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	batch, now := repositoryBatch(t)
	if _, err = repository.Create(batch, "batch.created", "create", "admin", "administrator", now); err != nil {
		t.Fatal(err)
	}
	clip := domain.RecordingClip{ClipID: "clip-a", SourceName: "a.wav", DurationMillis: 1000, ContentDigest: "digest-a", CaptureTimestamp: now}
	result, err := repository.UpdateAtomic(batch.BatchID, 1, "clips.bulk_registered", "bulk", "admin", "administrator", now, func(value *domain.ReviewBatch) (MutationMetadata, error) {
		clips, inner := value.RegisterClips([]domain.RecordingClip{clip}, now)
		return MutationMetadata{Result: clips, AuditDetails: map[string]any{"count": 1, "clipIDs": []string{"clip-a"}}}, inner
	})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := repository.UpdateAtomic(batch.BatchID, 1, "clips.bulk_registered", "bulk", "admin", "administrator", now, func(value *domain.ReviewBatch) (MutationMetadata, error) {
		t.Fatal("幂等重放不应再次执行领域变更")
		return MutationMetadata{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replay || replayed.Batch.Version != 2 || string(replayed.OperationResult) != string(result.OperationResult) {
		t.Fatalf("原子结果重放不稳定: first=%#v replay=%#v", result, replayed)
	}
	var clips []domain.RecordingClip
	if err = json.Unmarshal(replayed.OperationResult, &clips); err != nil || len(clips) != 1 || clips[0].ClipID != "clip-a" {
		t.Fatalf("重放业务结果无效: %#v err=%v", clips, err)
	}
	audit := repository.Audit(batch.BatchID)
	if len(audit) != 2 || audit[1].Details["count"].(int) != 1 {
		t.Fatalf("幂等重放追加了审计或丢失摘要: %#v", audit)
	}
}

func TestRepositoryTruncatesTrailingPartialFrame(t *testing.T) {
	directory := t.TempDir()
	repository, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	batch, now := repositoryBatch(t)
	if _, err = repository.Create(batch, "batch.created", "create", "admin", "administrator", now); err != nil {
		t.Fatal(err)
	}
	if err = repository.Close(); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(directory, "events.log")
	before, err := os.Stat(logPath)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.Write([]byte{0, 0, 1}); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(directory)
	if err != nil {
		t.Fatalf("可判定末尾半帧应被忽略: %v", err)
	}
	if _, err = reopened.Get(batch.BatchID); err != nil {
		t.Fatal(err)
	}
	if err = reopened.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if after.Size() != before.Size() {
		t.Fatalf("末尾半帧未截断: before=%d after=%d", before.Size(), after.Size())
	}
}

func TestRepositoryRejectsCorruptCompleteFrame(t *testing.T) {
	directory := t.TempDir()
	repository, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	batch, now := repositoryBatch(t)
	if _, err = repository.Create(batch, "batch.created", "create", "admin", "administrator", now); err != nil {
		t.Fatal(err)
	}
	if err = repository.Close(); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(directory, "events.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-1] ^= 0xff
	if err = os.WriteFile(logPath, data, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(directory); err == nil {
		t.Fatal("完整损坏帧不应被当作末尾半帧忽略")
	}
}
