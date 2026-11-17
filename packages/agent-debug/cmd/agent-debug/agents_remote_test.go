package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentListServerResponses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		token  string
		want   string
	}{
		{"empty authenticated", http.StatusOK, "[]", "test-api-token", ""},
		{"empty anonymous", http.StatusOK, "[]", "", ""},
		{"missing token", http.StatusUnauthorized, "test-api-token", "", "AGENT_STATUS_TOKEN"},
		{"wrong token", http.StatusUnauthorized, "test-api-token", "test-api-token", "AGENT_STATUS_TOKEN"},
		{"forbidden", http.StatusForbidden, "test-api-token", "test-api-token", "403"},
		{"server failure", http.StatusInternalServerError, "test-api-token", "test-api-token", "500"},
		{"malformed JSON", http.StatusOK, "[", "test-api-token", "invalid agent list"},
		{"wrong JSON shape", http.StatusOK, "{}", "test-api-token", "invalid agent list"},
		{"HTML", http.StatusOK, "<html>test-api-token</html>", "test-api-token", "invalid agent list"},
		{"extra JSON", http.StatusOK, "[] []", "test-api-token", "invalid agent list"},
		{"trailing junk", http.StatusOK, "[] test-api-token", "test-api-token", "invalid agent list"},
		{"partial list", http.StatusOK, `[{"sessionId":"partial"},`, "test-api-token", "invalid agent list"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				wantAuth := ""
				if tc.token != "" {
					wantAuth = "Bearer " + tc.token
				}
				if r.Header.Get("Authorization") != wantAuth || r.URL.RawQuery != "lifecycle=RUNNING" {
					t.Error("unexpected authentication or query parameters")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			// An explicit URL overrides the environment, and remote listing ignores DB paths.
			t.Setenv("AGENT_SERVER_HTTP_URL", "invalid environment URL")
			t.Setenv("AGENT_DB_PATH", filepath.Join(t.TempDir(), "missing.db"))
			t.Setenv("AGENT_STATUS_TOKEN", tc.token)
			cmd := newRootCmd()
			var out, diagnostics bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&diagnostics)
			cmd.SetArgs([]string{"agent", "list", "--all", "--server-url", server.URL, "--debug"})
			err := cmd.Execute()
			if tc.want == "" && err != nil {
				t.Fatal(err)
			}
			if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if out.Len() != 0 || diagnostics.Len() != 0 {
				t.Fatalf("unexpected output: stdout=%q stderr=%q", out.String(), diagnostics.String())
			}
			if err != nil && strings.Contains(err.Error(), "test-api-token") {
				t.Fatal("credential or response body leaked into the error")
			}
		})
	}
}

func TestAgentListRemoteOptionErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"database conflict", []string{"--all", "--db", "missing.db"}, "[all db] were all set"},
		{"URL without all", []string{"--server-url", "https://example.com"}, "--server-url requires --all"},
		{"empty URL", []string{"--all", "--server-url", ""}, "server URL"},
		{"relative URL", []string{"--all", "--server-url", "/agents"}, "server URL"},
		{"invalid URL", []string{"--all", "--server-url", "http://%test-api-token"}, "server URL"},
		{"wrong scheme", []string{"--all", "--server-url", "ws://example.com"}, "server URL"},
		{"URL credential", []string{"--all", "--server-url", "https://user:test-api-token@example.com"}, "server URL"},
		{"URL query", []string{"--all", "--server-url", "https://example.com?token=test-api-token"}, "server URL"},
		{"empty URL query", []string{"--all", "--server-url", "https://example.com?"}, "server URL"},
		{"URL fragment", []string{"--all", "--server-url", "https://example.com#test-api-token"}, "server URL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newRootCmd()
			var out, diagnostics bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&diagnostics)
			cmd.SetArgs(append([]string{"agent", "list"}, tc.args...))
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if strings.Contains(err.Error(), "test-api-token") || out.Len() != 0 || diagnostics.Len() != 0 {
				t.Fatal("invalid options leaked credentials or wrote command output")
			}
		})
	}
}

func TestAgentListRemoteTransport(t *testing.T) {
	t.Setenv("AGENT_STATUS_TOKEN", "test-api-token")
	for _, name := range []string{"redirect", "malformed redirect", "cancellation", "connection refused", "invalid token", "write failure"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				switch name {
				case "redirect":
					if requests > 1 {
						t.Error("client followed redirect")
						return
					}
					w.Header().Set("Location", "/other?token=test-api-token")
					w.WriteHeader(http.StatusFound)
				case "malformed redirect":
					w.Header().Set("Location", "http://%test-api-token")
					w.WriteHeader(http.StatusFound)
				case "cancellation":
					cancel()
					<-r.Context().Done()
				case "write failure":
					_, _ = w.Write([]byte(`[{"sessionId":"remote","environment":{"id":"remote"},"activity":{"lifecycle":"RUNNING","currentRun":{"pid":123}}}]`))
				default:
					t.Error("unexpected request")
				}
			}))
			defer server.Close()
			if name == "connection refused" {
				server.Close()
			}
			if name == "invalid token" {
				t.Setenv("AGENT_STATUS_TOKEN", "test-api-token\r\nInjected: header")
			}
			cmd := newRootCmd()
			var out, diagnostics bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&diagnostics)
			if name == "write failure" {
				cmd.SetOut(failedAgentWriter{})
			}
			cmd.SetArgs([]string{"agent", "list", "--all", "--server-url", server.URL})
			err := cmd.ExecuteContext(ctx)
			if err == nil {
				t.Fatal("expected error")
			}
			if name == "cancellation" && !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want cancellation", err)
			}
			if name == "write failure" && !strings.Contains(err.Error(), "output closed") {
				t.Fatalf("error = %v, want output error", err)
			}
			if strings.Contains(err.Error(), "test-api-token") || out.Len() != 0 || diagnostics.Len() != 0 {
				t.Fatalf("unexpected output or credential disclosure: %v stdout=%q stderr=%q", err, out.String(), diagnostics.String())
			}
		})
	}
}
