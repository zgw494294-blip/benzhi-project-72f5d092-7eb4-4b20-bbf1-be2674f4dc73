package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type selfcheckClient struct {
	baseURL string
	client  *http.Client
	version int64
}

func runSelfcheck(configuration config, logger interface{ Info(string, ...any) }) error {
	temporaryDirectory, err := os.MkdirTemp("", "acoustic-release-selfcheck-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporaryDirectory)
	configuration.dataDirectory = filepath.Join(temporaryDirectory, "store")
	application, err := buildApplication(configuration, defaultLogger())
	if err != nil {
		return err
	}
	errorsChannel := make(chan error, 1)
	go application.serve(errorsChannel)
	context, cancel := context.WithTimeout(context.Background(), configuration.selfcheckTimeout)
	defer cancel()
	client := &selfcheckClient{baseURL: "http://" + application.listener.Addr().String(), client: &http.Client{Timeout: minDuration(configuration.selfcheckTimeout, 5*time.Second)}}
	flowError := client.completeFlow(context)
	shutdownContext, shutdownCancel := contextWithMaximum(context, 2*time.Second)
	shutdownError := application.shutdown(shutdownContext)
	shutdownCancel()
	serveError := <-errorsChannel
	if flowError != nil {
		return flowError
	}
	if shutdownError != nil {
		return shutdownError
	}
	if serveError != nil {
		return serveError
	}
	logger.Info("selfcheck 完成", "address", configuration.address)
	return nil
}

func (c *selfcheckClient) completeFlow(context context.Context) error {
	start := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	end := start.Add(2 * time.Hour)
	batchID := "selfcheck-batch"
	create := map[string]any{"batchID": batchID, "surveySite": "自检湿地", "captureWindowStart": start, "captureWindowEnd": end, "authorizationStatement": "自检授权声明"}
	if err := c.write(context, http.MethodPost, "/api/v1/review-batches", create, "administrator", "selfcheck-admin", "create", 0); err != nil {
		return err
	}
	clips := map[string]any{"clips": []map[string]any{{"clipID": "clip-001", "sourceName": "selfcheck.wav", "durationMillis": 10000, "contentDigest": "sha256:selfcheck-content", "captureTimestamp": start.Add(30 * time.Minute), "authorizationConfirmed": false, "humanVoiceDetected": true, "redactionNote": ""}}}
	if err := c.write(context, http.MethodPost, "/api/v1/review-batches/"+batchID+"/clips/bulk", clips, "administrator", "selfcheck-admin", "clips", c.version); err != nil {
		return err
	}
	if err := c.write(context, http.MethodPost, "/api/v1/review-batches/"+batchID+"/start-annotation", nil, "administrator", "selfcheck-admin", "start", c.version); err != nil {
		return err
	}
	if err := c.readAndRequire(context, "/api/v1/review-batches/"+batchID+"/annotation-tasks?round=1&status=pending", "annotator", "annotator-one", "\"status\":\"pending\""); err != nil {
		return err
	}
	first := map[string]any{"clipID": "clip-001", "round": 1, "speciesLabel": "Anas platyrhynchos", "startMillis": 1000, "endMillis": 4000, "confidence": 0.94, "evidenceNote": "连续鸣叫", "revision": 1}
	if err := c.write(context, http.MethodPost, "/api/v1/review-batches/"+batchID+"/annotations", first, "annotator", "annotator-one", "annotation-1", c.version); err != nil {
		return err
	}
	second := map[string]any{"clipID": "clip-001", "round": 2, "speciesLabel": "ardea cinerea", "startMillis": 1100, "endMillis": 3900, "confidence": 0.91, "evidenceNote": "频谱显示另一物种", "revision": 1}
	if err := c.write(context, http.MethodPost, "/api/v1/review-batches/"+batchID+"/annotations", second, "annotator", "annotator-two", "annotation-2", c.version); err != nil {
		return err
	}
	if err := c.readAndRequire(context, "/api/v1/review-batches/"+batchID+"/conflicts?status=open&reasonCode=label_mismatch", "reviewer", "reviewer-one", "\"reasonCode\":\"label_mismatch\""); err != nil {
		return err
	}
	decisions := map[string]any{"decisions": []map[string]any{{"conflictID": "conflict-clip-001", "decision": "merge", "resolvedLabel": "anas platyrhynchos", "resolutionNote": "自检合并裁决"}}}
	if err := c.write(context, http.MethodPost, "/api/v1/review-batches/"+batchID+"/conflicts/decisions", decisions, "reviewer", "reviewer-one", "decisions", c.version); err != nil {
		return err
	}
	remediations := map[string]any{"remediations": []map[string]any{{"clipID": "clip-001", "authorizationConfirmed": true, "humanVoiceDetected": true, "redactionNote": "已静音敏感人声"}}}
	if err := c.write(context, http.MethodPost, "/api/v1/review-batches/"+batchID+"/release-gate/remediations", remediations, "reviewer", "reviewer-one", "remediations", c.version); err != nil {
		return err
	}
	if err := c.readAndRequire(context, "/api/v1/review-batches/"+batchID+"/release-gate", "reviewer", "reviewer-one", "\"passed\":true"); err != nil {
		return err
	}
	if err := c.write(context, http.MethodPost, "/api/v1/review-batches/"+batchID+"/freeze", nil, "reviewer", "reviewer-one", "freeze", c.version); err != nil {
		return err
	}
	if err := c.write(context, http.MethodPost, "/api/v1/review-batches/"+batchID+"/credentials", map[string]any{"issuerID": "reviewer-one"}, "reviewer", "reviewer-one", "issue", c.version); err != nil {
		return err
	}
	if err := c.readAndRequire(context, "/api/v1/review-batches/"+batchID, "reviewer", "reviewer-one", "\"status\":\"released\""); err != nil {
		return err
	}
	return c.readAndRequire(context, "/api/v1/credentials/verify", "reviewer", "reviewer-one", "\"valid\":true")
}

func (c *selfcheckClient) write(context context.Context, method, path string, body any, role, actor, key string, version int64) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(context, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("X-Role", role)
	request.Header.Set("X-Actor-ID", actor)
	request.Header.Set("Idempotency-Key", key)
	request.Header.Set("If-Match-Version", fmt.Sprintf("%d", version))
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("selfcheck %s %s 返回 %d: %s", method, path, response.StatusCode, data)
	}
	var envelope struct {
		Batch struct {
			Version int64 `json:"version"`
		} `json:"batch"`
	}
	if err = json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("selfcheck 响应不是 JSON: %w", err)
	}
	if envelope.Batch.Version == 0 {
		return fmt.Errorf("selfcheck 写响应缺少批次版本: %s", data)
	}
	c.version = envelope.Batch.Version
	return nil
}

func (c *selfcheckClient) readAndRequire(context context.Context, path, role, actor, expected string) error {
	request, err := http.NewRequestWithContext(context, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	request.Header.Set("X-Role", role)
	request.Header.Set("X-Actor-ID", actor)
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK || !bytes.Contains(data, []byte(expected)) {
		return fmt.Errorf("selfcheck 查询 %s 未满足 %s: %d %s", path, expected, response.StatusCode, data)
	}
	return nil
}

func contextWithMaximum(parent context.Context, maximum time.Duration) (context.Context, context.CancelFunc) {
	if deadline, ok := parent.Deadline(); ok && time.Until(deadline) > 0 {
		return context.WithTimeout(context.Background(), minDuration(time.Until(deadline), maximum))
	}
	return context.WithTimeout(context.Background(), maximum)
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
