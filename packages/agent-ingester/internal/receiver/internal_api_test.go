package receiver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/classifier"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/store"
)

func TestInternalAgentStoreFailures(t *testing.T) {
	for _, tc := range []struct {
		name       string
		method     string
		suffix     string
		body       string
		classified bool
	}{
		{"visibility", http.MethodPatch, "", `{"isHidden":true}`, false},
		{"dismissal", http.MethodPatch, "", `{"dismissCompletion":true}`, false},
		{"classified dismissal", http.MethodPatch, "", `{"dismissCompletion":true}`, true},
		{"focus", http.MethodPost, "/focus", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, err := store.Open(":memory:")
			if err != nil {
				t.Fatal(err)
			}
			var cls *classifier.Classifier
			if tc.classified {
				cls = classifier.New(st, nil, nil)
				t.Cleanup(func() { _ = cls.Close() })
			}
			rcv := New(st, cls, nil, "test-token")
			if err := st.Close(); err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(tc.method, "/api/v1/internal/agents/agent-1"+tc.suffix, strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer test-token")
			req.SetPathValue("id", "agent-1")
			w := httptest.NewRecorder()
			rcv.InternalHandler().ServeHTTP(w, req)
			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500; body: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestInternalAgentRejectsCompletionAlias(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	unseen := true
	completed := int64(123)
	agent := types.IAgentState{ID: "agent-1", Harness: types.HarnessClaudeCode, Environment: types.IEnvironmentInfo{ID: "env", Platform: "darwin"}, Activity: types.IAgentActivity{Activity: types.ActivityIdle, HasUnseenCompletion: &unseen, CompletedAt: &completed}}
	if err := st.SaveAgentState(agent); err != nil {
		t.Fatal(err)
	}
	rcv := New(st, nil, nil, "test-token")
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/internal/agents/agent-1", strings.NewReader(`{"acknowledgeCompletion":true}`))
	req.Header.Set("Authorization", "Bearer test-token")
	req.SetPathValue("id", "agent-1")
	w := httptest.NewRecorder()
	rcv.InternalHandler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	got, err := st.GetAgent(agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Activity.HasUnseenCompletion == nil || !*got.Activity.HasUnseenCompletion || got.Activity.CompletedAt == nil || *got.Activity.CompletedAt != completed {
		t.Errorf("unrecognized alias changed persisted completion: %+v", got.Activity)
	}
}

func TestInternalAgentPatch(t *testing.T) {
	for _, classified := range []bool{false, true} {
		name := "store"
		if classified {
			name = "classifier"
		}
		t.Run(name, func(t *testing.T) {
			st, err := store.Open(":memory:")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = st.Close() })
			var cls *classifier.Classifier
			if classified {
				cls = classifier.New(st, nil, nil)
				t.Cleanup(func() { _ = cls.Close() })
			}
			handler := New(st, cls, nil, "test-token").InternalHandler()
			for _, tc := range []struct {
				name, id, body    string
				want              int
				hidden, completed bool
			}{
				{"hide", "agent-1", `{"isHidden":true}`, 200, true, true},
				{"unhide", "agent-1", `{"isHidden":false}`, 200, false, true},
				{"dismiss", "agent-1", `{"dismissCompletion":true}`, 200, false, false},
				{"composite", "agent-1", `{"isHidden":true,"dismissCompletion":true}`, 200, true, false},
				{"empty", "agent-1", `{}`, 400, false, true},
				{"false dismissal", "agent-1", `{"dismissCompletion":false}`, 400, false, true},
				{"malformed", "agent-1", `{invalid`, 400, false, true},
				{"missing visibility", "missing", `{"isHidden":true}`, 404, false, true},
				{"missing dismissal", "missing", `{"dismissCompletion":true}`, 404, false, true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					unseen := true
					completedAt := int64(123)
					initial := types.IAgentState{
						ID: "agent-1", Harness: types.HarnessClaudeCode,
						Environment: types.IEnvironmentInfo{ID: "env", Platform: "darwin"},
						Activity:    types.IAgentActivity{Activity: types.ActivityIdle, HasUnseenCompletion: &unseen, CompletedAt: &completedAt},
					}
					if err := st.SaveAgentState(initial); err != nil {
						t.Fatal(err)
					}
					req := httptest.NewRequest(http.MethodPatch, "/api/v1/internal/agents/"+tc.id, strings.NewReader(tc.body))
					req.Header.Set("Authorization", "Bearer test-token")
					w := httptest.NewRecorder()
					handler.ServeHTTP(w, req)
					if w.Code != tc.want {
						t.Fatalf("status = %d, want %d; body: %s", w.Code, tc.want, w.Body.String())
					}
					loaded, err := st.GetAgent(initial.ID)
					if err != nil {
						t.Fatal(err)
					}
					assertState := func(agent *types.IAgentState) {
						t.Helper()
						if agent == nil {
							t.Fatal("missing agent")
						}
						if agent.IsHidden != tc.hidden || (agent.HiddenAt != nil) != tc.hidden {
							t.Errorf("visibility = %t, hiddenAt = %v", agent.IsHidden, agent.HiddenAt)
						}
						hasUnseen := agent.Activity.HasUnseenCompletion != nil && *agent.Activity.HasUnseenCompletion
						if hasUnseen != tc.completed || (agent.Activity.CompletedAt != nil) != tc.completed {
							t.Errorf("completion = %t, completedAt = %v", hasUnseen, agent.Activity.CompletedAt)
						}
					}
					assertState(loaded)
					if tc.want == http.StatusOK {
						var response types.IAgentState
						if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
							t.Fatal(err)
						}
						assertState(&response)
					}
				})
			}
		})
	}
}

func TestInternalAgentAuthentication(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, auth string
		want                     int
	}{
		{"missing patch auth", "PATCH", "/api/v1/internal/agents/agent-1", "", 401},
		{"missing focus auth", "POST", "/api/v1/internal/agents/agent-1/focus", "", 401},
		{"wrong scheme", "PATCH", "/api/v1/internal/agents/agent-1", "Basic test-token", 401},
		{"wrong token", "PATCH", "/api/v1/internal/agents/agent-1", "Bearer wrong-token", 401},
		{"empty token", "PATCH", "/api/v1/internal/agents/agent-1", "Bearer ", 401},
		{"valid token", "PATCH", "/api/v1/internal/agents/agent-1", "Bearer test-token", 400},
		{"wrong patch method", "POST", "/api/v1/internal/agents/agent-1", "Bearer test-token", 405},
		{"wrong focus method", "PATCH", "/api/v1/internal/agents/agent-1/focus", "Bearer test-token", 405},
		{"missing id", "PATCH", "/api/v1/internal/agents/", "Bearer test-token", 404},
		{"unknown route", "GET", "/api/v1/internal/unknown", "", 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := New(nil, nil, nil, "test-token").InternalHandler()
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
			req.Header.Set("Authorization", tc.auth)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d", w.Code, tc.want)
			}
		})
	}
	t.Run("unconfigured token", func(t *testing.T) {
		handler := New(nil, nil, nil, "").InternalHandler()
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/internal/agents/agent-1", strings.NewReader(`{}`))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})
}
