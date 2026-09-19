package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/alexgorbatchev/agent-status/packages/agent-server/internal/store"
	"github.com/alexgorbatchev/agent-status/packages/agent-server/internal/stream"
	"github.com/alexgorbatchev/agent-status/packages/agent-server/internal/types"
)

type mockStore struct {
	status       *types.StatusResponse
	projects     []types.ProjectGroup
	environments []types.IEnvironmentInfo
	agents       []types.IAgentState
	agentMap     map[string]*types.IAgentState
	logs         []types.ITelemetryFrame
	err          error
}

func (m *mockStore) GetStatus(ctx context.Context) (*types.StatusResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.status != nil {
		return m.status, nil
	}
	return &types.StatusResponse{Status: "ok"}, nil
}

func (m *mockStore) GetProjects(ctx context.Context) ([]types.ProjectGroup, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.projects, nil
}

func (m *mockStore) GetEnvironments(ctx context.Context) ([]types.IEnvironmentInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.environments, nil
}

func (m *mockStore) GetAgents(ctx context.Context, filter store.AgentFilter) ([]types.IAgentState, error) {
	if m.err != nil {
		return nil, m.err
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
	if m.err != nil {
		return nil, m.err
	}
	if a, ok := m.agentMap[id]; ok {
		return a, nil
	}
	return nil, store.ErrNotFound
}

func (m *mockStore) GetAgentLogs(ctx context.Context, agentID string, limit int, beforeID int64, subagentID string) ([]types.ITelemetryFrame, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.logs, nil
}

func setupTestServer(t *testing.T) (*Server, *mockStore, *stream.Hub) {
	t.Helper()
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

func TestAuthMiddleware(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	tests := []struct {
		name       string
		header     string
		query      string
		wantStatus int
	}{
		{"no auth", "", "", http.StatusUnauthorized},
		{"invalid header", "Bearer wrong", "", http.StatusUnauthorized},
		{"valid header", "Bearer secret-token", "", http.StatusOK},
		{"valid query param", "", "?token=secret-token", http.StatusOK},
		{"valid query param access_token", "", "?access_token=secret-token", http.StatusOK},
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

	t.Run("error", func(t *testing.T) {
		mock.err = errors.New("db error")
		defer func() { mock.err = nil }()

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

	t.Run("error", func(t *testing.T) {
		mock.err = errors.New("db error")
		defer func() { mock.err = nil }()

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

	t.Run("error", func(t *testing.T) {
		mock.err = errors.New("db error")
		defer func() { mock.err = nil }()

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

		if len(agents) != 1 {
			t.Fatalf("len(agents) = %d, want 1", len(agents))
		}
	})

	t.Run("filter matches", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/agents?project=p1&lifecycle=RUNNING", nil)
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

	t.Run("error", func(t *testing.T) {
		mock.err = errors.New("db error")
		defer func() { mock.err = nil }()

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

	t.Run("found", func(t *testing.T) {
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

	t.Run("not found", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/agents/agent-non-existent", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
		}
	})

	t.Run("error", func(t *testing.T) {
		mock.err = errors.New("db error")
		defer func() { mock.err = nil }()

		req := httptest.NewRequest("GET", "/api/v1/agents/agent-1", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
		}
	})
}

func TestGetAgentLogs(t *testing.T) {
	srv, mock, _ := setupTestServer(t)

	t.Run("success", func(t *testing.T) {
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
	})

	t.Run("error", func(t *testing.T) {
		mock.err = errors.New("db error")
		defer func() { mock.err = nil }()

		req := httptest.NewRequest("GET", "/api/v1/agents/agent-1/logs", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
		}
	})
}

func TestEventsSSE(t *testing.T) {
	srv, mock, hub := setupTestServer(t)

	t.Run("success stream", func(t *testing.T) {
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

	t.Run("environments error on sse connect", func(t *testing.T) {
		mock.err = errors.New("load error")
		defer func() { mock.err = nil }()

		req := httptest.NewRequest("GET", "/api/v1/events", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()

		srv.Router().ServeHTTP(w, req)
	})
}

func TestEventsWS(t *testing.T) {
	srv, _, hub := setupTestServer(t)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws/events?token=secret-token"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.CloseNow()

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
}

func TestAgentLiveTailSSE(t *testing.T) {
	srv, _, hub := setupTestServer(t)

	t.Run("success", func(t *testing.T) {
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
}
