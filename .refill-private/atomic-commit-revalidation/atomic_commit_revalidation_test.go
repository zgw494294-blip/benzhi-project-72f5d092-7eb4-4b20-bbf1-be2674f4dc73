package atomic_commit_revalidation_test

import (
	"sync"
	"testing"
	"time"

	"acoustic-annotation-release/internal/domain"
	"acoustic-annotation-release/internal/persistence"
)

type updateOutcome struct {
	result persistence.CommitResult
	err    error
}

func TestConcurrentAtomicUpdatesRevalidateAtCommit(t *testing.T) {
	t.Run("expected_version", func(t *testing.T) {
		repository, now := newRepositoryWithBatch(t, "batch-version")
		outcomes := runConcurrentUpdates(t, repository, now, "batch-version", "key-a", "key-b", "clip-a", "clip-b")

		successes, conflicts := 0, 0
		for _, outcome := range outcomes {
			if outcome.err == nil {
				successes++
			} else if domain.ErrorCodeOf(outcome.err) == domain.CodeConflict {
				conflicts++
			}
		}
		if successes != 1 || conflicts != 1 {
			t.Errorf("并发同版本更新必须恰有一次提交和一次版本冲突，得到 successes=%d conflicts=%d errors=[%v, %v]", successes, conflicts, outcomes[0].err, outcomes[1].err)
		}
		batch, err := repository.Get("batch-version")
		if err != nil {
			t.Fatal(err)
		}
		if len(batch.Clips) != 1 || batch.Version != 2 {
			t.Errorf("并发冲突后只应保留一个版本 2 的更新，得到 clips=%d version=%d", len(batch.Clips), batch.Version)
		}
	})

	t.Run("idempotency_key", func(t *testing.T) {
		repository, now := newRepositoryWithBatch(t, "batch-idempotency")
		outcomes := runConcurrentUpdates(t, repository, now, "batch-idempotency", "shared-key", "shared-key", "clip-same", "clip-same")

		successes, replays := 0, 0
		for _, outcome := range outcomes {
			if outcome.err == nil {
				successes++
				if outcome.result.Replay {
					replays++
				}
			}
		}
		if successes != 2 || replays != 1 {
			t.Errorf("并发相同幂等请求必须一次提交、一次重放，得到 successes=%d replays=%d errors=[%v, %v]", successes, replays, outcomes[0].err, outcomes[1].err)
		}
		if auditCount := len(repository.Audit("batch-idempotency")); auditCount != 2 {
			t.Errorf("相同幂等请求只能追加一个更新审计，含创建事件应为 2 条，得到 %d", auditCount)
		}
	})
}

func newRepositoryWithBatch(t *testing.T, batchID string) (*persistence.Repository, time.Time) {
	t.Helper()
	repository, err := persistence.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	now := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	batch, err := domain.NewBatch(batchID, "样地-A", now.Add(-time.Hour), now.Add(time.Hour), "已获得调查授权", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repository.Create(batch, "batch.created", "create-"+batchID, "admin-1", "administrator", now); err != nil {
		t.Fatal(err)
	}
	return repository, now
}

func runConcurrentUpdates(t *testing.T, repository *persistence.Repository, now time.Time, batchID, firstKey, secondKey, firstClip, secondClip string) [2]updateOutcome {
	t.Helper()
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	outcomeChannel := make(chan updateOutcome, 2)
	var started sync.WaitGroup
	started.Add(2)

	launch := func(key, clipID string) {
		defer started.Done()
		result, err := repository.UpdateAtomic(batchID, 1, "clips.bulk_registered", key, "admin-1", "administrator", now.Add(time.Minute), func(batch *domain.ReviewBatch) (persistence.MutationMetadata, error) {
			ready <- struct{}{}
			<-release
			clip := domain.RecordingClip{ClipID: clipID, SourceName: clipID + ".wav", DurationMillis: 1000, ContentDigest: "sha256-" + clipID, CaptureTimestamp: now, AuthorizationConfirmed: true}
			return persistence.MutationMetadata{}, batch.AddClip(clip, now.Add(time.Minute))
		})
		outcomeChannel <- updateOutcome{result: result, err: err}
	}

	go launch(firstKey, firstClip)
	go launch(secondKey, secondClip)
	<-ready
	<-ready
	close(release)
	started.Wait()
	close(outcomeChannel)

	var outcomes [2]updateOutcome
	index := 0
	for outcome := range outcomeChannel {
		outcomes[index] = outcome
		index++
	}
	return outcomes
}
