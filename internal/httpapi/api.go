package httpapi

import (
	"log/slog"
	"net/http"
	"strings"

	"acoustic-annotation-release/internal/workflow"
)

type API struct {
	workflow *workflow.Service
	logger   *slog.Logger
	mux      *http.ServeMux
}

func New(service *workflow.Service, logger *slog.Logger) *API {
	if logger == nil {
		logger = slog.Default()
	}
	api := &API{workflow: service, logger: logger, mux: http.NewServeMux()}
	api.routes()
	return api
}

func (a *API) Handler() http.Handler { return a.middleware(a.mux) }

func (a *API) routes() {
	a.mux.HandleFunc("GET /healthz", a.HealthHandler)
	a.mux.HandleFunc("POST /api/v1/review-batches", a.CreateBatchHandler)
	a.mux.HandleFunc("GET /api/v1/review-batches/{batchID}", a.GetBatchHandler)
	a.mux.HandleFunc("POST /api/v1/review-batches/{batchID}/clips", a.AddClipHandler)
	a.mux.HandleFunc("POST /api/v1/review-batches/{batchID}/clips/bulk", a.AddClipsHandler)
	a.mux.HandleFunc("POST /api/v1/review-batches/{batchID}/start-annotation", a.StartAnnotationHandler)
	a.mux.HandleFunc("POST /api/v1/review-batches/{batchID}/annotations", a.SubmitAnnotationHandler)
	a.mux.HandleFunc("GET /api/v1/review-batches/{batchID}/annotation-tasks", a.GetBlindTasksHandler)
	a.mux.HandleFunc("GET /api/v1/review-batches/{batchID}/clips/{clipID}/annotations", a.GetBlindAnnotationsHandler)
	a.mux.HandleFunc("PATCH /api/v1/review-batches/{batchID}/clips/{clipID}/privacy", a.UpdatePrivacyHandler)
	a.mux.HandleFunc("GET /api/v1/review-batches/{batchID}/conflicts", a.GetConflictTasksHandler)
	a.mux.HandleFunc("POST /api/v1/review-batches/{batchID}/conflicts/decisions", a.AdjudicateConflictsHandler)
	a.mux.HandleFunc("POST /api/v1/review-batches/{batchID}/conflicts/{conflictID}/decisions", a.ResolveConflictHandler)
	a.mux.HandleFunc("GET /api/v1/review-batches/{batchID}/release-gate", a.CheckGateHandler)
	a.mux.HandleFunc("POST /api/v1/review-batches/{batchID}/release-gate/remediations", a.RemediateGateHandler)
	a.mux.HandleFunc("POST /api/v1/review-batches/{batchID}/freeze", a.FreezeHandler)
	a.mux.HandleFunc("POST /api/v1/review-batches/{batchID}/credentials", a.IssueCredentialHandler)
	a.mux.HandleFunc("GET /api/v1/credentials/verify", a.VerifyCredentialsHandler)
	a.mux.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		writeAPIError(writer, request, http.StatusNotFound, "route_not_found", "路由不存在", nil)
	})
}

func identifier(request *http.Request, name string) (string, error) {
	value := strings.TrimSpace(request.PathValue(name))
	if value == "" || len(value) > 128 {
		return "", newBoundaryError("路径标识无效")
	}
	for _, character := range value {
		if !isIdentifierRune(character) {
			return "", newBoundaryError("路径标识只能包含字母、数字、点、下划线和连字符")
		}
	}
	return value, nil
}

func isIdentifierRune(value rune) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value == '-' || value == '_' || value == '.'
}
