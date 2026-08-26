package failed_mutation_isolation_test

import (
	"errors"
	"testing"
	"time"

	"acoustic-annotation-release/internal/domain"
	"acoustic-annotation-release/internal/persistence"
)

func TestFailedMutationDoesNotLeakIntoRepository(t *testing.T) {
	repository, err := persistence.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	now := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	batch, err := domain.NewBatch(
		"failed-mutation-batch",
		"河谷样地",
		now,
		now.Add(time.Hour),
		"已取得录音授权",
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repository.Create(batch, "batch.created", "create-batch", "admin-1", "administrator", now); err != nil {
		t.Fatal(err)
	}

	mutationFailure := errors.New("模拟跨层后置校验失败")
	_, err = repository.Update(
		batch.BatchID,
		batch.Version,
		"clip.registered",
		"failed-add-clip",
		"admin-1",
		"administrator",
		now.Add(time.Minute),
		func(working *domain.ReviewBatch) error {
			clip := domain.RecordingClip{
				ClipID:                 "clip-leaked",
				SourceName:             "leaked.wav",
				DurationMillis:         2500,
				ContentDigest:          "sha256-leaked",
				CaptureTimestamp:       now.Add(30 * time.Minute),
				AuthorizationConfirmed: true,
			}
			if inner := working.AddClip(clip, now.Add(time.Minute)); inner != nil {
				return inner
			}
			return mutationFailure
		},
	)
	if !errors.Is(err, mutationFailure) {
		t.Fatalf("事务应返回模拟失败，得到 %v", err)
	}

	stored, err := repository.Get(batch.BatchID)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := stored.Clips["clip-leaked"]; exists {
		t.Fatalf("失败 mutation 泄漏录音到仓储；version=%d", stored.Version)
	}
	if stored.Version != batch.Version {
		t.Fatalf("失败 mutation 改变版本：got %d want %d", stored.Version, batch.Version)
	}
}
