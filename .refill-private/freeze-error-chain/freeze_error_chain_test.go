package freeze_error_chain_test

import (
	"bytes"
	"encoding/json"
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

func TestFreezeWrappedDomainErrorPreservesHTTPContract(t *testing.T) {
	repository, err := persistence.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(httpapi.New(workflow.NewService(repository, certificate.NewService()), logger).Handler())
	defer server.Close()

	start := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	createBody, err := json.Marshal(workflow.CreateBatchInput{
		BatchID:                "blocked-freeze",
		SurveySite:             "湖区",
		CaptureWindowStart:     start,
		CaptureWindowEnd:       start.Add(time.Hour),
		AuthorizationStatement: "已取得调查授权",
	})
	if err != nil {
		t.Fatal(err)
	}
	create := newWriteRequest(t, server.URL+"/api/v1/review-batches", createBody, "administrator", "admin", "create", "0")
	createResponse, err := http.DefaultClient.Do(create)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, createResponse.Body)
	createResponse.Body.Close()
	if createResponse.StatusCode != http.StatusCreated {
		t.Fatalf("创建批次返回 %d", createResponse.StatusCode)
	}

	freeze := newWriteRequest(t, server.URL+"/api/v1/review-batches/blocked-freeze/freeze", nil, "reviewer", "reviewer-a", "freeze", "1")
	freezeResponse, err := http.DefaultClient.Do(freeze)
	if err != nil {
		t.Fatal(err)
	}
	defer freezeResponse.Body.Close()
	responseBody, err := io.ReadAll(freezeResponse.Body)
	if err != nil {
		t.Fatal(err)
	}
	if freezeResponse.StatusCode != http.StatusUnprocessableEntity || !bytes.Contains(responseBody, []byte(`"code":"release_gate_blocked"`)) {
		t.Fatalf("包装后的门禁错误丢失 HTTP 契约: status=%d body=%s", freezeResponse.StatusCode, responseBody)
	}
}

func newWriteRequest(t *testing.T, url string, body []byte, role, actor, key, version string) *http.Request {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("X-Role", role)
	request.Header.Set("X-Actor-ID", actor)
	request.Header.Set("Idempotency-Key", key)
	request.Header.Set("If-Match-Version", version)
	return request
}
