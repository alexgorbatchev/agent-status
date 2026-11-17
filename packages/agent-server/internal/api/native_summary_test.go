package api

import (
	"encoding/json"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"net/http/httptest"
	"testing"
)

func TestAgentEndpointsExposeNativeSummary(t *testing.T) {
	srv, mock, _ := setupTestServer(t)
	text := "We're finishing the Yarn-to-Bun migration PR and its review fixes"
	mock.agents[0].Activity.Summary = &text
	mock.agents[0].Activity.SummaryUpdatedAt = new(int64(1791589631636))
	mock.agents[0].Activity.Action = "Running tests"
	for _, path := range []string{"/api/v1/agents/agent-1", "/api/v1/agents"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			req.Header.Set("Authorization", "Bearer secret-token")
			rec := httptest.NewRecorder()
			srv.Router().ServeHTTP(rec, req)
			if rec.Code != 200 {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			var agent types.IAgentState
			if path == "/api/v1/agents" {
				var agents []types.IAgentState
				if err := json.Unmarshal(rec.Body.Bytes(), &agents); err != nil {
					t.Fatal(err)
				}
				if len(agents) == 0 {
					t.Fatal("no agents")
				}
				agent = agents[0]
			} else if err := json.Unmarshal(rec.Body.Bytes(), &agent); err != nil {
				t.Fatal(err)
			}
			if agent.Activity.Summary == nil || *agent.Activity.Summary != text || *agent.Activity.SummaryUpdatedAt != 1791589631636 || agent.Activity.Action != "Running tests" {
				t.Fatalf("lost native recap: %+v", agent.Activity)
			}
		})
	}
}
