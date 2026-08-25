package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

type contextKey string

const requestIDKey contextKey = "request-id"

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
func (r *statusRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.WriteHeader(http.StatusOK)
	}
	count, err := r.ResponseWriter.Write(data)
	r.bytes += count
	return count, err
}

func (a *API) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started := time.Now()
		requestID := request.Header.Get("X-Request-ID")
		if requestID == "" || len(requestID) > 128 {
			requestID = generatedRequestID()
		}
		writer.Header().Set("X-Request-ID", requestID)
		request = request.WithContext(context.WithValue(request.Context(), requestIDKey, requestID))
		recorder := &statusRecorder{ResponseWriter: writer}
		defer func() {
			if recovered := recover(); recovered != nil {
				a.logger.Error("HTTP panic 已恢复", slog.String("requestID", requestID), slog.Any("panic", recovered), slog.String("stack", string(debug.Stack())))
				if recorder.status == 0 {
					writeAPIError(recorder, request, http.StatusInternalServerError, "internal_error", "服务器内部错误", nil)
				}
			}
			status := recorder.status
			if status == 0 {
				status = http.StatusOK
			}
			a.logger.Info("HTTP access", slog.String("requestID", requestID), slog.String("method", request.Method), slog.String("path", request.URL.Path), slog.Int("status", status), slog.Int("bytes", recorder.bytes), slog.Duration("duration", time.Since(started)))
		}()
		next.ServeHTTP(recorder, request)
	})
}

func generatedRequestID() string {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return "request-fallback"
	}
	return hex.EncodeToString(bytes)
}
func requestIDFrom(context context.Context) string {
	value, _ := context.Value(requestIDKey).(string)
	return value
}
