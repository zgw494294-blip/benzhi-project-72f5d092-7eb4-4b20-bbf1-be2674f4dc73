package httpapi

import (
	"net/http"
	"strconv"

	"acoustic-annotation-release/internal/domain"
	"acoustic-annotation-release/internal/workflow"
)

func (a *API) AddClipsHandler(writer http.ResponseWriter, request *http.Request) {
	batchID, err := identifier(request, "batchID")
	if err != nil {
		writeError(writer, request, err)
		return
	}
	var input workflow.BulkAddClipsInput
	if err = decodeJSON(writer, request, &input); err != nil {
		writeError(writer, request, err)
		return
	}
	if err = validateBulkCount(len(input.Clips), "clips"); err != nil {
		writeError(writer, request, err)
		return
	}
	context, err := parseWriteContext(request)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	result, err := a.workflow.AddClips(batchID, input, context)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (a *API) GetBlindTasksHandler(writer http.ResponseWriter, request *http.Request) {
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
	round, err := strconv.Atoi(request.URL.Query().Get("round"))
	if err != nil || round != 1 && round != 2 {
		writeError(writer, request, newBoundaryError("round 必须为 1 或 2"))
		return
	}
	status := request.URL.Query().Get("status")
	if status != "" && !domain.ValidBlindTaskStatus(status) {
		writeError(writer, request, newBoundaryError("status 不是支持的盲标待办状态"))
		return
	}
	tasks, err := a.workflow.GetBlindTasks(batchID, actorID, role, round, status)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"round": round, "tasks": tasks})
}

func (a *API) GetConflictTasksHandler(writer http.ResponseWriter, request *http.Request) {
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
	status := request.URL.Query().Get("status")
	reason, reasonAlias := request.URL.Query().Get("reasonCode"), request.URL.Query().Get("reason")
	if reason != "" && reasonAlias != "" && reason != reasonAlias {
		writeError(writer, request, newBoundaryError("reasonCode 与 reason 不能给出不同值"))
		return
	}
	if reason == "" {
		reason = reasonAlias
	}
	if status != "" && !domain.ValidConflictStatus(status) {
		writeError(writer, request, newBoundaryError("status 不是支持的冲突状态"))
		return
	}
	if reason != "" && !domain.ValidConflictReason(reason) {
		writeError(writer, request, newBoundaryError("reason 不是支持的冲突原因"))
		return
	}
	tasks, err := a.workflow.GetConflictTasks(batchID, role, status, reason)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"tasks": tasks})
}

func (a *API) AdjudicateConflictsHandler(writer http.ResponseWriter, request *http.Request) {
	batchID, err := identifier(request, "batchID")
	if err != nil {
		writeError(writer, request, err)
		return
	}
	var input workflow.BulkConflictDecisionsInput
	if err = decodeJSON(writer, request, &input); err != nil {
		writeError(writer, request, err)
		return
	}
	if err = validateBulkCount(len(input.Decisions), "decisions"); err != nil {
		writeError(writer, request, err)
		return
	}
	context, err := parseWriteContext(request)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	result, err := a.workflow.AdjudicateConflicts(batchID, input, context)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (a *API) RemediateGateHandler(writer http.ResponseWriter, request *http.Request) {
	batchID, err := identifier(request, "batchID")
	if err != nil {
		writeError(writer, request, err)
		return
	}
	var input workflow.GateRemediationsInput
	if err = decodeJSON(writer, request, &input); err != nil {
		writeError(writer, request, err)
		return
	}
	if err = validateBulkCount(len(input.Remediations), "remediations"); err != nil {
		writeError(writer, request, err)
		return
	}
	context, err := parseWriteContext(request)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	result, err := a.workflow.RemediateGate(batchID, input, context)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func validateBulkCount(count int, field string) error {
	if count < 1 || count > domain.MaxBulkItems {
		return newBoundaryError(field + " 数量必须在 1 到 " + strconv.Itoa(domain.MaxBulkItems) + " 之间")
	}
	return nil
}
