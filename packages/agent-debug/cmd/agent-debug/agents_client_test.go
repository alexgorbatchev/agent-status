package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
)

func TestServerFailureIdentifiesEndpoint(t *testing.T) {
	for _, kind := range []string{"connection refused", "TLS certificate", "deadline", "HTTP 503", "invalid JSON"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if kind == "HTTP 503" {
					w.WriteHeader(http.StatusServiceUnavailable)
				}
				_, _ = w.Write([]byte("private response"))
			})
			server := httptest.NewServer(handler)
			if kind == "TLS certificate" {
				server.Close()
				server = httptest.NewTLSServer(handler)
			}
			defer server.Close()
			if kind == "connection refused" {
				server.Close()
			}
			if kind == "deadline" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, time.Now().Add(-time.Second))
				defer cancel()
			}
			var result any
			err := fetchServerJSON(ctx, server.URL+"/monitor", "private-token", []string{"api", "v1", "agents", "codex", "usage"}, nil, "agent usage", &result)
			if err == nil || !strings.Contains(err.Error(), "GET "+server.URL+"/monitor/api/v1/agents/codex/usage") {
				t.Fatalf("failure omitted the attempted endpoint: %v", err)
			}
			if kind == "connection refused" && !strings.Contains(err.Error(), "connection refused") {
				t.Fatalf("network cause lost: %v", err)
			}
			if kind == "TLS certificate" && !strings.Contains(err.Error(), "certificate") {
				t.Fatalf("TLS cause lost: %v", err)
			}
			if strings.Contains(err.Error(), "private-token") || strings.Contains(err.Error(), "private response") {
				t.Fatalf("failure disclosed private data: %v", err)
			}
		})
	}
}

func TestMainLogsServerFailureOnce(t *testing.T) {
	previous := logging.Default()
	t.Cleanup(func() { logging.SetDefault(previous) })
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()
	for _, mode := range []string{"0", "1"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("AGENT", mode)
			var out, diag bytes.Buffer
			code := logging.RunCommand("agent-debug", &diag, func() error {
				cmd := newRootCmd()
				cmd.SetOut(&out)
				cmd.SetErr(&diag)
				cmd.SetArgs([]string{"agent", "usage", "codex", "--server-url", server.URL})
				return cmd.Execute()
			})
			if code != 1 || out.Len() != 0 || strings.Count(diag.String(), "command_failed") != 1 || !strings.Contains(diag.String(), server.URL) {
				t.Fatalf("failure must log once to stderr with endpoint: code=%d stdout=%s stderr=%s", code, &out, &diag)
			}
		})
	}
}
