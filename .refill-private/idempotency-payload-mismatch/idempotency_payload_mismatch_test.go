package idempotencypayloadmismatch_test

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"acoustic-annotation-release/internal/certificate"
	"acoustic-annotation-release/internal/httpapi"
	"acoustic-annotation-release/internal/persistence"
	"acoustic-annotation-release/internal/workflow"
)

func TestIdempotencyKeyRejectsChangedPayload(t *testing.T) {
	repository, err := persistence.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(httpapi.New(workflow.NewService(repository, certificate.NewService()), logger).Handler())
	defer server.Close()

	start := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	create := `{"batchID":"payload-batch","surveySite":"湿地","captureWindowStart":"2026-08-25T00:00:00Z","captureWindowEnd":"2026-08-25T01:00:00Z","authorizationStatement":"授权"}`
	response := write(t, server.URL+"/api/v1/review-batches", create, "create", "0")
	if response.StatusCode != http.StatusCreated {
		data, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("创建批次失败: status=%d body=%s", response.StatusCode, data)
	}
	response.Body.Close()

	first := `{"clipID":"clip-a","sourceName":"a.wav","durationMillis":1000,"contentDigest":"digest-a","captureTimestamp":"` + start.Add(10*time.Minute).Format(time.RFC3339) + `","authorizationConfirmed":true}`
	response = write(t, server.URL+"/api/v1/review-batches/payload-batch/clips", first, "reused-key", "1")
	if response.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("首次登记失败: status=%d body=%s", response.StatusCode, data)
	}
	response.Body.Close()

	changed := `{"clipID":"clip-b","sourceName":"b.wav","durationMillis":1000,"contentDigest":"digest-b","captureTimestamp":"` + start.Add(20*time.Minute).Format(time.RFC3339) + `","authorizationConfirmed":true}`
	response = write(t, server.URL+"/api/v1/review-batches/payload-batch/clips", changed, "reused-key", "1")
	data, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		t.Fatalf("同一 Idempotency-Key 携带不同请求内容仍被当作安全重试: status=%d body=%s", response.StatusCode, data)
	}
}

func write(t *testing.T, url, body, key, version string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Actor-ID", "admin")
	request.Header.Set("X-Role", "administrator")
	request.Header.Set("Idempotency-Key", key)
	request.Header.Set("If-Match-Version", version)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
