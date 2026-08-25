package httpapi

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
	"acoustic-annotation-release/internal/persistence"
	"acoustic-annotation-release/internal/workflow"
)

type apiTestClient struct {
	t       *testing.T
	server  *httptest.Server
	version int64
}

func setupAPI(t *testing.T) (*apiTestClient, func()) {
	t.Helper()
	repository, err := persistence.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(New(workflow.NewService(repository, certificate.NewService()), logger).Handler())
	return &apiTestClient{t: t, server: server}, func() { server.Close(); repository.Close() }
}

func (c *apiTestClient) write(method, path, body, role, actor, key string, expectedStatus int) []byte {
	c.t.Helper()
	request, err := http.NewRequest(method, c.server.URL+path, bytes.NewBufferString(body))
	if err != nil {
		c.t.Fatal(err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
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
	if response.StatusCode != expectedStatus {
		c.t.Fatalf("%s %s 返回 %d，期望 %d: %s", method, path, response.StatusCode, expectedStatus, data)
	}
	if expectedStatus >= 200 && expectedStatus < 300 {
		var value struct {
			Batch struct {
				Version int64 `json:"version"`
			} `json:"batch"`
		}
		if err = json.Unmarshal(data, &value); err != nil {
			c.t.Fatal(err)
		}
		if value.Batch.Version > 0 {
			c.version = value.Batch.Version
		}
	}
	return data
}

func TestHTTPBlindAnnotationAndStrictJSON(t *testing.T) {
	client, cleanup := setupAPI(t)
	defer cleanup()
	start := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	create, _ := json.Marshal(workflow.CreateBatchInput{BatchID: "http-batch", SurveySite: "湿地", CaptureWindowStart: start, CaptureWindowEnd: start.Add(time.Hour), AuthorizationStatement: "授权"})
	client.write(http.MethodPost, "/api/v1/review-batches", string(create), "administrator", "admin", "create", http.StatusCreated)
	clip, _ := json.Marshal(workflow.AddClipInput{ClipID: "clip", SourceName: "clip.wav", DurationMillis: 5000, ContentDigest: "digest", CaptureTimestamp: start.Add(time.Minute), AuthorizationConfirmed: true})
	client.write(http.MethodPost, "/api/v1/review-batches/http-batch/clips", string(clip), "administrator", "admin", "clip", http.StatusOK)
	client.write(http.MethodPost, "/api/v1/review-batches/http-batch/start-annotation", "", "administrator", "admin", "start", http.StatusOK)
	annotation, _ := json.Marshal(workflow.AnnotationInput{ClipID: "clip", Round: 1, SpeciesLabel: "hidden-bird", StartMillis: 100, EndMillis: 1000, Confidence: .9, EvidenceNote: "证据", Revision: 1})
	client.write(http.MethodPost, "/api/v1/review-batches/http-batch/annotations", string(annotation), "annotator", "ann-one", "ann-1", http.StatusOK)
	request, _ := http.NewRequest(http.MethodGet, client.server.URL+"/api/v1/review-batches/http-batch/clips/clip/annotations?round=2", nil)
	request.Header.Set("X-Role", "annotator")
	request.Header.Set("X-Actor-ID", "ann-two")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || bytes.Contains(data, []byte("hidden-bird")) {
		t.Fatalf("未完成双轮时泄露另一轮: %d %s", response.StatusCode, data)
	}
	bad := `{"clipID":"x","unknown":true}`
	client.write(http.MethodPost, "/api/v1/review-batches/http-batch/annotations", bad, "annotator", "ann-two", "bad", http.StatusBadRequest)
}

func TestHTTPRequiresConcurrencyAndIdentityHeaders(t *testing.T) {
	client, cleanup := setupAPI(t)
	defer cleanup()
	request, _ := http.NewRequest(http.MethodPost, client.server.URL+"/api/v1/review-batches", bytes.NewBufferString(`{}`))
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("缺少并发头应返回 400，得到 %d", response.StatusCode)
	}
}

func TestHTTPBulkClipsStructuredErrorAndIdempotentReplay(t *testing.T) {
	client, cleanup := setupAPI(t)
	defer cleanup()
	start := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	create, _ := json.Marshal(workflow.CreateBatchInput{BatchID: "bulk-http", SurveySite: "湿地", CaptureWindowStart: start, CaptureWindowEnd: start.Add(time.Hour), AuthorizationStatement: "授权"})
	client.write(http.MethodPost, "/api/v1/review-batches", string(create), "administrator", "admin", "create", http.StatusCreated)
	bulk := `{"clips":[{"clipID":"clip-z","sourceName":"z.wav","durationMillis":1000,"contentDigest":"Z","captureTimestamp":"2026-08-25T00:10:00Z"},{"clipID":"clip-a","sourceName":"a.wav","durationMillis":1000,"contentDigest":"A","captureTimestamp":"2026-08-25T00:11:00Z"}]}`
	first := client.write(http.MethodPost, "/api/v1/review-batches/bulk-http/clips/bulk", bulk, "administrator", "admin", "bulk", http.StatusOK)
	replay := client.write(http.MethodPost, "/api/v1/review-batches/bulk-http/clips/bulk", bulk, "administrator", "admin", "bulk", http.StatusOK)
	if !bytes.Contains(first, []byte(`"clipID":"clip-a"`)) || bytes.Index(first, []byte(`"clipID":"clip-a"`)) > bytes.Index(first, []byte(`"clipID":"clip-z"`)) || !bytes.Contains(replay, []byte(`"idempotentReplay":true`)) {
		t.Fatalf("批量结果未稳定排序或未幂等重放: first=%s replay=%s", first, replay)
	}
	bad := `{"clips":[{"clipID":"clip-b","sourceName":"b.wav","durationMillis":1000,"contentDigest":"B","captureTimestamp":"2026-08-25T00:12:00Z"},{"clipID":"clip-c","sourceName":"c.wav","durationMillis":1000,"contentDigest":" a ","captureTimestamp":"2026-08-25T00:13:00Z"}]}`
	errorBody := client.write(http.MethodPost, "/api/v1/review-batches/bulk-http/clips/bulk", bad, "administrator", "admin", "bad-bulk", http.StatusBadRequest)
	if !bytes.Contains(errorBody, []byte(`"index":1`)) || !bytes.Contains(errorBody, []byte(`"clipID":"clip-c"`)) || !bytes.Contains(errorBody, []byte(`"reasonCode":"duplicate_content_digest"`)) {
		t.Fatalf("重复摘要错误不可定位: %s", errorBody)
	}
	request, _ := http.NewRequest(http.MethodGet, client.server.URL+"/api/v1/review-batches/bulk-http", nil)
	request.Header.Set("X-Role", "administrator")
	request.Header.Set("X-Actor-ID", "admin")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || bytes.Contains(data, []byte(`"clip-b"`)) || !bytes.Contains(data, []byte(`"version":2`)) {
		t.Fatalf("失败批次产生部分写入或版本变化: %d %s", response.StatusCode, data)
	}
}
