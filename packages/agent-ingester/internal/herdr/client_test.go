package herdr

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type mockHerdrServer struct {
	listener net.Listener
	sockPath string
	t        *testing.T
}

func startMockHerdrServer(t *testing.T) *mockHerdrServer {
	t.Helper()
	// Use a short path under /tmp to respect macOS 104-char unix socket limit
	sockPath := fmt.Sprintf("/tmp/h-%d-%d.sock", os.Getpid(), time.Now().UnixNano()%100000)

	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to listen on mock unix socket: %v", err)
	}

	server := &mockHerdrServer{
		listener: l,
		sockPath: sockPath,
		t:        t,
	}

	go server.serve()
	return server
}

func (s *mockHerdrServer) close() {
	_ = s.listener.Close()
	_ = os.Remove(s.sockPath)
}

func (s *mockHerdrServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *mockHerdrServer) handleConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	reader := bufio.NewReader(conn)

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}

		var req struct {
			ID     string          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		var respData []byte
		switch req.Method {
		case "ping":
			respData, _ = json.Marshal(map[string]interface{}{
				"id":     req.ID,
				"result": map[string]interface{}{"ok": true},
			})
		case "pane.list":
			respData, _ = json.Marshal(map[string]interface{}{
				"id": req.ID,
				"result": map[string]interface{}{
					"panes": []map[string]interface{}{
						{
							"pane_id":      "w1:p1",
							"workspace_id": "w1",
							"tab_id":       "w1:t1",
							"title":        "Agent Terminal",
						},
						{
							"pane_id":      "w1:p2",
							"workspace_id": "w1",
							"tab_id":       "w1:t1",
							"title":        "Fallback Pane",
						},
					},
				},
			})
		case "pane.process_info":
			var p struct {
				PaneID string `json:"pane_id"`
			}
			_ = json.Unmarshal(req.Params, &p)
			switch p.PaneID {
			case "w1:p1":
				respData, _ = json.Marshal(map[string]interface{}{
					"id": req.ID,
					"result": map[string]interface{}{
						"process_info": map[string]interface{}{
							"pane_id": "w1:p1",
							"foreground_processes": []map[string]interface{}{
								{
									"pid":   12345,
									"name":  "node",
									"argv0": "pi",
									"cwd":   "/Users/dev/project",
								},
							},
						},
					},
				})
			case "w1:p2":
				respData, _ = json.Marshal(map[string]interface{}{
					"id": req.ID,
					"result": map[string]interface{}{
						"process_info": map[string]interface{}{
							"pane_id": "w1:p2",
							"foreground_processes": []map[string]interface{}{
								{
									"pid":   88888,
									"name":  "claude",
									"argv0": "claude",
									"cwd":   "/Users/dev/fallback-project",
								},
							},
						},
					},
				})
			default:
				respData, _ = json.Marshal(map[string]interface{}{
					"id": req.ID,
					"result": map[string]interface{}{
						"process_info": map[string]interface{}{
							"pane_id":              p.PaneID,
							"foreground_processes": []interface{}{},
						},
					},
				})
			}
		case "pane.focus":
			var p struct {
				PaneID string `json:"pane_id"`
			}
			_ = json.Unmarshal(req.Params, &p)
			if p.PaneID == "invalid-pane" {
				respData, _ = json.Marshal(map[string]interface{}{
					"id": req.ID,
					"error": map[string]interface{}{
						"code":    -32602,
						"message": "pane not found",
					},
				})
			} else {
				respData, _ = json.Marshal(map[string]interface{}{
					"id":     req.ID,
					"result": map[string]interface{}{"ok": true},
				})
			}
		default:
			respData, _ = json.Marshal(map[string]interface{}{
				"id": req.ID,
				"error": map[string]interface{}{
					"code":    -32601,
					"message": "method not found",
				},
			})
		}

		respData = append(respData, '\n')
		_, _ = conn.Write(respData)
	}
}

func TestClient_Ping_Success(t *testing.T) {
	srv := startMockHerdrServer(t)
	defer srv.close()

	client := NewClient(srv.sockPath)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.Ping(ctx)
	if err != nil {
		t.Fatalf("expected Ping to succeed, got: %v", err)
	}
}

func TestClient_Ping_SocketNotFound(t *testing.T) {
	client := NewClient("/nonexistent/path/to/herdr.sock")
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := client.Ping(ctx)
	if err == nil {
		t.Fatalf("expected Ping to fail on nonexistent socket, got nil")
	}
}

func TestClient_FindPaneByPID_ExactMatch(t *testing.T) {
	srv := startMockHerdrServer(t)
	defer srv.close()

	client := NewClient(srv.sockPath)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pane, err := client.FindPaneByPID(ctx, 12345, "/Users/dev/project", "pi")
	if err != nil {
		t.Fatalf("expected FindPaneByPID to succeed, got: %v", err)
	}
	if pane == nil {
		t.Fatalf("expected non-nil pane")
	}
	if pane.ID != "w1:p1" {
		t.Errorf("pane.ID = %q, want 'w1:p1'", pane.ID)
	}
	if pane.WorkspaceID != "w1" {
		t.Errorf("pane.WorkspaceID = %q, want 'w1'", pane.WorkspaceID)
	}
}

func TestClient_FindPaneByPID_FallbackCwdAndHarness(t *testing.T) {
	srv := startMockHerdrServer(t)
	defer srv.close()

	client := NewClient(srv.sockPath)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// PID 0 or non-matching PID -> fallback matches /Users/dev/fallback-project and "claude"
	pane, err := client.FindPaneByPID(ctx, 0, "/Users/dev/fallback-project", "claude")
	if err != nil {
		t.Fatalf("expected fallback matching to succeed, got: %v", err)
	}
	if pane == nil {
		t.Fatalf("expected non-nil pane")
	}
	if pane.ID != "w1:p2" {
		t.Errorf("pane.ID = %q, want 'w1:p2'", pane.ID)
	}
}

func TestClient_FindPaneByPID_NotFound(t *testing.T) {
	srv := startMockHerdrServer(t)
	defer srv.close()

	client := NewClient(srv.sockPath)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pane, err := client.FindPaneByPID(ctx, 99999, "/unknown/path", "unknown-harness")
	if err == nil {
		t.Fatalf("expected ErrPaneNotFound, got pane: %+v", pane)
	}
	if !strings.Contains(err.Error(), "pane not found") {
		t.Errorf("expected pane not found error, got: %v", err)
	}

	// Empty CWD or empty harness must not match "." or empty strings
	_, errEmptyCWD := client.FindPaneByPID(ctx, 0, "", "claude")
	if errEmptyCWD == nil {
		t.Errorf("expected error for empty cwd, got nil")
	}
	_, errEmptyHarness := client.FindPaneByPID(ctx, 0, "/Users/dev/fallback-project", "")
	if errEmptyHarness == nil {
		t.Errorf("expected error for empty harness, got nil")
	}
}

func TestClient_FocusPane_Success(t *testing.T) {
	srv := startMockHerdrServer(t)
	defer srv.close()

	client := NewClient(srv.sockPath)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.FocusPane(ctx, "w1:p1")
	if err != nil {
		t.Fatalf("expected FocusPane to succeed, got: %v", err)
	}
}

func TestClient_FocusPane_Error(t *testing.T) {
	srv := startMockHerdrServer(t)
	defer srv.close()

	client := NewClient(srv.sockPath)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.FocusPane(ctx, "invalid-pane")
	if err == nil {
		t.Fatalf("expected FocusPane to fail on invalid-pane, got nil")
	}
}

func TestDefaultSocketPath(t *testing.T) {
	t.Setenv("HERDR_SOCKET_PATH", "/custom/herdr.sock")
	if p := DefaultSocketPath(); p != "/custom/herdr.sock" {
		t.Errorf("expected /custom/herdr.sock, got %q", p)
	}

	t.Setenv("HERDR_SOCKET_PATH", "")
	t.Setenv("XDG_CONFIG_HOME", "/custom/xdg")
	if p := DefaultSocketPath(); p != "/custom/xdg/herdr/herdr.sock" {
		t.Errorf("expected /custom/xdg/herdr/herdr.sock, got %q", p)
	}

	t.Setenv("XDG_CONFIG_HOME", "")
	if p := DefaultSocketPath(); !strings.HasSuffix(p, filepath.Join(".config", "herdr", "herdr.sock")) {
		t.Errorf("expected ~/.config/herdr/herdr.sock, got %q", p)
	}
}

func TestNewClient_EmptyPath(t *testing.T) {
	t.Setenv("HERDR_SOCKET_PATH", "/default/test.sock")
	c := NewClient("")
	if c.socketPath != "/default/test.sock" {
		t.Errorf("expected /default/test.sock, got %q", c.socketPath)
	}
}

func TestNewClientWithDialer(t *testing.T) {
	called := false
	c := NewClientWithDialer(func(ctx context.Context) (net.Conn, error) {
		called = true
		c1, _ := net.Pipe()
		return c1, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = c.Ping(ctx)
	if !called {
		t.Errorf("expected custom dialer to be called")
	}
}

func TestClient_Dial_EmptySocketPath(t *testing.T) {
	c := &Client{socketPath: ""}
	ctx := context.Background()
	_, err := c.dial(ctx)
	if err == nil || !errors.Is(err, ErrHerdrUnavailable) {
		t.Errorf("expected ErrHerdrUnavailable, got %v", err)
	}
}

func TestClient_FindPaneByPID_ContextCanceled(t *testing.T) {
	srv := startMockHerdrServer(t)
	defer srv.close()

	client := NewClient(srv.sockPath)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.FindPaneByPID(ctx, 12345, "/Users/dev/project", "pi")
	if err == nil {
		t.Fatalf("expected error on canceled context, got nil")
	}
}

func TestClient_Call_RpcError(t *testing.T) {
	c := NewClientWithDialer(func(ctx context.Context) (net.Conn, error) {
		s1, s2 := net.Pipe()
		go func() {
			defer func() { _ = s2.Close() }()
			reader := bufio.NewReader(s2)
			line, _ := reader.ReadBytes('\n')
			var req map[string]interface{}
			_ = json.Unmarshal(line, &req)
			resp, _ := json.Marshal(map[string]interface{}{
				"id": req["id"],
				"error": map[string]interface{}{
					"code":    -32000,
					"message": "custom rpc error",
				},
			})
			resp = append(resp, '\n')
			_, _ = s2.Write(resp)
		}()
		return s1, nil
	})

	err := c.Ping(context.Background())
	if err == nil || !strings.Contains(err.Error(), "custom rpc error") {
		t.Errorf("expected custom rpc error, got %v", err)
	}
}

func TestActivateTerminalWindow(t *testing.T) {
	t.Setenv("HERDR_TERMINAL_APP", "NonExistentTestApp")
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	// Should execute and not panic
	activateTerminalWindow(ctx)

	t.Setenv("HERDR_TERMINAL_APP", "")
	t.Setenv("TERM_PROGRAM", "NonExistentTermProgram")
	activateTerminalWindow(ctx)

	t.Setenv("TERM_PROGRAM", "")
	activateTerminalWindow(ctx)
}
