package canceled_queued_write

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"acoustic-annotation-release/internal/certificate"
	"acoustic-annotation-release/internal/domain"
	"acoustic-annotation-release/internal/httpapi"
	"acoustic-annotation-release/internal/persistence"
	"acoustic-annotation-release/internal/workflow"
)

type checkpointContext struct {
	context.Context
	checked chan struct{}
	once    sync.Once
}

func (c *checkpointContext) Err() error {
	err := c.Context.Err()
	c.once.Do(func() { close(c.checked) })
	return err
}

func TestCanceledQueuedWriteDoesNotCommit(t *testing.T) {
	repository, err := persistence.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })

	service := workflow.NewService(repository, certificate.NewService())
	start := time.Date(2025, 4, 8, 1, 0, 0, 0, time.UTC)
	_, _, err = service.CreateBatch(workflow.CreateBatchInput{
		BatchID:                "batch-context",
		SurveySite:             "受控调度样地",
		CaptureWindowStart:     start,
		CaptureWindowEnd:       start.Add(time.Hour),
		AuthorizationStatement: "已获调查授权",
	}, workflow.WriteContext{ActorID: "admin-1", Role: workflow.RoleAdministrator, ExpectedVersion: 0, IdempotencyKey: "create-context-batch"})
	if err != nil {
		t.Fatal(err)
	}

	mutationEntered := make(chan struct{})
	releaseMutation := make(chan struct{})
	blockerDone := make(chan error, 1)
	go func() {
		_, updateErr := repository.UpdateAtomic("batch-context", 1, "test.blocker", "test-blocker-key", "admin-1", workflow.RoleAdministrator, start, func(batch *domain.ReviewBatch) (persistence.MutationMetadata, error) {
			close(mutationEntered)
			<-releaseMutation
			return persistence.MutationMetadata{}, errors.New("受控 mutation 已释放")
		})
		blockerDone <- updateErr
	}()
	<-mutationEntered

	baseContext, cancel := context.WithCancel(context.Background())
	observed := &checkpointContext{Context: baseContext, checked: make(chan struct{})}
	body := `{"clipID":"clip-after-cancel","sourceName":"cancel.wav","durationMillis":1000,"contentDigest":"sha256:cancel","captureTimestamp":"2025-04-08T01:10:00Z","authorizationConfirmed":true,"humanVoiceDetected":false,"redactionNote":""}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/review-batches/batch-context/clips", strings.NewReader(body)).WithContext(observed)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Actor-ID", "admin-1")
	request.Header.Set("X-Role", workflow.RoleAdministrator)
	request.Header.Set("If-Match-Version", "1")
	request.Header.Set("Idempotency-Key", "add-after-cancel")
	recorder := httptest.NewRecorder()
	handlerDone := make(chan struct{})
	api := httpapi.New(service, slog.New(slog.NewTextHandler(io.Discard, nil)))
	go func() {
		api.Handler().ServeHTTP(recorder, request)
		close(handlerDone)
	}()

	<-observed.checked
	cancel()
	close(releaseMutation)
	<-handlerDone
	if blockerErr := <-blockerDone; blockerErr == nil || blockerErr.Error() != "受控 mutation 已释放" {
		t.Fatalf("受控锁持有者返回意外结果: %v", blockerErr)
	}

	batch, err := repository.Get("batch-context")
	if err != nil {
		t.Fatal(err)
	}
	if batch.Version != 1 || batch.Clips["clip-after-cancel"] != nil {
		t.Fatalf("取消后的排队写入仍被提交: status=%d version=%d clipPresent=%t", recorder.Code, batch.Version, batch.Clips["clip-after-cancel"] != nil)
	}
}
