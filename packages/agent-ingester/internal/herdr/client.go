package herdr

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

var (
	ErrPaneNotFound     = errors.New("pane not found")
	ErrHerdrUnavailable = errors.New("herdr service unavailable")
)

type PaneInfo struct {
	ID          string `json:"paneId"`
	WorkspaceID string `json:"workspaceId"`
	TabID       string `json:"tabId"`
	Title       string `json:"title,omitempty"`
}

type ProcessInfo struct {
	PID   int    `json:"pid"`
	Name  string `json:"name"`
	Argv0 string `json:"argv0"`
	CWD   string `json:"cwd"`
}

type Client struct {
	socketPath string
	dialer     func(ctx context.Context) (net.Conn, error)
	reqSeq     uint64
}

func DefaultSocketPath() string {
	if p := os.Getenv("HERDR_SOCKET_PATH"); p != "" {
		return p
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "herdr", "herdr.sock")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "herdr", "herdr.sock")
}

func NewClient(socketPath string) *Client {
	if socketPath == "" {
		socketPath = DefaultSocketPath()
	}
	return &Client{socketPath: socketPath}
}

func NewClientWithDialer(dialer func(ctx context.Context) (net.Conn, error)) *Client {
	return &Client{dialer: dialer}
}

func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	if c.dialer != nil {
		return c.dialer(ctx)
	}

	if c.socketPath == "" {
		return nil, ErrHerdrUnavailable
	}

	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", c.socketPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrHerdrUnavailable, err)
	}
	return conn, nil
}

type rpcRequest struct {
	ID     string      `json:"id"`
	Method string      `json:"method"`
	Params interface{} `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

func (c *Client) call(ctx context.Context, method string, params interface{}, result interface{}) error {
	conn, err := c.dial(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	reqID := fmt.Sprintf("req-%d", atomic.AddUint64(&c.reqSeq, 1))
	reqBody := rpcRequest{
		ID:     reqID,
		Method: method,
		Params: params,
	}
	if reqBody.Params == nil {
		reqBody.Params = map[string]interface{}{}
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshaling rpc request: %w", err)
	}
	data = append(data, '\n')

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	}

	if _, err := conn.Write(data); err != nil {
		return fmt.Errorf("writing rpc request: %w", err)
	}

	reader := bufio.NewReader(conn)
	respLine, err := reader.ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("reading rpc response: %w", err)
	}

	var resp rpcResponse
	if err := json.Unmarshal(respLine, &resp); err != nil {
		return fmt.Errorf("unmarshaling rpc response: %w", err)
	}

	if resp.Error != nil {
		return fmt.Errorf("herdr rpc error (code %d): %s", resp.Error.Code, resp.Error.Message)
	}

	if result != nil && len(resp.Result) > 0 {
		if err := json.Unmarshal(resp.Result, result); err != nil {
			return fmt.Errorf("unmarshaling rpc result: %w", err)
		}
	}

	return nil
}

func (c *Client) Ping(ctx context.Context) error {
	return c.call(ctx, "ping", nil, nil)
}

type paneListResult struct {
	Panes []struct {
		PaneID      string `json:"pane_id"`
		WorkspaceID string `json:"workspace_id"`
		TabID       string `json:"tab_id"`
		Title       string `json:"title"`
	} `json:"panes"`
}

type processInfoResult struct {
	ProcessInfo struct {
		PaneID              string        `json:"pane_id"`
		ForegroundProcesses []ProcessInfo `json:"foreground_processes"`
	} `json:"process_info"`
	ForegroundProcesses []ProcessInfo `json:"foreground_processes"`
}

func (c *Client) FindPaneByPID(ctx context.Context, pid int, cwd, harness string) (*PaneInfo, error) {
	var listRes paneListResult
	if err := c.call(ctx, "pane.list", nil, &listRes); err != nil {
		var rawList []struct {
			PaneID      string `json:"pane_id"`
			WorkspaceID string `json:"workspace_id"`
			TabID       string `json:"tab_id"`
			Title       string `json:"title"`
		}
		if err2 := c.call(ctx, "pane.list", nil, &rawList); err2 != nil {
			return nil, fmt.Errorf("listing herdr panes: %w", err)
		}
		listRes.Panes = rawList
	}

	if len(listRes.Panes) == 0 {
		return nil, ErrPaneNotFound
	}

	var fallbackPane *PaneInfo
	cleanCWD := filepath.Clean(cwd)
	harnessLower := strings.ToLower(harness)

	for _, p := range listRes.Panes {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		infoParams := map[string]interface{}{"pane_id": p.PaneID}
		var pInfo processInfoResult
		if err := c.call(ctx, "pane.process_info", infoParams, &pInfo); err != nil {
			continue
		}

		procs := pInfo.ProcessInfo.ForegroundProcesses
		if len(procs) == 0 && len(pInfo.ForegroundProcesses) > 0 {
			procs = pInfo.ForegroundProcesses
		}

		currentPane := &PaneInfo{
			ID:          p.PaneID,
			WorkspaceID: p.WorkspaceID,
			TabID:       p.TabID,
			Title:       p.Title,
		}

		for _, proc := range procs {
			// Primary match by PID
			if pid > 0 && proc.PID == pid {
				return currentPane, nil
			}

			// Fallback match by CWD + harness name/argv0
			if cleanCWD != "" && filepath.Clean(proc.CWD) == cleanCWD {
				procArg := strings.ToLower(proc.Argv0)
				procName := strings.ToLower(proc.Name)
				if strings.Contains(procArg, harnessLower) || strings.Contains(procName, harnessLower) {
					if fallbackPane == nil {
						fallbackPane = currentPane
					}
				}
			}
		}
	}

	if fallbackPane != nil {
		return fallbackPane, nil
	}

	return nil, ErrPaneNotFound
}

func (c *Client) FocusPane(ctx context.Context, paneID string) error {
	params := map[string]interface{}{
		"pane_id": paneID,
	}
	var res map[string]interface{}
	if err := c.call(ctx, "pane.focus", params, &res); err != nil {
		return err
	}

	// On macOS, optionally bring terminal window to front
	if runtime.GOOS == "darwin" {
		activateTerminalWindow(ctx)
	}

	return nil
}

func activateTerminalWindow(ctx context.Context) {
	termApp := os.Getenv("HERDR_TERMINAL_APP")
	if termApp == "" {
		// Common macOS terminal applications
		termApp = os.Getenv("TERM_PROGRAM")
	}
	if termApp == "" {
		return
	}

	script := fmt.Sprintf(`tell application "%s" to activate`, termApp)
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	_ = cmd.Run()
}
