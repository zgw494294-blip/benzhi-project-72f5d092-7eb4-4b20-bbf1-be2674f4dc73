package httpapi

import (
	"net/http"

	"acoustic-annotation-release/internal/workflow"
)

func (a *API) HealthHandler(writer http.ResponseWriter, request *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{"status": "ok"})
}

func (a *API) CreateBatchHandler(writer http.ResponseWriter, request *http.Request) {
	var input workflow.CreateBatchInput
	if err := decodeJSON(writer, request, &input); err != nil {
		writeError(writer, request, err)
		return
	}
	context, err := parseWriteContext(request)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	batch, replay, err := a.workflow.CreateBatch(input, context)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	status := http.StatusCreated
	if replay {
		status = http.StatusOK
	}
	writeJSON(writer, status, map[string]any{"batch": batch, "idempotentReplay": replay})
}

func (a *API) GetBatchHandler(writer http.ResponseWriter, request *http.Request) {
	batchID, err := identifier(request, "batchID")
	if err != nil {
		writeError(writer, request, err)
		return
	}
	actorID, role, err := readIdentity(request)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	view, err := a.workflow.GetBatch(batchID, actorID, role)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, view)
}

func (a *API) AddClipHandler(writer http.ResponseWriter, request *http.Request) {
	batchID, err := identifier(request, "batchID")
	if err != nil {
		writeError(writer, request, err)
		return
	}
	var input workflow.AddClipInput
	if err = decodeJSON(writer, request, &input); err != nil {
		writeError(writer, request, err)
		return
	}
	context, err := parseWriteContext(request)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	batch, replay, err := a.workflow.AddClip(batchID, input, context)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"batch": batch, "idempotentReplay": replay})
}

func (a *API) StartAnnotationHandler(writer http.ResponseWriter, request *http.Request) {
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
	batch, replay, err := a.workflow.StartAnnotation(batchID, context)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"batch": batch, "idempotentReplay": replay})
}

func (a *API) UpdatePrivacyHandler(writer http.ResponseWriter, request *http.Request) {
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
	var input workflow.PrivacyInput
	if err = decodeJSON(writer, request, &input); err != nil {
		writeError(writer, request, err)
		return
	}
	context, err := parseWriteContext(request)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	batch, replay, err := a.workflow.UpdatePrivacy(batchID, clipID, input, context)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"batch": batch, "idempotentReplay": replay})
}
