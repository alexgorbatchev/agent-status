package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/usage"
)

func (m *mockStore) GetAgentUsage(ctx context.Context, id string) (*usage.Report, error) {
	a, err := m.GetAgentByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if m.logsErr != nil {
		return nil, m.logsErr
	}
	r := usage.NewReport(a.ID, a.Harness)
	r.Skills = append(r.Skills, usage.Entry{Name: "review", Operation: "invoke", Count: 2, FirstUsedAt: 1000, LastUsedAt: 2000})
	return r, nil
}

func TestAgentUsageEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name, id, token string
		fail            bool
		status          int
	}{
		{"usage", "agent-1", "secret-token", false, http.StatusOK},
		{"encoded composite ID", "885d9dd12b6d62a9%3Aclaude-code%3Abe20b445", "secret-token", false, http.StatusOK},
		{"missing agent", "missing", "secret-token", false, http.StatusNotFound},
		{"unauthorized", "agent-1", "", false, http.StatusUnauthorized},
		{"store failure", "agent-1", "secret-token", true, http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, mock, hub := setupTestServer(t)
			defer hub.Close()
			if tc.fail {
				mock.logsErr = errors.New("private storage failure")
			}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+tc.id+"/usage", nil)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			w := httptest.NewRecorder()
			srv.Router().ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body)
			}
			if strings.Contains(w.Body.String(), "private storage failure") {
				t.Fatal("server leaked storage errors")
			}
			if w.Code == http.StatusOK {
				var r usage.Report
				if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
					t.Fatal(err)
				}
				if len(r.Skills) != 1 || r.Skills[0].Count != 2 || r.Coverage["commands"].Status != "partial" {
					t.Fatalf("bad usage: %+v", r)
				}
			}
		})
	}
}
