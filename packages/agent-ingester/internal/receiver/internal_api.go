package receiver

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/store"
)

// InternalHandler authenticates the internal agent API before routing requests.
func (r *Receiver) InternalHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /api/v1/internal/agents/{id}", r.HandlePatchAgent)
	mux.HandleFunc("POST /api/v1/internal/agents/{id}/focus", r.HandleFocusAgent)
	return r.withBearerAuth(mux)
}

func (r *Receiver) authorized(req *http.Request) bool {
	token, ok := strings.CutPrefix(req.Header.Get("Authorization"), "Bearer ")
	return ok && subtle.ConstantTimeCompare([]byte(token), []byte(r.bearerToken)) == 1
}

func (r *Receiver) withBearerAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if r.bearerToken != "" && !r.authorized(req) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, req)
	})
}

func writeAgentError(w http.ResponseWriter, err error) {
	status, message := http.StatusInternalServerError, "agent operation failed"
	if errors.Is(err, store.ErrAgentNotFound) {
		status, message = http.StatusNotFound, "agent not found"
	} else {
		logging.Error("agent_operation_failed", "err", err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
