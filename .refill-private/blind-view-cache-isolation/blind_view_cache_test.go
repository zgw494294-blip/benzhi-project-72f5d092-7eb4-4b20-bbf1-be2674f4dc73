package blindviewcacheisolation

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
	"acoustic-annotation-release/internal/httpapi"
	"acoustic-annotation-release/internal/persistence"
	"acoustic-annotation-release/internal/workflow"
)

type testClient struct {
	t       *testing.T
	baseURL string
	version int64
}

func (c *testClient) write(path string, body any, role, actor, key string, wantStatus int) {
	c.t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		c.t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		c.t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Role", role)
	request.Header.Set("X-Actor-ID", actor)
	request.Header.Set("Idempotency-Key", key)
	request.Header.Set("If-Match-Version", strconv.FormatInt(c.version, 10))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		c.t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		c.t.Fatal(err)
	}
	if response.StatusCode != wantStatus {
		c.t.Fatalf("POST %s 返回 %d，期望 %d: %s", path, response.StatusCode, wantStatus, data)
	}
	var result struct {
		Batch struct {
			Version int64 `json:"version"`
		} `json:"batch"`
	}
	if err = json.Unmarshal(data, &result); err != nil {
		c.t.Fatal(err)
	}
	c.version = result.Batch.Version
}

func (c *testClient) blindView(actor string, round int) []byte {
	c.t.Helper()
	request, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/v1/review-batches/cache-batch/clips/clip-a/annotations?round="+strconv.Itoa(round), nil)
	if err != nil {
		c.t.Fatal(err)
	}
	request.Header.Set("X-Role", "annotator")
	request.Header.Set("X-Actor-ID", actor)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		c.t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		c.t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		c.t.Fatalf("GET 盲标结果返回 %d: %s", response.StatusCode, data)
	}
	return data
}

func TestBlindSubmissionCacheIsolatedByActorAndRound(t *testing.T) {
	repository, err := persistence.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(httpapi.New(workflow.NewService(repository, certificate.NewService()), logger).Handler())
	defer server.Close()
	client := &testClient{t: t, baseURL: server.URL}

	start := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	client.write("/api/v1/review-batches", workflow.CreateBatchInput{
		BatchID: "cache-batch", SurveySite: "湿地", CaptureWindowStart: start,
		CaptureWindowEnd: start.Add(time.Hour), AuthorizationStatement: "授权",
	}, "administrator", "admin", "create", http.StatusCreated)
	client.write("/api/v1/review-batches/cache-batch/clips", workflow.AddClipInput{
		ClipID: "clip-a", SourceName: "a.wav", DurationMillis: 5000,
		ContentDigest: "digest-a", CaptureTimestamp: start.Add(time.Minute), AuthorizationConfirmed: true,
	}, "administrator", "admin", "clip", http.StatusOK)
	client.write("/api/v1/review-batches/cache-batch/start-annotation", struct{}{}, "administrator", "admin", "start", http.StatusOK)
	client.write("/api/v1/review-batches/cache-batch/annotations", workflow.AnnotationInput{
		ClipID: "clip-a", Round: 1, SpeciesLabel: "hidden-bird", StartMillis: 100,
		EndMillis: 1000, Confidence: 0.9, EvidenceNote: "仅第一轮可见", Revision: 1,
	}, "annotator", "ann-one", "submit-one", http.StatusOK)

	first := client.blindView("ann-one", 1)
	if !bytes.Contains(first, []byte("hidden-bird")) {
		t.Fatalf("第一轮提交者未读取到自己的结果: %s", first)
	}
	second := client.blindView("ann-two", 2)
	if bytes.Contains(second, []byte("hidden-bird")) || bytes.Contains(second, []byte(`"round":1`)) {
		t.Fatalf("第二轮标注员读取到第一轮缓存结果: %s", second)
	}

	client.write("/api/v1/review-batches/cache-batch/annotations", workflow.AnnotationInput{
		ClipID: "clip-a", Round: 2, SpeciesLabel: "second-bird", StartMillis: 120,
		EndMillis: 980, Confidence: 0.92, EvidenceNote: "第二轮独立证据", Revision: 1,
	}, "annotator", "ann-two", "submit-two", http.StatusOK)
	refreshed := client.blindView("ann-two", 2)
	if !bytes.Contains(refreshed, []byte("hidden-bird")) || !bytes.Contains(refreshed, []byte("second-bird")) {
		t.Fatalf("双轮提交完成后仍返回写入前的盲标缓存: %s", refreshed)
	}
}
