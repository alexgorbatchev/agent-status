package api

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/alexgorbatchev/agent-status/packages/agent-server/internal/store"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleGetAgentUsage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if decoded, err := url.PathUnescape(id); err == nil {
		id = decoded
	}
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing agent id")
		return
	}
	report, err := s.store.GetAgentUsage(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "agent not found")
			return
		}
		s.logger.ErrorContext(r.Context(), "agent_usage_fetch_failed", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to get agent usage")
		return
	}
	writeJSON(w, http.StatusOK, report)
}
