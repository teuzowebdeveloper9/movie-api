package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func writeJSON(w http.ResponseWriter, statusCode int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, statusCode int, code, message string) {
	writeJSON(w, statusCode, ErrorResponse{Error: ErrorDetail{Code: code, Message: message}})
}

func writeGRPCError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	st, ok := status.FromError(err)
	if !ok {
		logger.ErrorContext(r.Context(), "unexpected non-grpc error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	switch st.Code() {
	case codes.NotFound:
		writeError(w, http.StatusNotFound, "not_found", st.Message())
	case codes.InvalidArgument:
		writeError(w, http.StatusBadRequest, "invalid_request", st.Message())
	case codes.AlreadyExists:
		writeError(w, http.StatusConflict, "conflict", st.Message())
	case codes.DeadlineExceeded:
		logger.ErrorContext(r.Context(), "movies service timed out", "error", err)
		writeError(w, http.StatusGatewayTimeout, "upstream_timeout", "the movies service took too long to respond")
	case codes.Unavailable:
		logger.ErrorContext(r.Context(), "movies service unavailable", "error", err)
		writeError(w, http.StatusServiceUnavailable, "upstream_unavailable", "the movies service is temporarily unavailable")
	default:
		logger.ErrorContext(r.Context(), "movies service call failed", "code", st.Code().String(), "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
	}
}
