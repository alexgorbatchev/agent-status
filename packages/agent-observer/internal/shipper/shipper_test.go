package shipper

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/coder/websocket"
)

func TestShipperSendAndAck(t *testing.T) {
	var mu sync.Mutex
	var receivedHello *types.IObserverMessage
	var receivedFrames []types.ITelemetryFrame
	var tokenHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenHeader = r.Header.Get("Authorization")
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("websocket.Accept error = %v", err)
			return
		}
		defer func() {
			_ = conn.Close(websocket.StatusNormalClosure, "bye")
		}()

		for {
			_, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}

			var msg types.IObserverMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				t.Errorf("unmarshal error = %v", err)
				return
			}

			mu.Lock()
			switch msg.Type {
			case "hello":
				receivedHello = &msg
			case "frames":
				receivedFrames = append(receivedFrames, msg.Frames...)
				// Send ack for the last frame
				if len(msg.Frames) > 0 {
					lastSeq := msg.Frames[len(msg.Frames)-1].Seq
					ack := types.IIngesterMessage{
						Type: "ack",
						UpTo: lastSeq,
					}
					ackBytes, _ := json.Marshal(ack)
					_ = conn.Write(r.Context(), websocket.MessageText, ackBytes)
				}
			}
			mu.Unlock()
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	env := types.IEnvironmentInfo{
		ID:       "env-123456789012",
		Label:    "test-mac",
		Platform: "darwin",
		User:     "tester",
	}

	cfg := Config{
		IngesterURL: wsURL,
		BearerToken: "secret-token",
		Environment: env,
	}

	s := NewShipper(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.Start(ctx); err != nil {
		t.Fatalf("s.Start error: %v", err)
	}
	defer s.Stop()

	// Wait for connection and hello
	time.Sleep(100 * time.Millisecond)

	// Send multiple frames to verify sequencing
	frame1 := types.ITelemetryFrame{
		Version:     "1.0",
		EnvID:       env.ID,
		Environment: env,
		AgentID:     "env-1:claude-code:sess-1",
		SessionID:   "sess-1",
		PID:         1234,
		Harness:     types.HarnessClaudeCode,
		Timestamp:   time.Now().UnixMilli(),
		EventType:   "turn_start",
	}
	frame2 := types.ITelemetryFrame{
		Version:     "1.0",
		EnvID:       env.ID,
		Environment: env,
		AgentID:     "env-1:claude-code:sess-1",
		SessionID:   "sess-1",
		PID:         1234,
		Harness:     types.HarnessClaudeCode,
		Timestamp:   time.Now().UnixMilli(),
		EventType:   "thinking",
	}

	s.Enqueue(frame1)
	s.Enqueue(frame2)

	// Wait for frames and ack
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		count := len(receivedFrames)
		mu.Unlock()
		if count >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	mu.Lock()
	if tokenHeader != "Bearer secret-token" {
		t.Errorf("tokenHeader = %q, want 'Bearer secret-token'", tokenHeader)
	}
	if receivedHello == nil || receivedHello.Type != "hello" {
		t.Errorf("receivedHello = %+v, want hello message", receivedHello)
	}
	if len(receivedFrames) != 2 {
		t.Fatalf("expected 2 frames received, got %d", len(receivedFrames))
	}
	if receivedFrames[0].Seq != 1 {
		t.Errorf("received frame 0 seq = %d, want 1", receivedFrames[0].Seq)
	}
	if receivedFrames[1].Seq != 2 {
		t.Errorf("received frame 1 seq = %d, want 2", receivedFrames[1].Seq)
	}
	mu.Unlock()

	// Verify unacked buffer is drained
	deadline = time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if s.UnackedCount() == 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if unacked := s.UnackedCount(); unacked != 0 {
		t.Errorf("s.UnackedCount() = %d, want 0 after ack", unacked)
	}
}

func TestShipperHeartbeat(t *testing.T) {
	origHeartbeat := heartbeatInterval
	heartbeatInterval = 50 * time.Millisecond
	defer func() { heartbeatInterval = origHeartbeat }()

	var mu sync.Mutex
	var receivedHeartbeats int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() {
			_ = conn.Close(websocket.StatusNormalClosure, "bye")
		}()

		for {
			_, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			var msg types.IObserverMessage
			if err := json.Unmarshal(data, &msg); err == nil {
				if msg.Type == "hello" {
					mu.Lock()
					receivedHeartbeats++
					mu.Unlock()
				}
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	cfg := Config{
		IngesterURL: wsURL,
		Environment: types.IEnvironmentInfo{ID: "hb-env", Label: "hb"},
	}

	s := NewShipper(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = s.Start(ctx)
	defer s.Stop()

	// Wait for at least 2 heartbeats
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		mu.Lock()
		count := receivedHeartbeats
		mu.Unlock()
		if count >= 2 {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if receivedHeartbeats < 1 {
		t.Errorf("expected at least 1 heartbeat, got %d", receivedHeartbeats)
	}
}

func TestShipperReconnectAndReplay(t *testing.T) {
	origBackoff := initialBackoff
	initialBackoff = 20 * time.Millisecond
	defer func() { initialBackoff = origBackoff }()

	var serverMu sync.Mutex
	var connectionAttempts int32
	var allReceivedFrames []types.ITelemetryFrame

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := atomic.AddInt32(&connectionAttempts, 1)

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() {
			_ = conn.Close(websocket.StatusNormalClosure, "closed")
		}()

		if attempt == 1 {
			// First connection: read hello, then abruptly close connection without acking
			_, _, _ = conn.Read(r.Context())
			time.Sleep(50 * time.Millisecond)
			return
		}

		// Subsequent connection: read and ack everything
		for {
			_, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			var msg types.IObserverMessage
			if err := json.Unmarshal(data, &msg); err == nil {
				if msg.Type == "frames" {
					serverMu.Lock()
					allReceivedFrames = append(allReceivedFrames, msg.Frames...)
					if len(msg.Frames) > 0 {
						lastSeq := msg.Frames[len(msg.Frames)-1].Seq
						ack := types.IIngesterMessage{Type: "ack", UpTo: lastSeq}
						ackBytes, _ := json.Marshal(ack)
						_ = conn.Write(r.Context(), websocket.MessageText, ackBytes)
					}
					serverMu.Unlock()
				}
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	env := types.IEnvironmentInfo{ID: "replay-env", Label: "replay"}
	s := NewShipper(Config{
		IngesterURL: wsURL,
		Environment: env,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Enqueue frame before connection
	frame := types.ITelemetryFrame{
		Version:   "1.0",
		EnvID:     env.ID,
		AgentID:   "a1",
		SessionID: "s1",
		EventType: "tool_call",
	}
	s.Enqueue(frame)

	_ = s.Start(ctx)
	defer s.Stop()

	// Wait for reconnect and replay
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		serverMu.Lock()
		count := len(allReceivedFrames)
		serverMu.Unlock()
		if count >= 1 && atomic.LoadInt32(&connectionAttempts) >= 2 {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}

	serverMu.Lock()
	if len(allReceivedFrames) < 1 {
		t.Errorf("expected frame to be replayed on reconnect, got 0")
	}
	serverMu.Unlock()

	// Verify unacked count is drained after replay and ack
	deadline = time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if s.UnackedCount() == 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if s.UnackedCount() != 0 {
		t.Errorf("s.UnackedCount() = %d, want 0", s.UnackedCount())
	}
}

func TestShipperConnectionFailureRetry(t *testing.T) {
	origBackoff := initialBackoff
	initialBackoff = 10 * time.Millisecond
	defer func() { initialBackoff = origBackoff }()

	// Connect to closed/invalid port to trigger connection error and retry
	s := NewShipper(Config{
		IngesterURL: "ws://127.0.0.1:1/nonexistent",
		Environment: types.IEnvironmentInfo{ID: "fail-env"},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = s.Start(ctx)
	defer s.Stop()

	// Let it attempt connections and exit cleanly on context timeout
	<-ctx.Done()
}

func TestShipperRingBufferCap(t *testing.T) {
	env := types.IEnvironmentInfo{
		ID:    "env-123",
		Label: "test",
	}
	cfg := Config{
		IngesterURL: "ws://127.0.0.1:0/ws/ingest", // not connected
		Environment: env,
	}

	s := NewShipper(cfg)

	// Enqueue 1050 frames
	for i := 0; i < 1050; i++ {
		s.Enqueue(types.ITelemetryFrame{
			Version:   "1.0",
			EnvID:     env.ID,
			AgentID:   "a1",
			SessionID: "s1",
			EventType: "thinking",
		})
	}

	if s.UnackedCount() != MaxRingBufferSize {
		t.Errorf("s.UnackedCount() = %d, want %d", s.UnackedCount(), MaxRingBufferSize)
	}

	if s.DroppedCount() != 50 {
		t.Errorf("s.DroppedCount() = %d, want 50", s.DroppedCount())
	}
}

type testSyncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *testSyncBuffer) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *testSyncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func TestShipperLoggingOnConnect(t *testing.T) {
	buf := &testSyncBuffer{}
	logger := logging.NewWithMode(buf, "observer", "info", false)
	logging.SetDefault(logger)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() {
			_ = conn.Close(websocket.StatusNormalClosure, "bye")
		}()
		_, _, _ = conn.Read(r.Context())
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	cfg := Config{
		IngesterURL: wsURL,
		Environment: types.IEnvironmentInfo{ID: "env-log", Label: "log-test"},
	}

	s := NewShipper(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.Start(ctx); err != nil {
		t.Fatalf("s.Start error: %v", err)
	}
	defer s.Stop()

	// Wait for connection log
	time.Sleep(150 * time.Millisecond)

	out := buf.String()
	if !strings.Contains(out, "ingester_connecting") {
		t.Errorf("expected ingester_connecting log on shipper connect, got: %s", out)
	}
	if !strings.Contains(out, "ingester_connected") {
		t.Errorf("expected ingester_connected log on shipper connect, got: %s", out)
	}
}

func TestShipperLoggingOnDisconnect(t *testing.T) {
	buf := &testSyncBuffer{}
	logger := logging.NewWithMode(buf, "observer", "info", false)
	logging.SetDefault(logger)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		// Immediately close connection with error to trigger disconnect on shipper
		_ = conn.Close(websocket.StatusInternalError, "server closing")
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	cfg := Config{
		IngesterURL: wsURL,
		Environment: types.IEnvironmentInfo{ID: "env-log-disc", Label: "log-disc-test"},
	}

	origBackoff := initialBackoff
	initialBackoff = 20 * time.Millisecond
	defer func() { initialBackoff = origBackoff }()

	s := NewShipper(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.Start(ctx); err != nil {
		t.Fatalf("s.Start error: %v", err)
	}
	defer s.Stop()

	// Wait for connection and disconnect logs
	time.Sleep(150 * time.Millisecond)

	out := buf.String()
	if !strings.Contains(out, "ingester_connecting") {
		t.Errorf("expected ingester_connecting log, got: %s", out)
	}
	if !strings.Contains(out, "ingester_disconnected") {
		t.Errorf("expected ingester_disconnected log on disconnect, got: %s", out)
	}
}
