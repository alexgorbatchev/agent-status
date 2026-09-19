package shipper

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-observer/internal/types"
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
		defer conn.Close(websocket.StatusNormalClosure, "bye")

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
			if msg.Type == "hello" {
				receivedHello = &msg
			} else if msg.Type == "frames" {
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

	// Send a frame
	frame := types.ITelemetryFrame{
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

	s.Enqueue(frame)

	// Wait for frame and ack
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		count := len(receivedFrames)
		mu.Unlock()
		if count > 0 {
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
	if len(receivedFrames) != 1 {
		t.Fatalf("expected 1 frame received, got %d", len(receivedFrames))
	}
	if receivedFrames[0].Seq != 1 {
		t.Errorf("received frame seq = %d, want 1", receivedFrames[0].Seq)
	}
	mu.Unlock()

	// Verify unacked buffer is drained
	if unacked := s.UnackedCount(); unacked != 0 {
		t.Errorf("s.UnackedCount() = %d, want 0 after ack", unacked)
	}
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
