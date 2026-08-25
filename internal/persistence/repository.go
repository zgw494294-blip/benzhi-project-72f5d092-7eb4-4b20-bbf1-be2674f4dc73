package persistence

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"acoustic-annotation-release/internal/domain"
)

type Repository struct {
	mu           sync.RWMutex
	directory    string
	logPath      string
	snapshotPath string
	logFile      *os.File
	batches      map[string]*domain.ReviewBatch
	idempotency  map[string]IdempotentRecord
	audit        []domain.AuditEvent
	lastSequence int64
	lastDigest   string
}

func Open(directory string) (*Repository, error) {
	if directory == "" {
		return nil, fmt.Errorf("存储目录不能为空")
	}
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return nil, err
	}
	repository := &Repository{directory: directory, logPath: filepath.Join(directory, "events.log"), snapshotPath: filepath.Join(directory, "snapshot.json"), batches: map[string]*domain.ReviewBatch{}, idempotency: map[string]IdempotentRecord{}}
	validBytes, err := repository.recover()
	if err != nil {
		return nil, err
	}
	if err = os.Truncate(repository.logPath, validBytes); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("清理日志末尾半帧失败: %w", err)
	}
	file, err := os.OpenFile(repository.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return nil, err
	}
	repository.logFile = file
	return repository, nil
}

func (r *Repository) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.logFile == nil {
		return nil
	}
	err := r.logFile.Close()
	r.logFile = nil
	return err
}

func (r *Repository) recover() (int64, error) {
	storedSnapshot, err := readSnapshot(r.snapshotPath)
	if err != nil {
		return 0, err
	}
	frames, validBytes, err := readFrames(r.logPath)
	if err != nil {
		return 0, fmt.Errorf("恢复事件日志失败: %w", err)
	}
	startIndex := 0
	if storedSnapshot != nil {
		if storedSnapshot.LastSequence < 0 || storedSnapshot.LastSequence > int64(len(frames)) {
			return 0, fmt.Errorf("快照序号超出事件日志范围")
		}
		if storedSnapshot.LastSequence > 0 && frames[storedSnapshot.LastSequence-1].Digest != storedSnapshot.LastDigest {
			return 0, fmt.Errorf("快照摘要与事件日志不一致")
		}
		for batchID, batch := range storedSnapshot.Batches {
			if batch == nil || batch.BatchID != batchID {
				return 0, fmt.Errorf("快照批次索引无效: %s", batchID)
			}
			if err = batch.ValidateIntegrity(); err != nil {
				return 0, fmt.Errorf("快照批次完整性校验失败: %w", err)
			}
		}
		r.batches = storedSnapshot.Batches
		r.idempotency = storedSnapshot.Idempotency
		r.audit = storedSnapshot.Audit
		r.lastSequence, r.lastDigest = storedSnapshot.LastSequence, storedSnapshot.LastDigest
		startIndex = int(storedSnapshot.LastSequence)
	}
	for _, frame := range frames[startIndex:] {
		var payload logPayload
		if err = json.Unmarshal(frame.Payload, &payload); err != nil {
			return 0, fmt.Errorf("事件载荷损坏: %w", err)
		}
		if payload.SchemaVersion != schemaVersion || payload.Batch == nil {
			return 0, fmt.Errorf("事件载荷 schemaVersion 或批次无效")
		}
		if err = payload.Batch.ValidateIntegrity(); err != nil {
			return 0, fmt.Errorf("事件批次完整性校验失败: %w", err)
		}
		r.batches[payload.Batch.BatchID] = payload.Batch
		r.audit = append(r.audit, payload.Audit)
		r.idempotency[idempotencyIndex(payload.Idempotency.BatchID, payload.Idempotency.Key)] = payload.Idempotency
		r.lastSequence, r.lastDigest = frame.Sequence, frame.Digest
	}
	if r.batches == nil {
		r.batches = map[string]*domain.ReviewBatch{}
	}
	if r.idempotency == nil {
		r.idempotency = map[string]IdempotentRecord{}
	}
	return validBytes, nil
}

func (r *Repository) Create(batch *domain.ReviewBatch, operation, key, actorID, actorRole string, at time.Time) (CommitResult, error) {
	return r.commit(batch.BatchID, 0, operation, key, actorID, actorRole, at, func(current *domain.ReviewBatch) (*domain.ReviewBatch, MutationMetadata, error) {
		if current != nil {
			return nil, MutationMetadata{}, domain.NewError(domain.CodeInvalid, "批次已存在")
		}
		next, err := cloneBatch(batch)
		return next, MutationMetadata{}, err
	})
}

func (r *Repository) Update(batchID string, expectedVersion int64, operation, key, actorID, actorRole string, at time.Time, mutate func(*domain.ReviewBatch) error) (CommitResult, error) {
	return r.UpdateAtomic(batchID, expectedVersion, operation, key, actorID, actorRole, at, func(batch *domain.ReviewBatch) (MutationMetadata, error) {
		return MutationMetadata{}, mutate(batch)
	})
}

func (r *Repository) UpdateAtomic(batchID string, expectedVersion int64, operation, key, actorID, actorRole string, at time.Time, mutate func(*domain.ReviewBatch) (MutationMetadata, error)) (CommitResult, error) {
	return r.commit(batchID, expectedVersion, operation, key, actorID, actorRole, at, func(current *domain.ReviewBatch) (*domain.ReviewBatch, MutationMetadata, error) {
		if current == nil {
			return nil, MutationMetadata{}, domain.NewError(domain.CodeNotFound, "批次不存在")
		}
		working, err := cloneBatch(current)
		if err != nil {
			return nil, MutationMetadata{}, err
		}
		metadata, err := mutate(working)
		if err != nil {
			return nil, MutationMetadata{}, err
		}
		return working, metadata, nil
	})
}

func (r *Repository) commit(batchID string, expectedVersion int64, operation, key, actorID, actorRole string, at time.Time, apply func(*domain.ReviewBatch) (*domain.ReviewBatch, MutationMetadata, error)) (CommitResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if key == "" {
		return CommitResult{}, domain.NewError(domain.CodeInvalid, "idempotencyKey 不能为空")
	}
	index := idempotencyIndex(batchID, key)
	if existing, ok := r.idempotency[index]; ok {
		if existing.Operation != operation {
			return CommitResult{}, domain.NewError(domain.CodeInvalid, "idempotencyKey 已用于其他操作")
		}
		var result CommitResult
		if err := json.Unmarshal(existing.Result, &result); err != nil {
			return CommitResult{}, err
		}
		result.Replay = true
		return result, nil
	}
	current := r.batches[batchID]
	if current != nil && current.Version != expectedVersion {
		return CommitResult{}, domain.NewError(domain.CodeConflict, "版本冲突：当前 %d，期望 %d", current.Version, expectedVersion)
	}
	if current == nil && expectedVersion != 0 {
		return CommitResult{}, domain.NewError(domain.CodeConflict, "新批次期望版本必须为 0")
	}
	next, metadata, err := apply(current)
	if err != nil {
		return CommitResult{}, err
	}
	if err = next.ValidateIntegrity(); err != nil {
		return CommitResult{}, fmt.Errorf("提交后的批次完整性校验失败: %w", err)
	}
	audit := domain.AuditEvent{Sequence: r.lastSequence + 1, BatchID: batchID, Action: operation, ActorID: actorID, ActorRole: actorRole, At: at.UTC(), BatchVersion: next.Version, Details: metadata.AuditDetails}
	result := CommitResult{Batch: next, Audit: audit}
	if metadata.Result != nil {
		result.OperationResult, err = json.Marshal(metadata.Result)
		if err != nil {
			return CommitResult{}, err
		}
	}
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return CommitResult{}, err
	}
	record := IdempotentRecord{Key: key, BatchID: batchID, Operation: operation, Result: resultBytes, CreatedAt: at.UTC()}
	payload := logPayload{SchemaVersion: schemaVersion, Batch: next, Audit: audit, Idempotency: record}
	frame, err := makeFrame(r.lastSequence+1, r.lastDigest, payload)
	if err != nil {
		return CommitResult{}, err
	}
	if r.logFile == nil {
		return CommitResult{}, errors.New("仓储已关闭")
	}
	if err = appendFrame(r.logFile, frame); err != nil {
		return CommitResult{}, err
	}
	r.batches[batchID], r.idempotency[index] = next, record
	r.audit = append(r.audit, audit)
	r.lastSequence, r.lastDigest = frame.Sequence, frame.Digest
	if err = writeSnapshot(r.snapshotPath, r.snapshotState()); err != nil {
		return CommitResult{}, err
	}
	return result, nil
}

func (r *Repository) Get(batchID string) (*domain.ReviewBatch, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	batch := r.batches[batchID]
	if batch == nil {
		return nil, domain.NewError(domain.CodeNotFound, "批次不存在")
	}
	return cloneBatch(batch)
}

func (r *Repository) Audit(batchID string) []domain.AuditEvent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]domain.AuditEvent, 0)
	for _, event := range r.audit {
		if event.BatchID == batchID {
			result = append(result, event)
		}
	}
	return result
}

func (r *Repository) Credentials() []domain.ReleaseCredential {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]domain.ReleaseCredential, 0)
	for _, batch := range r.batches {
		if batch.Credential != nil {
			items = append(items, *batch.Credential)
		}
	}
	return items
}

func (r *Repository) snapshotState() snapshot {
	return snapshot{SchemaVersion: schemaVersion, LastSequence: r.lastSequence, LastDigest: r.lastDigest, Batches: r.batches, Idempotency: r.idempotency, Audit: r.audit}
}

func cloneBatch(batch *domain.ReviewBatch) (*domain.ReviewBatch, error) {
	data, err := json.Marshal(batch)
	if err != nil {
		return nil, err
	}
	var cloned domain.ReviewBatch
	if err = json.Unmarshal(data, &cloned); err != nil {
		return nil, err
	}
	return &cloned, nil
}

func idempotencyIndex(batchID, key string) string { return batchID + "\x00" + key }
