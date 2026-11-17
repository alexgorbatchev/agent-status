package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	ingesterstore "github.com/alexgorbatchev/agent-status/packages/agent-ingester/store"
	"github.com/alexgorbatchev/agent-status/packages/agent-observer/environment"
	"github.com/alexgorbatchev/agent-status/packages/agent-server/store"
	"github.com/alexgorbatchev/agent-watcher/scanner"
	"github.com/shirou/gopsutil/v4/process"
)

func activeAgentsDB(t *testing.T) (string, int) {
	t.Helper()
	t.Setenv("AGENT_STATUS_TEST_MOCK_PROC", "0")
	path := filepath.Join(t.TempDir(), "agents.db")
	s, err := ingesterstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	localID, err := environment.ID()
	if err != nil {
		t.Fatal(err)
	}
	p, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	started, err := p.CreateTime()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	for _, env := range []types.IEnvironmentInfo{
		{ID: localID, Hostname: "local-host", LastHeartbeatAt: 1},
		{ID: "remote", Hostname: "remote-host", LastHeartbeatAt: now},
		{ID: "stale", Hostname: "stale-host", LastHeartbeatAt: 1},
	} {
		env.Platform = "linux"
		if err := s.UpsertEnvironment(env); err != nil {
			t.Fatal(err)
		}
	}
	// Use real current/dead PIDs, never the watcher's mock-process switch.
	cases := []struct {
		id, env, activity string
		pid               int
		start             int64
		ended             any
		current           bool
	}{
		{"local", localID, "IDLE", os.Getpid(), started, nil, true},
		{"waiting", localID, "WAITING_QUESTION", os.Getpid(), started, nil, true},
		{"dead", localID, "BUSY", 1 << 30, started, nil, true},
		{"reused", localID, "BUSY", os.Getpid(), started - 120000, nil, true},
		{"exited", localID, "IDLE", os.Getpid(), started, now, false},
		{"ended", localID, "IDLE", os.Getpid(), started, now, true},
		{"remote", "remote", "BUSY", 1 << 30, started, nil, true},
		{"stale", "stale", "BUSY", os.Getpid(), started, nil, true},
		{"invalid", "remote", "BUSY", 0, started, nil, true},
	}
	for _, c := range cases {
		var run any
		if c.current {
			run = c.id
		}
		execAgentSQL(t, s.DB(), `INSERT INTO runs (id, agent_id, pid, harness_version, started_at, ended_at) VALUES (?, ?, ?, '1', ?, ?)`,
			c.id, c.id, c.pid, c.start, c.ended)
		execAgentSQL(t, s.DB(), `INSERT INTO agents
			(id, session_id, environment_id, harness, project_id, project_name, canonical_root, branch, cwd, current_run_id, activity, title, first_started_at, updated_at)
			VALUES (?, ?, ?, 'codex', 'project', 'project', '/repo', 'main', ?, ?, ?, ?, ?, ?)`,
			c.id, c.id, c.env, "/repo with spaces", run, c.activity, "Fix\tbug\n\x1b[31mred\x1b[0m", started, now)
	}
	return path, os.Getpid()
}

func execAgentSQL(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func TestAgentList(t *testing.T) {
	path, pid := activeAgentsDB(t)
	s, err := store.OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/monitor/api/v1/agents" || r.URL.RawQuery != "lifecycle=RUNNING" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer test-api-token" {
			t.Error("missing bearer authorization")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		agents, err := s.GetAgents(r.Context(), store.AgentFilter{Lifecycle: types.LifecycleRunning})
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := json.NewEncoder(w).Encode(agents); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("AGENT_SERVER_HTTP_URL", server.URL+"/monitor/")
	t.Setenv("AGENT_STATUS_TOKEN", "test-api-token")
	t.Setenv("AGENT_DB_PATH", filepath.Join(t.TempDir(), "missing.db"))
	for _, mode := range []string{"0", "1"} {
		for _, all := range []bool{false, true} {
			t.Run(fmt.Sprintf("agent=%s/all=%t", mode, all), func(t *testing.T) {
				t.Setenv("AGENT", mode)
				cmd := newRootCmd()
				var out, diagnostics bytes.Buffer
				cmd.SetOut(&out)
				cmd.SetErr(&diagnostics)
				args := []string{"agent", "list", "--db", path}
				if all {
					args = []string{"agent", "list", "--all"}
				}
				cmd.SetArgs(args)
				if err := cmd.Execute(); err != nil {
					t.Fatal(err)
				}
				lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
				want := 2
				if all {
					want++
				}
				if len(lines) != want {
					t.Fatalf("got %d rows, want %d: %q", len(lines), want, out.String())
				}
				seen := map[string]bool{}
				for _, line := range lines {
					fields := strings.Split(line, "\t")
					if len(fields) != 6 {
						t.Fatalf("want six TSV fields: %q", line)
					}
					seen[fields[2]] = true
					wantPID, host := pid, "local-host"
					if fields[2] == "remote" {
						wantPID, host = 1<<30, "remote-host"
					}
					if fields[0] != fmt.Sprint(wantPID) || fields[1] != "codex" || fields[3] != host || fields[4] != "/repo with spaces" || fields[5] != "Fix bug red" {
						t.Fatalf("unexpected row: %q", line)
					}
				}
				if !seen["local"] || !seen["waiting"] || seen["remote"] != all {
					t.Fatalf("unexpected sessions: %v", seen)
				}
				if diagnostics.Len() != 0 {
					t.Fatalf("unexpected stderr: %s", &diagnostics)
				}
			})
		}
	}
}

func TestAgentListErrorsAndEmpty(t *testing.T) {
	path, _ := activeAgentsDB(t)
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"missing database", []string{"agent", "list", "--db", filepath.Join(t.TempDir(), "missing.db")}, "not found"},
		{"unexpected argument", []string{"agent", "list", "extra"}, "unknown command"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newRootCmd()
			var out, diagnostics bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&diagnostics)
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if out.Len() != 0 {
				t.Fatalf("stdout polluted: %q", out.String())
			}
		})
	}
	w, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close() }()
	execAgentSQL(t, w, "UPDATE agents SET current_run_id = NULL")
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"agent", "list", "--db", path})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("empty list wrote %q", out.String())
	}
}

type failedAgentWriter struct{}

func (failedAgentWriter) Write([]byte) (int, error) { return 0, errors.New("output closed") }

func TestAgentListWriteError(t *testing.T) {
	path, _ := activeAgentsDB(t)
	cmd := newRootCmd()
	cmd.SetOut(failedAgentWriter{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"agent", "list", "--db", path})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "output closed") {
		t.Fatalf("error = %v", err)
	}
}

func isolateLocalAgents(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for key, value := range map[string]string{
		"HOME": dir, "XDG_DATA_HOME": dir, "AGENT_DB_PATH": filepath.Join(dir, "missing.db"),
		"CLAUDE_SESSIONS_DIR":         filepath.Join(dir, "sessions"),
		"PI_SESSIONS_DIR":             filepath.Join(dir, "pi"),
		"CODEX_SESSIONS_DIR":          filepath.Join(dir, "codex"),
		"OPENCODE_DB_PATH":            filepath.Join(dir, "opencode.db"),
		"AGENT_STATUS_TEST_MOCK_PROC": "0",
	} {
		t.Setenv(key, value)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sessions"), 0755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestAgentListWithoutDatabase(t *testing.T) {
	dir := isolateLocalAgents(t)
	t.Setenv("AGENT_SERVER_HTTP_URL", "invalid remote URL ignored by local discovery")
	p, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	started, err := p.CreateTime()
	if err != nil {
		t.Fatal(err)
	}
	for _, desc := range []scanner.SessionDescriptor{
		{PID: os.Getpid(), StartedAt: started, SessionID: "live-session", CWD: "/repo with spaces", Name: "Fix\tbug\n\x1b[31mred\x1b[0m"},
		{PID: 1 << 30, StartedAt: started, SessionID: "dead-session", CWD: "/repo"},
		{PID: os.Getpid(), StartedAt: started - 120000, SessionID: "reused-session", CWD: "/repo"},
	} {
		data, err := json.Marshal(desc)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "sessions", desc.SessionID+".json"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"0", "1"} {
		t.Run("agent="+mode, func(t *testing.T) {
			t.Setenv("AGENT", mode)
			cmd := newRootCmd()
			var out, diagnostics bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&diagnostics)
			cmd.SetArgs([]string{"agent", "list"})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			want := fmt.Sprintf("%d\tclaude-code\tlive-session\t%s\t/repo with spaces\tFix bug red\n", os.Getpid(), host)
			if out.String() != want {
				t.Fatalf("stdout = %q, want %q", out.String(), want)
			}
			if diagnostics.Len() != 0 {
				t.Fatalf("stderr = %q", diagnostics.String())
			}
		})
	}
}

func TestAgentListLocalErrorsAndEmpty(t *testing.T) {
	isolateLocalAgents(t)
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"empty", []string{"agent", "list"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newRootCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if tc.want == "" && err != nil {
				t.Fatal(err)
			}
			if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("error = %v", err)
			}
			if out.Len() != 0 {
				t.Fatalf("stdout = %q", out.String())
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := discoverLocalAgents(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	for _, key := range []string{"PI_SESSIONS_DIR", "CODEX_SESSIONS_DIR", "OPENCODE_DB_PATH"} {
		t.Run(key, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "invalid")
			if err := os.WriteFile(path, []byte("invalid"), 0600); err != nil {
				t.Fatal(err)
			}
			if key == "CODEX_SESSIONS_DIR" {
				path += ".cycle"
				if err := os.Symlink(path, path); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv(key, path)
			cmd := newRootCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs([]string{"agent", "list"})
			if err := cmd.Execute(); err == nil {
				t.Fatal("expected discovery error")
			}
			if out.Len() != 0 {
				t.Fatalf("stdout = %q", out.String())
			}
		})
	}
}

func TestAgentListDiscoversHarnessProcesses(t *testing.T) {
	// Build one native process fixture, then run it under each harness name.
	// Real OS discovery and the library's session pairing are exercised together.
	root := t.TempDir()
	source := filepath.Join(root, "process.go")
	if err := os.WriteFile(source, []byte("package main\nimport \"time\"\nfunc main() { time.Sleep(time.Minute) }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0755); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(bin, "fixture")
	if out, err := exec.Command("go", "build", "-o", fixture, source).CombinedOutput(); err != nil {
		t.Fatalf("building process fixture: %v: %s", err, out)
	}
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	for _, harness := range []string{"pi", "codex", "opencode"} {
		t.Run(harness, func(t *testing.T) {
			dir := isolateLocalAgents(t)
			cwd, err := filepath.EvalSymlinks(dir)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(bin, harness)
			if err := os.WriteFile(path, data, 0700); err != nil {
				t.Fatal(err)
			}
			proc := exec.Command(path)
			proc.Dir = cwd
			if err := proc.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = proc.Process.Kill(); _ = proc.Wait() })
			now := time.Now().UTC().Format(time.RFC3339Nano)
			if harness == "opencode" {
				dbPath := createTestOpencodeDB(t, dir)
				t.Setenv("OPENCODE_DB_PATH", dbPath)
				w, err := sql.Open("sqlite3", dbPath)
				if err != nil {
					t.Fatal(err)
				}
				execAgentSQL(t, w, "UPDATE session SET directory = ?", cwd)
				if err := w.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				sessionDir := os.Getenv("CODEX_SESSIONS_DIR")
				var header any = map[string]any{"type": "session_meta", "payload": map[string]any{"id": "live-codex", "cwd": cwd, "timestamp": now}}
				if harness == "pi" {
					sessionDir = filepath.Join(os.Getenv("PI_SESSIONS_DIR"), scanner.EncodePiCWD(cwd))
					header = map[string]any{"type": "session", "id": "live-pi", "cwd": cwd, "timestamp": now}
				}
				if err := os.MkdirAll(sessionDir, 0755); err != nil {
					t.Fatal(err)
				}
				b, err := json.Marshal(header)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(sessionDir, "session.jsonl"), append(b, '\n'), 0644); err != nil {
					t.Fatal(err)
				}
			}
			cmd := newRootCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs([]string{"agent", "list"})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			fields := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\t")
			if len(fields) != 6 || fields[0] != fmt.Sprint(proc.Process.Pid) || fields[1] != harness || fields[2] == "" || fields[4] != cwd {
				t.Fatalf("unexpected local selection row: %q", out.String())
			}
		})
	}
}
