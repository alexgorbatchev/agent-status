package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/alexgorbatchev/agent-status/packages/agent-server/internal/store"
	"github.com/alexgorbatchev/agent-status/packages/agent-server/internal/stream"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

type mockStore struct {
	mu              sync.Mutex
	status          *types.StatusResponse
	statusErr       error
	projects        []types.ProjectGroup
	projectsErr     error
	environments    []types.IEnvironmentInfo
	environmentsErr error
	agents          []types.IAgentState
	agentsErr       error
	agentMap        map[string]*types.IAgentState
	agentErr        error
	lastRequestedID string
	logs            []types.ITelemetryFrame
	logsErr         error
	lastLogsLimit   int
	lastLogsBefore  int64
	lastLogsSub     string
}

func (m *mockStore) GetStatus(ctx context.Context) (*types.StatusResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.statusErr != nil {
		return nil, m.statusErr
	}
	if m.status != nil {
		return m.status, nil
	}
	return &types.StatusResponse{Status: "ok"}, nil
}

func (m *mockStore) GetProjects(ctx context.Context) ([]types.ProjectGroup, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.projectsErr != nil {
		return nil, m.projectsErr
	}
	return m.projects, nil
}

func (m *mockStore) GetEnvironments(ctx context.Context) ([]types.IEnvironmentInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.environmentsErr != nil {
		return nil, m.environmentsErr
	}
	return m.environments, nil
}

func (m *mockStore) GetAgents(ctx context.Context, filter store.AgentFilter) ([]types.IAgentState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.agentsErr != nil {
		return nil, m.agentsErr
	}
	var filtered []types.IAgentState
	for _, a := range m.agents {
		if filter.Project != "" && a.Project.ID != filter.Project {
			continue
		}
		if filter.Environment != "" && a.Environment.ID != filter.Environment {
			continue
		}
		if filter.Lifecycle != "" && a.Activity.Lifecycle != filter.Lifecycle {
			continue
		}
		if filter.Activity != "" && a.Activity.Activity != filter.Activity {
			continue
		}
		filtered = append(filtered, a)
	}
	return filtered, nil
}

func (m *mockStore) GetAgentByID(ctx context.Context, id string) (*types.IAgentState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastRequestedID = id
	if m.agentErr != nil {
		return nil, m.agentErr
	}
	if a, ok := m.agentMap[id]; ok {
		return a, nil
	}
	return nil, store.ErrNotFound
}

func (m *mockStore) GetAgentLogs(ctx context.Context, agentID string, limit int, beforeID int64, subagentID string) ([]types.ITelemetryFrame, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastRequestedID = agentID
	m.lastLogsLimit = limit
	m.lastLogsBefore = beforeID
	m.lastLogsSub = subagentID
	if m.logsErr != nil {
		return nil, m.logsErr
	}
	return m.logs, nil
}

func setupTestServer(t *testing.T) (*Server, *mockStore, *stream.Hub) {
	t.Helper()
	encodedID := "885d9dd12b6d62a9:claude-code:be20b445"
	mock := &mockStore{
		status: &types.StatusResponse{
			Status:                 "ok",
			ActiveAgentCount:       1,
			RequiresAttentionCount: 0,
			TotalFleetSpend:        10.50,
		},
		environments: []types.IEnvironmentInfo{
			{ID: "env1", Label: "macbook", Platform: "darwin"},
		},
		projects: []types.ProjectGroup{
			{
				Project:   types.IProjectInfo{ID: "p1", Name: "project1"},
				TotalCost: 10.50,
			},
		},
		agents: []types.IAgentState{
			{
				ID:        "agent-1",
				SessionID: "sess-1",
				Harness:   types.HarnessClaudeCode,
				Project:   types.IProjectInfo{ID: "p1", Name: "project1"},
				Environment: types.IEnvironmentInfo{
					ID: "env1",
				},
				Activity: types.IAgentActivity{
					Lifecycle: types.LifecycleRunning,
					Activity:  types.ActivityBusy,
				},
			},
			{
				ID:        encodedID,
				SessionID: "be20b445",
				Harness:   types.HarnessClaudeCode,
				Project:   types.IProjectInfo{ID: "p1", Name: "project1"},
				Environment: types.IEnvironmentInfo{
					ID: "885d9dd12b6d62a9",
				},
				Activity: types.IAgentActivity{
					Lifecycle: types.LifecycleRunning,
					Activity:  types.ActivityBusy,
				},
			},
		},
		agentMap: make(map[string]*types.IAgentState),
		logs: []types.ITelemetryFrame{
			{
				Seq:       1,
				AgentID:   "agent-1",
				EventType: "tool_call",
			},
		},
	}
	mock.agentMap["agent-1"] = &mock.agents[0]
	mock.agentMap[encodedID] = &mock.agents[1]

	hub := stream.NewHub()
	t.Cleanup(func() { hub.Close() })

	srv := NewServer(ServerConfig{
		Store:       mock,
		Hub:         hub,
		AuthToken:   "secret-token",
		CORSOrigins: []string{"*"},
	})

	return srv, mock, hub
}

func TestNewServer_Defaults(t *testing.T) {
	mock := &mockStore{}
	hub := stream.NewHub()
	defer hub.Close()

	srv := NewServer(ServerConfig{
		Store: mock,
		Hub:   hub,
	})

	if srv.logger == nil {
		t.Errorf("expected default logger")
	}
	if len(srv.cfg.CORSOrigins) != 1 || srv.cfg.CORSOrigins[0] != "http://localhost:3000" {
		t.Errorf("expected default CORS origin http://localhost:3000, got: %v", srv.cfg.CORSOrigins)
	}
}

func TestAuthMiddleware(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	tests := []struct {
		name       string
		header     string
		query      string
		wantStatus int
	}{
		{"no auth", "", "", http.StatusUnauthorized},
		{"invalid header wrong scheme", "Basic secret-token", "", http.StatusUnauthorized},
		{"invalid header token", "Bearer wrong-token", "", http.StatusUnauthorized},
		{"malformed auth header single word", "Bearer", "", http.StatusUnauthorized},
		{"valid header", "Bearer secret-token", "", http.StatusOK},
		{"valid header lowercase bearer", "bearer secret-token", "", http.StatusOK},
		{"valid query param token", "", "?token=secret-token", http.StatusOK},
		{"invalid query param token", "", "?token=wrong-token", http.StatusUnauthorized},
		{"valid query param access_token", "", "?access_token=secret-token", http.StatusOK},
		{"invalid query param access_token", "", "?access_token=wrong-token", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/status"+tt.query, nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			w := httptest.NewRecorder()
			srv.Router().ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}

	t.Run("empty expected token allows unauthenticated access", func(t *testing.T) {
		unauthSrv := NewServer(ServerConfig{
			Store:     &mockStore{},
			Hub:       stream.NewHub(),
			AuthToken: "",
		})
		req := httptest.NewRequest("GET", "/api/v1/status", nil)
		w := httptest.NewRecorder()
		unauthSrv.Router().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
		}
	})
}

func TestCORSPreflight_PatchMethod(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	req := httptest.NewRequest("OPTIONS", "/api/v1/agents/agent-1", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "PATCH")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	allowMethods := w.Header().Get("Access-Control-Allow-Methods")
	if !strings.Contains(allowMethods, "PATCH") {
		t.Errorf("Access-Control-Allow-Methods = %q, want it to contain PATCH", allowMethods)
	}
}

func TestGetStatus(t *testing.T) {
	srv, mock, _ := setupTestServer(t)

	t.Run("success", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/status", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}

		var resp types.StatusResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decoding json: %v", err)
		}

		if resp.ActiveAgentCount != 1 || resp.TotalFleetSpend != 10.50 {
			t.Errorf("unexpected status response: %+v", resp)
		}
	})

	t.Run("error 500", func(t *testing.T) {
		mock.mu.Lock()
		mock.statusErr = errors.New("db error")
		mock.mu.Unlock()
		defer func() {
			mock.mu.Lock()
			mock.statusErr = nil
			mock.mu.Unlock()
		}()

		req := httptest.NewRequest("GET", "/api/v1/status", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
		}
	})
}

func TestGetProjects(t *testing.T) {
	srv, mock, _ := setupTestServer(t)

	t.Run("success", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/projects", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}

		var projects []types.ProjectGroup
		if err := json.NewDecoder(w.Body).Decode(&projects); err != nil {
			t.Fatalf("decoding json: %v", err)
		}

		if len(projects) != 1 || projects[0].Project.ID != "p1" {
			t.Errorf("unexpected projects response: %+v", projects)
		}
	})

	t.Run("error 500", func(t *testing.T) {
		mock.mu.Lock()
		mock.projectsErr = errors.New("db error")
		mock.mu.Unlock()
		defer func() {
			mock.mu.Lock()
			mock.projectsErr = nil
			mock.mu.Unlock()
		}()

		req := httptest.NewRequest("GET", "/api/v1/projects", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
		}
	})
}

func TestGetEnvironments(t *testing.T) {
	srv, mock, _ := setupTestServer(t)

	t.Run("success", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/environments", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}

		var envs []types.IEnvironmentInfo
		if err := json.NewDecoder(w.Body).Decode(&envs); err != nil {
			t.Fatalf("decoding json: %v", err)
		}

		if len(envs) != 1 || envs[0].ID != "env1" {
			t.Errorf("unexpected environments response: %+v", envs)
		}
	})

	t.Run("error 500", func(t *testing.T) {
		mock.mu.Lock()
		mock.environmentsErr = errors.New("db error")
		mock.mu.Unlock()
		defer func() {
			mock.mu.Lock()
			mock.environmentsErr = nil
			mock.mu.Unlock()
		}()

		req := httptest.NewRequest("GET", "/api/v1/environments", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
		}
	})
}

func TestGetAgents(t *testing.T) {
	srv, mock, _ := setupTestServer(t)

	t.Run("list all", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/agents", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}

		var agents []types.IAgentState
		if err := json.NewDecoder(w.Body).Decode(&agents); err != nil {
			t.Fatalf("decoding json: %v", err)
		}

		if len(agents) != 2 {
			t.Fatalf("len(agents) = %d, want 2", len(agents))
		}
	})

	t.Run("filter matches", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/agents?project=p1&environment=env1&lifecycle=RUNNING&activity=BUSY", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		var agents []types.IAgentState
		_ = json.NewDecoder(w.Body).Decode(&agents)
		if len(agents) != 1 {
			t.Fatalf("len(agents) = %d, want 1", len(agents))
		}
	})

	t.Run("filter no matches", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/agents?project=p2", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		var agents []types.IAgentState
		_ = json.NewDecoder(w.Body).Decode(&agents)
		if len(agents) != 0 {
			t.Fatalf("len(agents) = %d, want 0", len(agents))
		}
	})

	t.Run("error 500", func(t *testing.T) {
		mock.mu.Lock()
		mock.agentsErr = errors.New("db error")
		mock.mu.Unlock()
		defer func() {
			mock.mu.Lock()
			mock.agentsErr = nil
			mock.mu.Unlock()
		}()

		req := httptest.NewRequest("GET", "/api/v1/agents", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
		}
	})
}

func TestGetAgentByID(t *testing.T) {
	srv, mock, _ := setupTestServer(t)

	t.Run("found with plain ID", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/agents/agent-1", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}

		var agent types.IAgentState
		if err := json.NewDecoder(w.Body).Decode(&agent); err != nil {
			t.Fatalf("decoding json: %v", err)
		}
		if agent.ID != "agent-1" {
			t.Errorf("agent.ID = %q, want agent-1", agent.ID)
		}
	})

	t.Run("found with URL-encoded agent ID", func(t *testing.T) {
		// URL-encoded 885d9dd12b6d62a9%3Aclaude-code%3Abe20b445
		req := httptest.NewRequest("GET", "/api/v1/agents/885d9dd12b6d62a9%3Aclaude-code%3Abe20b445", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}

		var agent types.IAgentState
		if err := json.NewDecoder(w.Body).Decode(&agent); err != nil {
			t.Fatalf("decoding json: %v", err)
		}
		if agent.ID != "885d9dd12b6d62a9:claude-code:be20b445" {
			t.Errorf("agent.ID = %q, want '885d9dd12b6d62a9:claude-code:be20b445'", agent.ID)
		}
		if mock.lastRequestedID != "885d9dd12b6d62a9:claude-code:be20b445" {
			t.Errorf("mock received ID = %q, want unescaped '885d9dd12b6d62a9:claude-code:be20b445'", mock.lastRequestedID)
		}
	})

	t.Run("not found 404", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/agents/agent-non-existent", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
		}
	})

	t.Run("error 500", func(t *testing.T) {
		mock.mu.Lock()
		mock.agentErr = errors.New("db error")
		mock.mu.Unlock()
		defer func() {
			mock.mu.Lock()
			mock.agentErr = nil
			mock.mu.Unlock()
		}()

		req := httptest.NewRequest("GET", "/api/v1/agents/agent-1", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
		}
	})
}

func TestPatchAgent(t *testing.T) {
	// Fake ingester backend
	fakeIngester := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPatch {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		if strings.Contains(r.URL.Path, "non-existent") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
			return
		}

		var body struct {
			IsHidden bool `json:"isHidden"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(types.IAgentState{
			ID:       "agent-1",
			IsHidden: body.IsHidden,
		})
	}))
	defer fakeIngester.Close()

	mock := &mockStore{}
	hub := stream.NewHub()
	defer hub.Close()

	srv := NewServer(ServerConfig{
		Store:       mock,
		Hub:         hub,
		AuthToken:   "secret-token",
		IngesterURL: fakeIngester.URL,
	})

	t.Run("success hide agent", func(t *testing.T) {
		body := strings.NewReader(`{"isHidden":true}`)
		req := httptest.NewRequest("PATCH", "/api/v1/agents/agent-1", body)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}

		var agent types.IAgentState
		if err := json.NewDecoder(w.Body).Decode(&agent); err != nil {
			t.Fatalf("decoding json: %v", err)
		}
		if !agent.IsHidden {
			t.Errorf("expected agent.IsHidden == true")
		}
	})

	t.Run("404 when agent not found in ingester", func(t *testing.T) {
		body := strings.NewReader(`{"isHidden":true}`)
		req := httptest.NewRequest("PATCH", "/api/v1/agents/non-existent", body)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
		}
	})

	t.Run("400 on invalid body", func(t *testing.T) {
		body := strings.NewReader(`{invalid json`)
		req := httptest.NewRequest("PATCH", "/api/v1/agents/agent-1", body)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})

	t.Run("502 when ingester is unreachable", func(t *testing.T) {
		unreachableSrv := NewServer(ServerConfig{
			Store:       mock,
			Hub:         hub,
			AuthToken:   "secret-token",
			IngesterURL: "http://127.0.0.1:59999",
		})
		body := strings.NewReader(`{"isHidden":true}`)
		req := httptest.NewRequest("PATCH", "/api/v1/agents/agent-1", body)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		unreachableSrv.Router().ServeHTTP(w, req)

		if w.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusBadGateway)
		}
	})

	t.Run("forwards incoming auth header when cfg.AuthToken is empty", func(t *testing.T) {
		noAuthSrv := NewServer(ServerConfig{
			Store:       mock,
			Hub:         hub,
			IngesterURL: fakeIngester.URL,
		})
		body := strings.NewReader(`{"isHidden":true}`)
		req := httptest.NewRequest("PATCH", "/api/v1/agents/agent-1", body)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		noAuthSrv.Router().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}
	})
}

func TestGetAgentLogs(t *testing.T) {
	srv, mock, _ := setupTestServer(t)

	t.Run("success with valid parameters", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/agents/agent-1/logs?limit=50&before=100&subagent=sub1", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}

		var logs []types.ITelemetryFrame
		if err := json.NewDecoder(w.Body).Decode(&logs); err != nil {
			t.Fatalf("decoding json: %v", err)
		}

		if len(logs) != 1 {
			t.Fatalf("len(logs) = %d, want 1", len(logs))
		}
		if mock.lastLogsLimit != 50 || mock.lastLogsBefore != 100 || mock.lastLogsSub != "sub1" {
			t.Errorf("mock received params limit=%d before=%d sub=%s", mock.lastLogsLimit, mock.lastLogsBefore, mock.lastLogsSub)
		}
	})

	t.Run("success with URL-encoded agent ID", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/agents/885d9dd12b6d62a9%3Aclaude-code%3Abe20b445/logs", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}

		if mock.lastRequestedID != "885d9dd12b6d62a9:claude-code:be20b445" {
			t.Errorf("mock received unescaped agentID = %q", mock.lastRequestedID)
		}
		// Default limit when unspecified is 100, before is 0
		if mock.lastLogsLimit != 100 || mock.lastLogsBefore != 0 {
			t.Errorf("default limit/before = %d/%d, want 100/0", mock.lastLogsLimit, mock.lastLogsBefore)
		}
	})

	t.Run("invalid query parameters gracefully fallback to defaults", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/agents/agent-1/logs?limit=not-a-number&before=invalid-number", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}
		if mock.lastLogsLimit != 100 || mock.lastLogsBefore != 0 {
			t.Errorf("invalid params should fallback to defaults: limit=%d before=%d", mock.lastLogsLimit, mock.lastLogsBefore)
		}
	})

	t.Run("negative limit/before parameters fallback to defaults", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/agents/agent-1/logs?limit=-10&before=-5", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}
		if mock.lastLogsLimit != 100 || mock.lastLogsBefore != 0 {
			t.Errorf("negative params should fallback to defaults: limit=%d before=%d", mock.lastLogsLimit, mock.lastLogsBefore)
		}
	})

	t.Run("error 500", func(t *testing.T) {
		mock.mu.Lock()
		mock.logsErr = errors.New("db error")
		mock.mu.Unlock()
		defer func() {
			mock.mu.Lock()
			mock.logsErr = nil
			mock.mu.Unlock()
		}()

		req := httptest.NewRequest("GET", "/api/v1/agents/agent-1/logs", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
		}
	})
}

// nonFlushingRecorder implements http.ResponseWriter without http.Flusher
type nonFlushingRecorder struct {
	header http.Header
	code   int
}

func (n *nonFlushingRecorder) Header() http.Header {
	if n.header == nil {
		n.header = make(http.Header)
	}
	return n.header
}

func (n *nonFlushingRecorder) Write(b []byte) (int, error) {
	return len(b), nil
}

func (n *nonFlushingRecorder) WriteHeader(statusCode int) {
	n.code = statusCode
}

func TestEventsSSE(t *testing.T) {
	srv, mock, hub := setupTestServer(t)

	t.Run("success stream and heartbeats", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		req := httptest.NewRequest("GET", "/api/v1/events", nil).WithContext(ctx)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()

		done := make(chan struct{})
		go func() {
			defer close(done)
			srv.Router().ServeHTTP(w, req)
		}()

		time.Sleep(50 * time.Millisecond)

		hub.Broadcast(&types.IServerEvent{
			Type:    "agent_removed",
			At:      time.Now().UnixMilli(),
			AgentID: "agent-1",
		})

		time.Sleep(50 * time.Millisecond)
		cancel()
		<-done

		body := w.Body.String()
		if !strings.Contains(body, "snapshot") {
			t.Errorf("expected SSE body to contain 'snapshot', got: %s", body)
		}
		if !strings.Contains(body, "agent_removed") {
			t.Errorf("expected SSE body to contain 'agent_removed', got: %s", body)
		}
	})

	t.Run("streaming unsupported when flusher missing", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/events", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		rec := &nonFlushingRecorder{}

		srv.Router().ServeHTTP(rec, req)
		if rec.code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", rec.code, http.StatusInternalServerError)
		}
	})

	t.Run("environments error on sse connect", func(t *testing.T) {
		mock.mu.Lock()
		mock.environmentsErr = errors.New("load error")
		mock.mu.Unlock()
		defer func() {
			mock.mu.Lock()
			mock.environmentsErr = nil
			mock.mu.Unlock()
		}()

		req := httptest.NewRequest("GET", "/api/v1/events", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)
	})

	t.Run("agents error on sse connect", func(t *testing.T) {
		mock.mu.Lock()
		mock.agentsErr = errors.New("agents load error")
		mock.mu.Unlock()
		defer func() {
			mock.mu.Lock()
			mock.agentsErr = nil
			mock.mu.Unlock()
		}()

		req := httptest.NewRequest("GET", "/api/v1/events", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)
	})

	t.Run("hub closed terminates stream", func(t *testing.T) {
		testHub := stream.NewHub()
		testSrv := NewServer(ServerConfig{
			Store:       mock,
			Hub:         testHub,
			AuthToken:   "secret-token",
			CORSOrigins: []string{"*"},
		})

		req := httptest.NewRequest("GET", "/api/v1/events", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()

		done := make(chan struct{})
		go func() {
			defer close(done)
			testSrv.Router().ServeHTTP(w, req)
		}()

		time.Sleep(50 * time.Millisecond)
		testHub.Close()

		select {
		case <-done:
			// Terminated cleanly
		case <-time.After(500 * time.Millisecond):
			t.Fatal("SSE handler did not exit after hub close")
		}
	})
}

func TestEventsWS(t *testing.T) {
	srv, mock, hub := setupTestServer(t)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	t.Run("success connect and message delivery", func(t *testing.T) {
		wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws/events?token=secret-token"

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		conn, _, err := websocket.Dial(ctx, wsURL, nil)
		if err != nil {
			t.Fatalf("websocket dial failed: %v", err)
		}
		defer func() {
			_ = conn.CloseNow()
		}()

		// 1. First message should be the initial snapshot
		_, msgBytes, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("reading first ws message: %v", err)
		}

		var snapshotEvt types.IServerEvent
		if err := json.Unmarshal(msgBytes, &snapshotEvt); err != nil {
			t.Fatalf("unmarshaling snapshot: %v", err)
		}
		if snapshotEvt.Type != "snapshot" {
			t.Errorf("expected snapshot event, got %q", snapshotEvt.Type)
		}

		// 2. Broadcast a live event and assert receipt
		hub.Broadcast(&types.IServerEvent{
			Type:    "agent_removed",
			At:      time.Now().UnixMilli(),
			AgentID: "agent-1",
		})

		_, msgBytes, err = conn.Read(ctx)
		if err != nil {
			t.Fatalf("reading second ws message: %v", err)
		}

		var liveEvt types.IServerEvent
		if err := json.Unmarshal(msgBytes, &liveEvt); err != nil {
			t.Fatalf("unmarshaling live event: %v", err)
		}
		if liveEvt.Type != "agent_removed" {
			t.Errorf("expected agent_removed event, got %q", liveEvt.Type)
		}
	})

	t.Run("environments error on ws connect", func(t *testing.T) {
		mock.mu.Lock()
		mock.environmentsErr = errors.New("load error")
		mock.mu.Unlock()
		defer func() {
			mock.mu.Lock()
			mock.environmentsErr = nil
			mock.mu.Unlock()
		}()

		wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws/events?token=secret-token"
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		conn, _, err := websocket.Dial(ctx, wsURL, nil)
		if err == nil {
			// If connect was accepted, reading should fail immediately due to early exit
			_, _, _ = conn.Read(ctx)
			_ = conn.CloseNow()
		}
	})

	t.Run("agents error on ws connect", func(t *testing.T) {
		mock.mu.Lock()
		mock.agentsErr = errors.New("agents load error")
		mock.mu.Unlock()
		defer func() {
			mock.mu.Lock()
			mock.agentsErr = nil
			mock.mu.Unlock()
		}()

		wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws/events?token=secret-token"
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		conn, _, err := websocket.Dial(ctx, wsURL, nil)
		if err == nil {
			_, _, _ = conn.Read(ctx)
			_ = conn.CloseNow()
		}
	})

	t.Run("hub closed terminates ws stream", func(t *testing.T) {
		testHub := stream.NewHub()
		testSrv := NewServer(ServerConfig{
			Store:       mock,
			Hub:         testHub,
			AuthToken:   "secret-token",
			CORSOrigins: []string{"*"},
		})
		wsServer := httptest.NewServer(testSrv.Router())
		defer wsServer.Close()

		wsURL := "ws" + strings.TrimPrefix(wsServer.URL, "http") + "/ws/events?token=secret-token"
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		conn, _, err := websocket.Dial(ctx, wsURL, nil)
		if err != nil {
			t.Fatalf("dial failed: %v", err)
		}
		defer func() {
			_ = conn.CloseNow()
		}()

		// Read snapshot
		_, _, _ = conn.Read(ctx)

		// Close hub
		testHub.Close()

		// Next read should return error/close
		_, _, err = conn.Read(ctx)
		if err == nil {
			t.Error("expected read error after hub closed")
		}
	})
}

func TestAgentLiveTailSSE(t *testing.T) {
	srv, _, hub := setupTestServer(t)

	t.Run("success with plain agent ID", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		req := httptest.NewRequest("GET", "/api/v1/agents/agent-1/live-tail", nil).WithContext(ctx)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()

		done := make(chan struct{})
		go func() {
			defer close(done)
			srv.Router().ServeHTTP(w, req)
		}()

		time.Sleep(50 * time.Millisecond)

		hub.Broadcast(&types.IServerEvent{
			Type:    "log_append",
			At:      time.Now().UnixMilli(),
			AgentID: "agent-1",
			Event: &types.ITelemetryFrame{
				AgentID:   "agent-1",
				EventType: "thinking",
			},
		})

		time.Sleep(50 * time.Millisecond)
		cancel()
		<-done

		body := w.Body.String()
		if !strings.Contains(body, "thinking") {
			t.Errorf("expected live-tail body to contain 'thinking', got: %s", body)
		}
	})

	t.Run("success with URL-encoded agent ID", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// URL-encoded 885d9dd12b6d62a9%3Aclaude-code%3Abe20b445
		req := httptest.NewRequest("GET", "/api/v1/agents/885d9dd12b6d62a9%3Aclaude-code%3Abe20b445/live-tail", nil).WithContext(ctx)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()

		done := make(chan struct{})
		go func() {
			defer close(done)
			srv.Router().ServeHTTP(w, req)
		}()

		time.Sleep(50 * time.Millisecond)

		hub.Broadcast(&types.IServerEvent{
			Type:    "log_append",
			At:      time.Now().UnixMilli(),
			AgentID: "885d9dd12b6d62a9:claude-code:be20b445",
			Event: &types.ITelemetryFrame{
				AgentID:   "885d9dd12b6d62a9:claude-code:be20b445",
				EventType: "tool_call",
			},
		})

		time.Sleep(50 * time.Millisecond)
		cancel()
		<-done

		body := w.Body.String()
		if !strings.Contains(body, "tool_call") {
			t.Errorf("expected live-tail body to contain 'tool_call', got: %s", body)
		}
	})

	t.Run("streaming unsupported when flusher missing", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/agents/agent-1/live-tail", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		rec := &nonFlushingRecorder{}

		srv.Router().ServeHTTP(rec, req)
		if rec.code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", rec.code, http.StatusInternalServerError)
		}
	})

	t.Run("hub closed terminates live-tail stream", func(t *testing.T) {
		testHub := stream.NewHub()
		testSrv := NewServer(ServerConfig{
			Store:       &mockStore{},
			Hub:         testHub,
			AuthToken:   "secret-token",
			CORSOrigins: []string{"*"},
		})

		req := httptest.NewRequest("GET", "/api/v1/agents/agent-1/live-tail", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()

		done := make(chan struct{})
		go func() {
			defer close(done)
			testSrv.Router().ServeHTTP(w, req)
		}()

		time.Sleep(50 * time.Millisecond)
		testHub.Close()

		select {
		case <-done:
			// Terminated cleanly
		case <-time.After(500 * time.Millisecond):
			t.Fatal("live-tail handler did not exit after hub close")
		}
	})
}

func TestHandlers_FocusAgentProxy(t *testing.T) {
	mockIngester := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/internal/agents/") || !strings.HasSuffix(r.URL.Path, "/focus") {
			http.NotFound(w, r)
			return
		}

		if r.Header.Get("Authorization") != "Bearer secret-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "exited") {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"agent is exited"}`))
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"paneId":"w1:p1"}`))
	}))
	defer mockIngester.Close()

	srv := NewServer(ServerConfig{
		Store:       &mockStore{},
		Hub:         stream.NewHub(),
		IngesterURL: mockIngester.URL,
		AuthToken:   "secret-token",
		CORSOrigins: []string{"*"},
	})

	// 1. Successful proxy
	req := httptest.NewRequest("POST", "/api/v1/agents/agent-1/focus", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var res map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if res["ok"] != true || res["paneId"] != "w1:p1" {
		t.Errorf("unexpected response payload: %+v", res)
	}

	// 2. Conflict status forwarded from ingester
	req = httptest.NewRequest("POST", "/api/v1/agents/agent-exited/focus", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", w.Code)
	}

	// 3. Ingester connection failure returns 502 Bad Gateway
	badSrv := NewServer(ServerConfig{
		Store:       &mockStore{},
		Hub:         stream.NewHub(),
		IngesterURL: "http://127.0.0.1:59999", // Unreachable port
		AuthToken:   "secret-token",
		CORSOrigins: []string{"*"},
	})
	req = httptest.NewRequest("POST", "/api/v1/agents/agent-1/focus", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	w = httptest.NewRecorder()
	badSrv.Router().ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d", w.Code)
	}

	// 5. Direct invocation with missing id returns 400 Bad Request
	req = httptest.NewRequest("POST", "/api/v1/agents//focus", nil)
	w = httptest.NewRecorder()
	srv.handleFocusAgent(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for empty id, got %d", w.Code)
	}
}
