package releasegatecache_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"acoustic-annotation-release/internal/certificate"
	"acoustic-annotation-release/internal/domain"
	"acoustic-annotation-release/internal/httpapi"
	"acoustic-annotation-release/internal/persistence"
	"acoustic-annotation-release/internal/workflow"
)

func TestReleaseGateCacheTracksBatchVersion(t *testing.T) {
	batch := readyBatchWithMissingAuthorization(t)
	repository, err := persistence.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if _, err = repository.Create(batch, "fixture.created", "fixture-create", "fixture", workflow.RoleAdministrator, batch.UpdatedAt); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(httpapi.New(workflow.NewService(repository, certificate.NewService()), logger).Handler())
	defer server.Close()

	first := readGate(t, server.URL, batch.BatchID)
	if first.Passed || !hasBlocker(first.Blockers, "authorization_missing") {
		t.Fatalf("首次门禁查询未返回授权阻断: %+v", first)
	}

	body := []byte(`{"remediations":[{"clipID":"clip-a","authorizationConfirmed":true}]}`)
	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/review-batches/"+batch.BatchID+"/release-gate/remediations", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Role", workflow.RoleReviewer)
	request.Header.Set("X-Actor-ID", "reviewer-a")
	request.Header.Set("Idempotency-Key", "remediate-authorization")
	request.Header.Set("If-Match-Version", strconv.FormatInt(batch.Version, 10))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	responseBody, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if response.StatusCode != http.StatusOK || !bytes.Contains(responseBody, []byte(`"passed":true`)) {
		t.Fatalf("整改请求未成功: status=%d body=%s", response.StatusCode, responseBody)
	}
	persisted, err := repository.Get(batch.BatchID)
	if err != nil {
		t.Fatal(err)
	}
	if blockers := persisted.CheckReleaseGate(); len(blockers) != 0 {
		t.Fatalf("整改未写入仓储: %+v", blockers)
	}

	second := readGate(t, server.URL, batch.BatchID)
	if !second.Passed || len(second.Blockers) != 0 {
		t.Fatalf("整改后的门禁查询复用了旧版本结果: %+v", second)
	}
}

func readyBatchWithMissingAuthorization(t *testing.T) *domain.ReviewBatch {
	t.Helper()
	start := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	batch, err := domain.NewBatch("gate-cache-batch", "湿地", start, start.Add(time.Hour), "调查授权", start)
	if err != nil {
		t.Fatal(err)
	}
	clip := domain.RecordingClip{ClipID: "clip-a", SourceName: "a.wav", DurationMillis: 5_000, ContentDigest: "digest-a", CaptureTimestamp: start.Add(time.Minute)}
	if err = batch.AddClip(clip, start.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err = batch.StartAnnotation(start.Add(2 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	first := domain.AnnotationSubmission{SubmissionID: "submission-a", ClipID: "clip-a", Round: 1, AnnotatorID: "annotator-a", SpeciesLabel: "bird", StartMillis: 100, EndMillis: 1_000, Confidence: 0.9, EvidenceNote: "证据一", Revision: 1}
	if _, err = batch.SubmitAnnotation(first, start.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	second := domain.AnnotationSubmission{SubmissionID: "submission-b", ClipID: "clip-a", Round: 2, AnnotatorID: "annotator-b", SpeciesLabel: "bird", StartMillis: 100, EndMillis: 1_000, Confidence: 0.9, EvidenceNote: "证据二", Revision: 1}
	if _, err = batch.SubmitAnnotation(second, start.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	return batch
}

func readGate(t *testing.T, baseURL, batchID string) workflow.GateResult {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, baseURL+"/api/v1/review-batches/"+batchID+"/release-gate", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Role", workflow.RoleReviewer)
	request.Header.Set("X-Actor-ID", "reviewer-a")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("门禁查询返回 %d: %s", response.StatusCode, data)
	}
	var result workflow.GateResult
	if err = json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func hasBlocker(blockers []domain.GateBlocker, code string) bool {
	for _, blocker := range blockers {
		if blocker.Code == code {
			return true
		}
	}
	return false
}
