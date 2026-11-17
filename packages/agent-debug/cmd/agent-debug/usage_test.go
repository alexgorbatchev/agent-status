package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/usage"
)

func TestAgentUsageCommand(t *testing.T) {
	const id = "env:pi:session"
	r := usage.NewReport(id, types.HarnessPi)
	r.Skills = append(r.Skills, usage.Entry{Name: "review", Operation: "load", Count: 2, FirstUsedAt: 1000, LastUsedAt: 2000, LatestEvidence: usage.Evidence{Source: "skill_expansion"}})
	r.Commands = append(r.Commands, usage.Entry{Name: "/skill:review", Operation: "invoke", Count: 2})
	r.MCP = append(r.MCP, usage.Entry{Name: "list_issues", Server: "github", Operation: "invoke", Count: 1})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.EscapedPath() != "/api/v1/agents/env:pi:session/usage" || req.Header.Get("Authorization") != "Bearer usage-test-token" || req.URL.RawQuery != "" {
			t.Errorf("unexpected request: %s", req.URL)
		}
		_ = json.NewEncoder(w).Encode(r)
	}))
	defer server.Close()
	t.Setenv("AGENT_SERVER_HTTP_URL", server.URL)
	t.Setenv("AGENT_STATUS_TOKEN", "usage-test-token")
	for _, mode := range []string{"0", "1"} {
		for _, jsonOut := range []bool{false, true} {
			t.Run(mode+map[bool]string{false: "/text", true: "/json"}[jsonOut], func(t *testing.T) {
				t.Setenv("AGENT", mode)
				cmd := newRootCmd()
				var out, diag bytes.Buffer
				cmd.SetOut(&out)
				cmd.SetErr(&diag)
				args := []string{"agent", "usage", id}
				if jsonOut {
					args = append(args, "--json")
				}
				cmd.SetArgs(args)
				if err := cmd.Execute(); err != nil {
					t.Fatal(err)
				}
				if diag.Len() != 0 || strings.Contains(out.String(), "usage-test-token") {
					t.Fatal("diagnostics or credential leaked into command output")
				}
				if jsonOut {
					var got usage.Report
					if err := json.Unmarshal(out.Bytes(), &got); err != nil {
						t.Fatal(err)
					}
					if got.AgentID != id || got.Skills[0].Count != 2 || got.Coverage["commands"].Status != "partial" {
						t.Fatalf("bad JSON report: %+v", got)
					}
				} else if !strings.Contains(out.String(), "review") || !strings.Contains(out.String(), "github") || !strings.Contains(out.String(), "partial") {
					t.Fatalf("missing usage/coverage: %s", &out)
				}
			})
		}
	}
	cmd := newRootCmd()
	cmd.SetOut(failedAgentWriter{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"agent", "usage", id})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected output failure")
	}
}

func TestAgentUsageErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		args       []string
		want       string
	}{
		{"missing ID", "", 200, nil, "accepts 1 arg"},
		{"not found", "private response", 404, []string{"missing"}, "404"},
		{"invalid JSON", "<html>private response</html>", 200, []string{"agent"}, "invalid agent usage"},
		{"wrong agent", `{"agentId":"different"}`, 200, []string{"agent"}, "invalid agent usage"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer s.Close()
			cmd := newRootCmd()
			var out, diag bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&diag)
			args := append([]string{"agent", "usage"}, tc.args...)
			args = append(args, "--server-url", s.URL)
			cmd.SetArgs(args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want %q", err, tc.want)
			}
			if out.Len() != 0 || diag.Len() != 0 || strings.Contains(err.Error(), "private response") {
				t.Fatal("failure polluted output")
			}
		})
	}
}
