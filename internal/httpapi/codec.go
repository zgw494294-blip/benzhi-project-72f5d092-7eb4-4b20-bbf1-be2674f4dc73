package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"acoustic-annotation-release/internal/domain"
	"acoustic-annotation-release/internal/workflow"
)

const maxRequestBody = 1 << 20

type boundaryError struct{ message string }

func (e *boundaryError) Error() string      { return e.message }
func newBoundaryError(message string) error { return &boundaryError{message: message} }

type errorEnvelope struct {
	Error     apiError `json:"error"`
	RequestID string   `json:"requestID"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func decodeJSON(writer http.ResponseWriter, request *http.Request, target any) error {
	contentType := request.Header.Get("Content-Type")
	if contentType != "application/json" && !strings.HasPrefix(contentType, "application/json;") {
		return newBoundaryError("Content-Type 必须为 application/json")
	}
	request.Body = http.MaxBytesReader(writer, request.Body, maxRequestBody)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var syntax *json.SyntaxError
		if errors.As(err, &syntax) {
			return newBoundaryError(fmt.Sprintf("JSON 语法错误，位置 %d", syntax.Offset))
		}
		if errors.Is(err, io.EOF) {
			return newBoundaryError("请求正文不能为空")
		}
		return newBoundaryError("JSON 请求无效: " + err.Error())
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return newBoundaryError("请求正文只能包含一个 JSON 对象")
	}
	return nil
}

func parseWriteContext(request *http.Request) (workflow.WriteContext, error) {
	actorID, role := strings.TrimSpace(request.Header.Get("X-Actor-ID")), strings.TrimSpace(request.Header.Get("X-Role"))
	key, rawVersion := strings.TrimSpace(request.Header.Get("Idempotency-Key")), strings.TrimSpace(request.Header.Get("If-Match-Version"))
	if rawVersion == "" {
		return workflow.WriteContext{}, newBoundaryError("缺少 If-Match-Version")
	}
	version, err := strconv.ParseInt(rawVersion, 10, 64)
	if err != nil || version < 0 {
		return workflow.WriteContext{}, newBoundaryError("If-Match-Version 必须是非负整数")
	}
	context := workflow.WriteContext{ActorID: actorID, Role: role, ExpectedVersion: version, IdempotencyKey: key}
	return context, nil
}

func readIdentity(request *http.Request) (string, string, error) {
	actorID, role := strings.TrimSpace(request.Header.Get("X-Actor-ID")), strings.TrimSpace(request.Header.Get("X-Role"))
	if actorID == "" || role == "" {
		return "", "", newBoundaryError("缺少 X-Actor-ID 或 X-Role")
	}
	return actorID, role, nil
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeError(writer http.ResponseWriter, request *http.Request, err error) {
	if boundary, ok := err.(*boundaryError); ok {
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_request", boundary.message, nil)
		return
	}
	code := domain.ErrorCodeOf(err)
	status := http.StatusInternalServerError
	switch code {
	case domain.CodeInvalid:
		status = http.StatusBadRequest
	case domain.CodeNotFound:
		status = http.StatusNotFound
	case domain.CodeConflict:
		status = http.StatusConflict
	case domain.CodeForbidden:
		status = http.StatusForbidden
	case domain.CodeInvalidState, domain.CodeFrozen, domain.CodeGateBlocked:
		status = http.StatusUnprocessableEntity
	}
	message := "服务器内部错误"
	var details any
	if typed, ok := err.(*domain.Error); ok {
		message = typed.Message
		details = typed.Details
	}
	writeAPIError(writer, request, status, string(code), message, details)
}

func writeAPIError(writer http.ResponseWriter, request *http.Request, status int, code, message string, details any) {
	writeJSON(writer, status, errorEnvelope{Error: apiError{Code: code, Message: message, Details: details}, RequestID: requestIDFrom(request.Context())})
}
