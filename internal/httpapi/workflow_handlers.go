package httpapi

import (
	"net/http"
	"strconv"

	"acoustic-annotation-release/internal/workflow"
)

func (a *API) SubmitAnnotationHandler(writer http.ResponseWriter, request *http.Request) {
	batchID, err := identifier(request, "batchID")
	if err != nil {
		writeError(writer, request, err)
		return
	}
	var input workflow.AnnotationInput
	if err = decodeJSON(writer, request, &input); err != nil {
		writeError(writer, request, err)
		return
	}
	context, err := parseWriteContext(request)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	batch, conflict, replay, err := a.workflow.SubmitAnnotation(batchID, input, context)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"batch": batch, "conflict": conflict, "idempotentReplay": replay})
}

func (a *API) GetBlindAnnotationsHandler(writer http.ResponseWriter, request *http.Request) {
	batchID, err := identifier(request, "batchID")
	if err != nil {
		writeError(writer, request, err)
		return
	}
	clipID, err := identifier(request, "clipID")
	if err != nil {
		writeError(writer, request, err)
		return
	}
	actorID, role, err := readIdentity(request)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	round, err := strconv.Atoi(request.URL.Query().Get("round"))
	if err != nil || round < 1 || round > 2 {
		writeError(writer, request, newBoundaryError("round 必须为 1 或 2"))
		return
	}
	items, err := a.workflow.GetBlindSubmissions(batchID, clipID, actorID, role, round)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"submissions": items})
}

func (a *API) ResolveConflictHandler(writer http.ResponseWriter, request *http.Request) {
	batchID, err := identifier(request, "batchID")
	if err != nil {
		writeError(writer, request, err)
		return
	}
	conflictID, err := identifier(request, "conflictID")
	if err != nil {
		writeError(writer, request, err)
		return
	}
	var input workflow.ResolveConflictInput
	if err = decodeJSON(writer, request, &input); err != nil {
		writeError(writer, request, err)
		return
	}
	context, err := parseWriteContext(request)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	batch, replay, err := a.workflow.ResolveConflict(batchID, conflictID, input, context)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"batch": batch, "idempotentReplay": replay})
}

func (a *API) CheckGateHandler(writer http.ResponseWriter, request *http.Request) {
	batchID, err := identifier(request, "batchID")
	if err != nil {
		writeError(writer, request, err)
		return
	}
	_, role, err := readIdentity(request)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	result, err := a.workflow.CheckGate(batchID, role)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (a *API) FreezeHandler(writer http.ResponseWriter, request *http.Request) {
	batchID, err := identifier(request, "batchID")
	if err != nil {
		writeError(writer, request, err)
		return
	}
	context, err := parseWriteContext(request)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	batch, replay, err := a.workflow.Freeze(batchID, context)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"batch": batch, "idempotentReplay": replay})
}

func (a *API) IssueCredentialHandler(writer http.ResponseWriter, request *http.Request) {
	batchID, err := identifier(request, "batchID")
	if err != nil {
		writeError(writer, request, err)
		return
	}
	var input workflow.IssueInput
	if err = decodeJSON(writer, request, &input); err != nil {
		writeError(writer, request, err)
		return
	}
	context, err := parseWriteContext(request)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	batch, replay, err := a.workflow.Issue(batchID, input, context)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusCreated, map[string]any{"batch": batch, "credential": batch.Credential, "idempotentReplay": replay})
}

func (a *API) VerifyCredentialsHandler(writer http.ResponseWriter, request *http.Request) {
	_, role, err := readIdentity(request)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	if role != workflow.RoleReviewer && role != workflow.RoleAdministrator {
		writeError(writer, request, newBoundaryError("仅管理员或复核人可校验凭据链"))
		return
	}
	writeJSON(writer, http.StatusOK, a.workflow.VerifyCredentials())
}
