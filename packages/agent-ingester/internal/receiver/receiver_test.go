package receiver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/classifier"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/herdr"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/notify"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/pricing"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/store"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/summarizer"
	"github.com/coder/websocket"
)

func newTestReceiver(t *testing.T) (*Receiver, *store.Store, *notify.Publisher) {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}

	priceEngine, err := pricing.NewPricingEngine()
	if err != nil {
		t.Fatalf("pricing engine: %v", err)
	}
	sumEngine, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer engine: %v", err)
	}

	sockPath := "/tmp/test-notify-" + t.Name() + ".sock"
	pub, err := notify.NewPublisher(sockPath)
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}

	cls := classifier.New(st, priceEngine, sumEngine)
	rcv := New(st, cls, pub, "test-token")

	t.Cleanup(func() {
		_ = pub.Close()
		_ = st.Close()
	})

	return rcv, st, pub
}

func TestReceiverHTTPRoutingAndAuth(t *testing.T) {
	rcv, _, _ := newTestReceiver(t)

	// 1. NotFound on wrong path
	req := httptest.NewRequest("GET", "/wrong-path", nil)
	w := httptest.NewRecorder()
	rcv.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want %d", w.Code, http.StatusNotFound)
	}

	// 2. Unauthorized on missing header
	req = httptest.NewRequest("GET", "/ws/ingest", nil)
	w = httptest.NewRecorder()
	rcv.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("got status %d on missing auth, want %d", w.Code, http.StatusUnauthorized)
	}

	// 3. Unauthorized on invalid bearer token
	req = httptest.NewRequest("GET", "/ws/ingest", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w = httptest.NewRecorder()
	rcv.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("got status %d on invalid token, want %d", w.Code, http.StatusUnauthorized)
	}

	// 4. Unauthorized on wrong scheme
	req = httptest.NewRequest("GET", "/ws/ingest", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	w = httptest.NewRecorder()
	rcv.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("got status %d on non-bearer scheme, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestReceiverWebSocketIngest(t *testing.T) {
	rcv, st, _ := newTestReceiver(t)

	mux := http.NewServeMux()
	mux.Handle("/ws/ingest", rcv)
	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/ingest"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer test-token"},
		},
	}

	conn, _, err := websocket.Dial(ctx, wsURL, opts)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer func() {
		_ = conn.Close(websocket.StatusNormalClosure, "done")
	}()

	// 1. Non-text message (binary) should be skipped cleanly
	if err := conn.Write(ctx, websocket.MessageBinary, []byte("binary data")); err != nil {
		t.Fatalf("writing binary message: %v", err)
	}

	// 2. Malformed JSON message returns malformed error
	if err := conn.Write(ctx, websocket.MessageText, []byte("{invalid-json")); err != nil {
		t.Fatalf("writing malformed message: %v", err)
	}
	_, errBytes, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("reading error response: %v", err)
	}
	var errMsg types.IIngesterMessage
	_ = json.Unmarshal(errBytes, &errMsg)
	if errMsg.Type != "error" || errMsg.Code != "malformed" {
		t.Errorf("expected malformed error message, got %+v", errMsg)
	}

	// 3. Hello message with dropped count
	helloMsg := types.IObserverMessage{
		Type:    "hello",
		EnvID:   "env1",
		Dropped: 5,
		Environment: &types.IEnvironmentInfo{
			ID:              "env1",
			Label:           "test-env",
			Hostname:        "localhost",
			Platform:        "darwin",
			User:            "test",
			LastHeartbeatAt: time.Now().UnixMilli(),
		},
	}
	helloBytes, _ := json.Marshal(helloMsg)
	if err := conn.Write(ctx, websocket.MessageText, helloBytes); err != nil {
		t.Fatalf("failed to write hello: %v", err)
	}

	// 4. Frames with unsupported version
	badFrameMsg := types.IObserverMessage{
		Type: "frames",
		Frames: []types.ITelemetryFrame{
			{
				Version: "0.9", // unsupported
				Seq:     1,
				EnvID:   "env1",
			},
		},
	}
	badFrameBytes, _ := json.Marshal(badFrameMsg)
	if err := conn.Write(ctx, websocket.MessageText, badFrameBytes); err != nil {
		t.Fatalf("failed to write bad frame: %v", err)
	}
	_, verErrBytes, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("reading version error: %v", err)
	}
	var verErrMsg types.IIngesterMessage
	_ = json.Unmarshal(verErrBytes, &verErrMsg)
	if verErrMsg.Type != "error" || verErrMsg.Code != "unsupported_version" {
		t.Errorf("expected unsupported_version error, got %+v", verErrMsg)
	}

	// 5. Valid frame ingestion and deduplication
	model := "claude-3-7-sonnet"
	frameMsg := types.IObserverMessage{
		Type: "frames",
		Frames: []types.ITelemetryFrame{
			{
				Version: "1.0",
				Seq:     1,
				EnvID:   "env1",
				Environment: types.IEnvironmentInfo{
					ID:              "env1",
					Label:           "test-env",
					Hostname:        "localhost",
					Platform:        "darwin",
					User:            "test",
					LastHeartbeatAt: time.Now().UnixMilli(),
				},
				AgentID:   "env1:claude-code:sess1",
				SessionID: "sess1",
				Harness:   types.HarnessClaudeCode,
				Timestamp: time.Now().UnixMilli(),
				EventType: "run_started",
				Data: types.TelemetryFrameData{
					Model: &model,
				},
			},
			// Duplicate frame seq 1 (should be skipped)
			{
				Version: "1.0",
				Seq:     1,
				EnvID:   "env1",
				Environment: types.IEnvironmentInfo{
					ID:              "env1",
					Label:           "test-env",
					Hostname:        "localhost",
					Platform:        "darwin",
					User:            "test",
					LastHeartbeatAt: time.Now().UnixMilli(),
				},
				AgentID:   "env1:claude-code:sess1",
				SessionID: "sess1",
				Harness:   types.HarnessClaudeCode,
				Timestamp: time.Now().UnixMilli(),
				EventType: "run_started",
				Data: types.TelemetryFrameData{
					Model: &model,
				},
			},
			// Second valid frame seq 2
			{
				Version: "1.0",
				Seq:     2,
				EnvID:   "env1",
				Environment: types.IEnvironmentInfo{
					ID:              "env1",
					Label:           "test-env",
					Hostname:        "localhost",
					Platform:        "darwin",
					User:            "test",
					LastHeartbeatAt: time.Now().UnixMilli(),
				},
				AgentID:   "env1:claude-code:sess1",
				SessionID: "sess1",
				Harness:   types.HarnessClaudeCode,
				Timestamp: time.Now().UnixMilli(),
				EventType: "turn_end",
				Data: types.TelemetryFrameData{
					Model: &model,
				},
			},
		},
	}
	frameBytes, _ := json.Marshal(frameMsg)
	if err := conn.Write(ctx, websocket.MessageText, frameBytes); err != nil {
		t.Fatalf("failed to write frame: %v", err)
	}

	// Read ack for highestSeq (2)
	_, ackBytes, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("failed to read ack: %v", err)
	}

	var ack types.IIngesterMessage
	if err := json.Unmarshal(ackBytes, &ack); err != nil {
		t.Fatalf("failed to parse ack: %v", err)
	}
	if ack.Type != "ack" || ack.UpTo != 2 {
		t.Fatalf("unexpected ack: %+v, want upTo 2", ack)
	}

	// Verify cursor was saved in database
	lastSeq, err := st.GetLastSeq("env1")
	if err != nil || lastSeq != 2 {
		t.Errorf("GetLastSeq(env1) = %d (err %v), want 2", lastSeq, err)
	}
}

func TestReceiverAckOnDeduplicatedBatch(t *testing.T) {
	rcv, st, _ := newTestReceiver(t)

	mux := http.NewServeMux()
	mux.Handle("/ws/ingest", rcv)
	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/ingest"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer test-token"},
		},
	}

	conn, _, err := websocket.Dial(ctx, wsURL, opts)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer func() {
		_ = conn.Close(websocket.StatusNormalClosure, "done")
	}()

	// 1. Hello message
	helloMsg := types.IObserverMessage{
		Type:  "hello",
		EnvID: "env1",
		Environment: &types.IEnvironmentInfo{
			ID:              "env1",
			Label:           "test-env",
			Hostname:        "localhost",
			Platform:        "darwin",
			User:            "test",
			LastHeartbeatAt: time.Now().UnixMilli(),
		},
	}
	helloBytes, _ := json.Marshal(helloMsg)
	if err := conn.Write(ctx, websocket.MessageText, helloBytes); err != nil {
		t.Fatalf("failed to write hello: %v", err)
	}

	// 2. Send valid frames seq 1, 2
	model := "claude-3-7-sonnet"
	frameMsg := types.IObserverMessage{
		Type: "frames",
		Frames: []types.ITelemetryFrame{
			{
				Version: "1.0",
				Seq:     1,
				EnvID:   "env1",
				Environment: types.IEnvironmentInfo{
					ID:              "env1",
					Label:           "test-env",
					Hostname:        "localhost",
					Platform:        "darwin",
					User:            "test",
					LastHeartbeatAt: time.Now().UnixMilli(),
				},
				AgentID:   "env1:claude-code:sess1",
				SessionID: "sess1",
				Harness:   types.HarnessClaudeCode,
				Timestamp: time.Now().UnixMilli(),
				EventType: "run_started",
				Data: types.TelemetryFrameData{
					Model: &model,
				},
			},
			{
				Version: "1.0",
				Seq:     2,
				EnvID:   "env1",
				Environment: types.IEnvironmentInfo{
					ID:              "env1",
					Label:           "test-env",
					Hostname:        "localhost",
					Platform:        "darwin",
					User:            "test",
					LastHeartbeatAt: time.Now().UnixMilli(),
				},
				AgentID:   "env1:claude-code:sess1",
				SessionID: "sess1",
				Harness:   types.HarnessClaudeCode,
				Timestamp: time.Now().UnixMilli(),
				EventType: "turn_end",
				Data: types.TelemetryFrameData{
					Model: &model,
				},
			},
		},
	}
	frameBytes, _ := json.Marshal(frameMsg)
	if err := conn.Write(ctx, websocket.MessageText, frameBytes); err != nil {
		t.Fatalf("failed to write frames: %v", err)
	}

	// Read initial ack (upTo 2)
	_, ackBytes, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("failed to read initial ack: %v", err)
	}
	var ack types.IIngesterMessage
	if err := json.Unmarshal(ackBytes, &ack); err != nil {
		t.Fatalf("failed to parse ack: %v", err)
	}
	if ack.Type != "ack" || ack.UpTo != 2 {
		t.Fatalf("unexpected ack: %+v, want upTo 2", ack)
	}

	// 3. Replay batch with ONLY deduplicated frames (seq 1, 2)
	dupMsg := types.IObserverMessage{
		Type: "frames",
		Frames: []types.ITelemetryFrame{
			{
				Version: "1.0",
				Seq:     1,
				EnvID:   "env1",
				Environment: types.IEnvironmentInfo{
					ID:              "env1",
					Label:           "test-env",
					Hostname:        "localhost",
					Platform:        "darwin",
					User:            "test",
					LastHeartbeatAt: time.Now().UnixMilli(),
				},
				AgentID:   "env1:claude-code:sess1",
				SessionID: "sess1",
				Harness:   types.HarnessClaudeCode,
				Timestamp: time.Now().UnixMilli(),
				EventType: "run_started",
				Data: types.TelemetryFrameData{
					Model: &model,
				},
			},
			{
				Version: "1.0",
				Seq:     2,
				EnvID:   "env1",
				Environment: types.IEnvironmentInfo{
					ID:              "env1",
					Label:           "test-env",
					Hostname:        "localhost",
					Platform:        "darwin",
					User:            "test",
					LastHeartbeatAt: time.Now().UnixMilli(),
				},
				AgentID:   "env1:claude-code:sess1",
				SessionID: "sess1",
				Harness:   types.HarnessClaudeCode,
				Timestamp: time.Now().UnixMilli(),
				EventType: "turn_end",
				Data: types.TelemetryFrameData{
					Model: &model,
				},
			},
		},
	}
	dupBytes, _ := json.Marshal(dupMsg)
	if err := conn.Write(ctx, websocket.MessageText, dupBytes); err != nil {
		t.Fatalf("failed to write duplicate frames: %v", err)
	}

	// Read ack for deduplicated batch - MUST return ack with upTo 2
	readCtx, readCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer readCancel()
	_, ackBytes2, err := conn.Read(readCtx)
	if err != nil {
		t.Fatalf("failed to read ack on deduplicated batch (ingester omitted ack): %v", err)
	}
	var ack2 types.IIngesterMessage
	if err := json.Unmarshal(ackBytes2, &ack2); err != nil {
		t.Fatalf("failed to parse ack2: %v", err)
	}
	if ack2.Type != "ack" || ack2.UpTo != 2 {
		t.Fatalf("unexpected ack on deduplicated batch: %+v, want upTo 2", ack2)
	}

	lastSeq, err := st.GetLastSeq("env1")
	if err != nil || lastSeq != 2 {
		t.Errorf("GetLastSeq(env1) = %d (err %v), want 2", lastSeq, err)
	}
}

func TestReceiverObserverRestartResetsCursor(t *testing.T) {
	rcv, st, _ := newTestReceiver(t)

	mux := http.NewServeMux()
	mux.Handle("/ws/ingest", rcv)
	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/ingest"

	opts := &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer test-token"},
		},
	}

	// First observer instance runs and reaches seq 5
	ctx1, cancel1 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel1()

	conn1, _, err := websocket.Dial(ctx1, wsURL, opts)
	if err != nil {
		t.Fatalf("websocket dial 1 failed: %v", err)
	}

	helloMsg1 := types.IObserverMessage{
		Type:  "hello",
		EnvID: "env1",
		Environment: &types.IEnvironmentInfo{
			ID:              "env1",
			Label:           "test-env",
			Hostname:        "localhost",
			Platform:        "darwin",
			User:            "test",
			LastHeartbeatAt: time.Now().UnixMilli(),
		},
	}
	helloBytes1, _ := json.Marshal(helloMsg1)
	if err := conn1.Write(ctx1, websocket.MessageText, helloBytes1); err != nil {
		t.Fatalf("failed to write hello 1: %v", err)
	}

	model := "claude-3-7-sonnet"
	frameMsg1 := types.IObserverMessage{
		Type: "frames",
		Frames: []types.ITelemetryFrame{
			{
				Version: "1.0",
				Seq:     5,
				EnvID:   "env1",
				Environment: types.IEnvironmentInfo{
					ID:              "env1",
					Label:           "test-env",
					Hostname:        "localhost",
					Platform:        "darwin",
					User:            "test",
					LastHeartbeatAt: time.Now().UnixMilli(),
				},
				AgentID:   "env1:claude-code:sess1",
				SessionID: "sess1",
				Harness:   types.HarnessClaudeCode,
				Timestamp: time.Now().UnixMilli(),
				EventType: "run_started",
				Data: types.TelemetryFrameData{
					Model: &model,
				},
			},
		},
	}
	frameBytes1, _ := json.Marshal(frameMsg1)
	if err := conn1.Write(ctx1, websocket.MessageText, frameBytes1); err != nil {
		t.Fatalf("failed to write frames 1: %v", err)
	}

	_, ackBytes1, err := conn1.Read(ctx1)
	if err != nil {
		t.Fatalf("failed to read ack 1: %v", err)
	}
	var ack1 types.IIngesterMessage
	_ = json.Unmarshal(ackBytes1, &ack1)
	if ack1.UpTo != 5 {
		t.Fatalf("want ack upTo 5, got %+v", ack1)
	}

	_ = conn1.Close(websocket.StatusNormalClosure, "restart")

	// Verify database cursor is 5
	lastSeq, err := st.GetLastSeq("env1")
	if err != nil || lastSeq != 5 {
		t.Fatalf("GetLastSeq(env1) = %d, want 5", lastSeq)
	}

	// Second observer instance starts freshly (restart), sends hello with Since = 0
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()

	conn2, _, err := websocket.Dial(ctx2, wsURL, opts)
	if err != nil {
		t.Fatalf("websocket dial 2 failed: %v", err)
	}
	defer func() {
		_ = conn2.Close(websocket.StatusNormalClosure, "done")
	}()

	helloMsg2 := types.IObserverMessage{
		Type:    "hello",
		EnvID:   "env1",
		Since:   0, // Fresh restart resets sequence
		Dropped: 0,
		Environment: &types.IEnvironmentInfo{
			ID:              "env1",
			Label:           "test-env",
			Hostname:        "localhost",
			Platform:        "darwin",
			User:            "test",
			LastHeartbeatAt: time.Now().UnixMilli(),
		},
	}
	helloBytes2, _ := json.Marshal(helloMsg2)
	if err := conn2.Write(ctx2, websocket.MessageText, helloBytes2); err != nil {
		t.Fatalf("failed to write hello 2: %v", err)
	}

	// New frames starting at seq 1 from restarted observer
	title := "New Session After Restart"
	frameMsg2 := types.IObserverMessage{
		Type: "frames",
		Frames: []types.ITelemetryFrame{
			{
				Version: "1.0",
				Seq:     1,
				EnvID:   "env1",
				Environment: types.IEnvironmentInfo{
					ID:              "env1",
					Label:           "test-env",
					Hostname:        "localhost",
					Platform:        "darwin",
					User:            "test",
					LastHeartbeatAt: time.Now().UnixMilli(),
				},
				AgentID:   "env1:claude-code:sess2",
				SessionID: "sess2",
				Harness:   types.HarnessClaudeCode,
				Timestamp: time.Now().UnixMilli(),
				EventType: "run_started",
				Data: types.TelemetryFrameData{
					Title: &title,
					Model: &model,
				},
			},
		},
	}
	frameBytes2, _ := json.Marshal(frameMsg2)
	if err := conn2.Write(ctx2, websocket.MessageText, frameBytes2); err != nil {
		t.Fatalf("failed to write frame after restart: %v", err)
	}

	readCtx2, readCancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer readCancel2()
	_, ackBytes2, err := conn2.Read(readCtx2)
	if err != nil {
		t.Fatalf("failed to read ack for restarted frame seq 1: %v", err)
	}
	var ack2 types.IIngesterMessage
	if err := json.Unmarshal(ackBytes2, &ack2); err != nil {
		t.Fatalf("failed to parse ack2: %v", err)
	}
	if ack2.Type != "ack" || ack2.UpTo != 1 {
		t.Fatalf("unexpected ack for restarted frame: %+v, want upTo 1", ack2)
	}

	// Verify agent state was created for sess2
	agent2, err := st.GetAgent("env1:claude-code:sess2")
	if err != nil || agent2 == nil {
		t.Fatalf("agent sess2 should have been created, got err=%v agent=%v", err, agent2)
	}
	if agent2.Activity.Title != title {
		t.Errorf("got title %q, want %q", agent2.Activity.Title, title)
	}

	// Verify cursor was updated to 1
	lastSeq2, err := st.GetLastSeq("env1")
	if err != nil || lastSeq2 != 1 {
		t.Errorf("GetLastSeq(env1) = %d (err %v), want 1", lastSeq2, err)
	}
}

func TestReceiverEnvNameTracking(t *testing.T) {
	rcv, _, _ := newTestReceiver(t)

	if got := rcv.getEnvName("unknown"); got != "" {
		t.Errorf("expected empty string for unknown env, got %q", got)
	}

	rcv.setEnvName("env-123", "work-macbook")
	if got := rcv.getEnvName("env-123"); got != "work-macbook" {
		t.Errorf("expected %q, got %q", "work-macbook", got)
	}

	// Empty checks
	rcv.setEnvName("", "something")
	rcv.setEnvName("env-456", "")
	if got := rcv.getEnvName(""); got != "" {
		t.Errorf("expected empty string for empty envID, got %q", got)
	}
	if got := rcv.getEnvName("env-456"); got != "" {
		t.Errorf("expected empty string for empty name, got %q", got)
	}

	// Nil envNames map initialization
	bareRcv := &Receiver{}
	bareRcv.setEnvName("env-nil", "Nil Map Box")
	if got := bareRcv.getEnvName("env-nil"); got != "Nil Map Box" {
		t.Errorf("bareRcv.getEnvName('env-nil') = %q, want 'Nil Map Box'", got)
	}
}

func TestReceiver_OversizedFrameBatch(t *testing.T) {
	rcv, st, _ := newTestReceiver(t)

	mux := http.NewServeMux()
	mux.Handle("/ws/ingest", rcv)
	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/ingest"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer test-token"},
		},
	}

	conn, _, err := websocket.Dial(ctx, wsURL, opts)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer func() {
		_ = conn.Close(websocket.StatusNormalClosure, "done")
	}()

	// Send hello frame (hello does not return an ACK)
	helloMsg := types.IObserverMessage{
		Type:  "hello",
		EnvID: "env-large",
		Environment: &types.IEnvironmentInfo{
			ID:              "env-large",
			Label:           "large-testbox",
			Hostname:        "large-testbox",
			Platform:        "darwin",
			User:            "testuser",
			LastHeartbeatAt: time.Now().UnixMilli(),
		},
	}
	helloBytes, err := json.Marshal(helloMsg)
	if err != nil {
		t.Fatalf("marshaling hello: %v", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, helloBytes); err != nil {
		t.Fatalf("writing hello: %v", err)
	}

	// Create an oversized frame (> 32 KiB, e.g. 128 KiB tool output / diff)
	// Default coder/websocket limit was 32768 bytes; this is ~100KB+
	largePayload := strings.Repeat("A large log line output containing agent telemetry bash output\n", 2000)
	toolName := "Bash"
	toolResult := largePayload

	largeFrame := types.ITelemetryFrame{
		Version:        "1.0",
		Seq:            1,
		EnvID:          "env-large",
		Environment:    *helloMsg.Environment,
		AgentID:        "agent-large-1",
		SessionID:      "sess-large-1",
		PID:            12345,
		Harness:        types.HarnessClaudeCode,
		HarnessVersion: "1.0.0",
		Project: types.IProjectInfo{
			ID:            "proj-large",
			Name:          "proj-large",
			CanonicalRoot: "/workspaces/large",
		},
		Workspace: types.IWorkspaceInfo{
			Branch: "main",
			CWD:    "/workspaces/large",
		},
		Timestamp: time.Now().UnixMilli(),
		EventType: "tool_result",
		Data: types.TelemetryFrameData{
			ToolName:   &toolName,
			ToolOutput: &toolResult,
		},
	}

	framesMsg := types.IObserverMessage{
		Type:   "frames",
		EnvID:  "env-large",
		Frames: []types.ITelemetryFrame{largeFrame},
	}
	framesBytes, err := json.Marshal(framesMsg)
	if err != nil {
		t.Fatalf("marshaling large frames: %v", err)
	}

	// Verify the payload actually exceeds 32 KiB
	if len(framesBytes) <= 32*1024 {
		t.Fatalf("payload size %d is not > 32 KiB", len(framesBytes))
	}

	// Send oversized message to receiver
	if err := conn.Write(ctx, websocket.MessageText, framesBytes); err != nil {
		t.Fatalf("writing oversized frame: %v", err)
	}

	// Read ack from receiver
	_, frameAckBytes, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("reading frame ack (receiver dropped oversized connection): %v", err)
	}
	var frameAck types.IIngesterMessage
	if err := json.Unmarshal(frameAckBytes, &frameAck); err != nil {
		t.Fatalf("unmarshaling frame ack: %v", err)
	}
	if frameAck.Type != "ack" || frameAck.UpTo != 1 {
		t.Fatalf("expected ack with UpTo=1, got %+v", frameAck)
	}

	// Verify agent was successfully saved in store
	agent, err := st.GetAgent("agent-large-1")
	if err != nil || agent == nil {
		t.Fatalf("agent should have been created in store, got: %v (err: %v)", agent, err)
	}
}

func TestReceiver_AbnormalCloseHandling(t *testing.T) {
	rcv, _, _ := newTestReceiver(t)

	mux := http.NewServeMux()
	mux.Handle("/ws/ingest", rcv)
	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/ingest"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer test-token"},
		},
	}

	conn, _, err := websocket.Dial(ctx, wsURL, opts)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}

	// Close abnormally with StatusPolicyViolation (neither normal nor going away)
	_ = conn.Close(websocket.StatusPolicyViolation, "simulated policy violation")
	time.Sleep(50 * time.Millisecond)
}

func TestReceiver_HelloWithHostnameOnlyAndEmptyLabel(t *testing.T) {
	rcv, _, _ := newTestReceiver(t)

	mux := http.NewServeMux()
	mux.Handle("/ws/ingest", rcv)
	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/ingest"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer test-token"},
		},
	}

	conn, _, err := websocket.Dial(ctx, wsURL, opts)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer func() {
		_ = conn.Close(websocket.StatusNormalClosure, "done")
	}()

	helloMsg := types.IObserverMessage{
		Type: "hello",
		Environment: &types.IEnvironmentInfo{
			ID:              "env-host-only",
			Label:           "",
			Hostname:        "host.local",
			Platform:        "linux",
			User:            "test",
			LastHeartbeatAt: time.Now().UnixMilli(),
		},
	}
	bytes, _ := json.Marshal(helloMsg)
	if err := conn.Write(ctx, websocket.MessageText, bytes); err != nil {
		t.Fatalf("failed to write hello: %v", err)
	}

	time.Sleep(20 * time.Millisecond)
	if name := rcv.getEnvName("env-host-only"); name != "host.local" {
		t.Errorf("expected hostname fallback 'host.local', got %q", name)
	}
}

func TestReceiver_UnsupportedFrameVersion(t *testing.T) {
	rcv, _, _ := newTestReceiver(t)

	mux := http.NewServeMux()
	mux.Handle("/ws/ingest", rcv)
	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/ingest"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer test-token"},
		},
	}

	conn, _, err := websocket.Dial(ctx, wsURL, opts)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer func() {
		_ = conn.Close(websocket.StatusNormalClosure, "done")
	}()

	msg := types.IObserverMessage{
		Type:  "frames",
		EnvID: "env-unsupp",
		Frames: []types.ITelemetryFrame{
			{
				Version: "2.0",
				Seq:     1,
				EnvID:   "env-unsupp",
			},
		},
	}
	bytes, _ := json.Marshal(msg)
	if err := conn.Write(ctx, websocket.MessageText, bytes); err != nil {
		t.Fatalf("failed to write frame: %v", err)
	}

	_, respBytes, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	var errMsg types.IIngesterMessage
	if err := json.Unmarshal(respBytes, &errMsg); err != nil {
		t.Fatalf("failed to parse error: %v", err)
	}
	if errMsg.Type != "error" || errMsg.Code != "unsupported_version" {
		t.Errorf("expected error code unsupported_version, got: %+v", errMsg)
	}

	// Write binary frame (should be ignored without breaking connection)
	_ = conn.Write(ctx, websocket.MessageBinary, []byte{0x01, 0x02})
}

func TestReceiver_HandleSetVisibility(t *testing.T) {
	rcv, st, _ := newTestReceiver(t)

	now := time.Now().UnixMilli()
	agent := types.IAgentState{
		ID:        "env-1:claude-code:sess-vis-api",
		SessionID: "sess-vis-api",
		Harness:   types.HarnessClaudeCode,
		Environment: types.IEnvironmentInfo{
			ID:              "env-1",
			Label:           "local",
			Hostname:        "dev",
			Platform:        "linux",
			User:            "user",
			LastHeartbeatAt: now,
		},
		Project: types.IProjectInfo{
			ID:            "proj",
			Name:          "proj",
			CanonicalRoot: "/work/proj",
		},
		Workspace: types.IWorkspaceInfo{
			Branch: "main",
			CWD:    "/work/proj",
		},
		Activity: types.IAgentActivity{
			Lifecycle:      types.LifecycleRunning,
			Activity:       types.ActivityIdle,
			Title:          "Visibility API Test",
			FirstStartedAt: now,
			UpdatedAt:      now,
		},
	}
	if err := st.SaveAgentState(agent); err != nil {
		t.Fatalf("SaveAgentState failed: %v", err)
	}

	// 1. Unauthorized without token
	body := strings.NewReader(`{"isHidden":true}`)
	req := httptest.NewRequest("PATCH", "/api/v1/internal/agents/env-1:claude-code:sess-vis-api/visibility", body)
	req.SetPathValue("id", "env-1:claude-code:sess-vis-api")
	w := httptest.NewRecorder()
	rcv.HandleSetVisibility(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("got status %d, want %d", w.Code, http.StatusUnauthorized)
	}

	// 2. 404 for non-existent agent
	body = strings.NewReader(`{"isHidden":true}`)
	req = httptest.NewRequest("PATCH", "/api/v1/internal/agents/non-existent/visibility", body)
	req.Header.Set("Authorization", "Bearer test-token")
	req.SetPathValue("id", "non-existent")
	w = httptest.NewRecorder()
	rcv.HandleSetVisibility(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want %d", w.Code, http.StatusNotFound)
	}

	// 3. 400 on malformed json
	body = strings.NewReader(`{invalid json`)
	req = httptest.NewRequest("PATCH", "/api/v1/internal/agents/env-1:claude-code:sess-vis-api/visibility", body)
	req.Header.Set("Authorization", "Bearer test-token")
	req.SetPathValue("id", "env-1:claude-code:sess-vis-api")
	w = httptest.NewRecorder()
	rcv.HandleSetVisibility(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", w.Code, http.StatusBadRequest)
	}

	// 4. Success hide
	body = strings.NewReader(`{"isHidden":true}`)
	req = httptest.NewRequest("PATCH", "/api/v1/internal/agents/env-1:claude-code:sess-vis-api/visibility", body)
	req.Header.Set("Authorization", "Bearer test-token")
	req.SetPathValue("id", "env-1:claude-code:sess-vis-api")
	w = httptest.NewRecorder()
	rcv.HandleSetVisibility(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", w.Code, http.StatusOK)
	}
	var resAgent types.IAgentState
	if err := json.NewDecoder(w.Body).Decode(&resAgent); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resAgent.IsHidden {
		t.Errorf("expected resAgent.IsHidden == true")
	}

	// 5. Success unhide
	body = strings.NewReader(`{"isHidden":false}`)
	req = httptest.NewRequest("PATCH", "/api/v1/internal/agents/env-1:claude-code:sess-vis-api/visibility", body)
	req.Header.Set("Authorization", "Bearer test-token")
	req.SetPathValue("id", "env-1:claude-code:sess-vis-api")
	w = httptest.NewRecorder()
	rcv.HandleSetVisibility(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", w.Code, http.StatusOK)
	}
	if err := json.NewDecoder(w.Body).Decode(&resAgent); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resAgent.IsHidden {
		t.Errorf("expected resAgent.IsHidden == false")
	}
}

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestReceiver_FramesBatchReceivedAndFrameIngestedLogLevel(t *testing.T) {
	rcv, _, _ := newTestReceiver(t)

	mux := http.NewServeMux()
	mux.Handle("/ws/ingest", rcv)
	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/ingest"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer test-token"},
		},
	}

	conn, _, err := websocket.Dial(ctx, wsURL, opts)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer func() {
		_ = conn.Close(websocket.StatusNormalClosure, "done")
	}()

	now := time.Now().UnixMilli()
	frame := types.ITelemetryFrame{
		Version:   "1.0",
		Seq:       100,
		EnvID:     "env-log-test",
		AgentID:   "env-log-test:pi:sess-1",
		SessionID: "sess-1",
		Harness:   types.HarnessPi,
		EventType: "tool_call",
		Timestamp: now,
		Environment: types.IEnvironmentInfo{
			ID:              "env-log-test",
			Label:           "test-env",
			Hostname:        "devbox",
			Platform:        "linux",
			User:            "user",
			LastHeartbeatAt: now,
		},
		Project: types.IProjectInfo{
			ID:            "proj-1",
			Name:          "proj",
			CanonicalRoot: "/work/proj",
		},
		Workspace: types.IWorkspaceInfo{
			Branch: "main",
			CWD:    "/work/proj",
		},
		Data: types.TelemetryFrameData{},
	}

	batchMsg := types.IObserverMessage{
		Type:   "frames",
		EnvID:  "env-log-test",
		Frames: []types.ITelemetryFrame{frame},
	}
	data, _ := json.Marshal(batchMsg)

	// Capture logs at INFO level
	var infoBuf safeBuffer
	infoLogger := logging.NewWithMode(&infoBuf, "ingester", "info", false)
	origLogger := logging.Default()
	logging.SetDefault(infoLogger)
	defer logging.SetDefault(origLogger)

	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("writing frame message: %v", err)
	}

	// Read ack from ingester
	_, ackBytes, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("reading ack response: %v", err)
	}
	var ackMsg types.IIngesterMessage
	if err := json.Unmarshal(ackBytes, &ackMsg); err != nil || ackMsg.Type != "ack" {
		t.Fatalf("expected ack message, got %+v", ackMsg)
	}

	// frames_batch_received and frame_ingested must NOT appear in INFO level logs
	if strings.Contains(infoBuf.String(), "frames_batch_received") {
		t.Errorf("frames_batch_received should not be logged at INFO level, got:\n%s", infoBuf.String())
	}
	if strings.Contains(infoBuf.String(), "frame_ingested") {
		t.Errorf("frame_ingested should not be logged at INFO level, got:\n%s", infoBuf.String())
	}

	// Now capture logs at DEBUG level
	var debugBuf safeBuffer
	debugLogger := logging.NewWithMode(&debugBuf, "ingester", "debug", false)
	logging.SetDefault(debugLogger)

	frame2 := frame
	frame2.Seq = 101
	batchMsg2 := types.IObserverMessage{
		Type:   "frames",
		EnvID:  "env-log-test",
		Frames: []types.ITelemetryFrame{frame2},
	}
	data2, _ := json.Marshal(batchMsg2)

	if err := conn.Write(ctx, websocket.MessageText, data2); err != nil {
		t.Fatalf("writing second frame message: %v", err)
	}

	_, ackBytes2, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("reading second ack response: %v", err)
	}
	if err := json.Unmarshal(ackBytes2, &ackMsg); err != nil || ackMsg.Type != "ack" {
		t.Fatalf("expected second ack message, got %+v", ackMsg)
	}

	// frames_batch_received and frame_ingested MUST appear in DEBUG level logs
	debugOutput := debugBuf.String()
	if !strings.Contains(debugOutput, "frames_batch_received") {
		t.Errorf("expected frames_batch_received in DEBUG level logs, got:\n%s", debugOutput)
	}
	if !strings.Contains(debugOutput, "count=1") {
		t.Errorf("expected count=1 in DEBUG level logs, got:\n%s", debugOutput)
	}
	if !strings.Contains(debugOutput, "frame_ingested") {
		t.Errorf("expected frame_ingested in DEBUG level logs, got:\n%s", debugOutput)
	}
}

func TestReceiver_HandleFocusAgent(t *testing.T) {
	var calledExecCmd string
	restoreExec := herdr.MockExecCommand(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		calledExecCmd = name
		return exec.CommandContext(ctx, "true")
	})
	defer restoreExec()

	rcv, st, _ := newTestReceiver(t)

	now := time.Now().UnixMilli()
	runningAgent := types.IAgentState{
		ID:        "env-1:claude-code:sess-focus",
		SessionID: "sess-focus",
		Harness:   types.HarnessClaudeCode,
		Environment: types.IEnvironmentInfo{
			ID:              "env-1",
			Label:           "local",
			Hostname:        "dev",
			Platform:        "darwin",
			User:            "user",
			LastHeartbeatAt: now,
		},
		Project: types.IProjectInfo{
			ID:            "proj",
			Name:          "proj",
			CanonicalRoot: "/work/proj",
		},
		Workspace: types.IWorkspaceInfo{
			Branch: "main",
			CWD:    "/work/proj",
		},
		Activity: types.IAgentActivity{
			Lifecycle:      types.LifecycleRunning,
			Activity:       types.ActivityBusy,
			Title:          "Focus Test",
			FirstStartedAt: now,
			UpdatedAt:      now,
			CurrentRun: &types.IRunInfo{
				ID:        "run-focus-1",
				PID:       54321,
				StartedAt: now,
			},
		},
	}
	if err := st.SaveAgentState(runningAgent); err != nil {
		t.Fatalf("SaveAgentState failed: %v", err)
	}

	exitedAgent := runningAgent
	exitedAgent.ID = "env-1:claude-code:sess-focus-exited"
	exitedAgent.SessionID = "sess-focus-exited"
	exitedAgent.Activity.Lifecycle = types.LifecycleExited
	exitedAgent.Activity.CurrentRun = nil
	if err := st.SaveAgentState(exitedAgent); err != nil {
		t.Fatalf("SaveAgentState(exitedAgent) failed: %v", err)
	}

	// 1. Unauthorized without token
	req := httptest.NewRequest("POST", "/api/v1/internal/agents/env-1:claude-code:sess-focus/focus", nil)
	req.SetPathValue("id", "env-1:claude-code:sess-focus")
	w := httptest.NewRecorder()
	rcv.HandleFocusAgent(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("got status %d, want %d", w.Code, http.StatusUnauthorized)
	}

	// 2. 404 for non-existent agent
	req = httptest.NewRequest("POST", "/api/v1/internal/agents/non-existent/focus", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	req.SetPathValue("id", "non-existent")
	w = httptest.NewRecorder()
	rcv.HandleFocusAgent(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want %d", w.Code, http.StatusNotFound)
	}

	// 3. 409 Conflict for exited agent
	req = httptest.NewRequest("POST", "/api/v1/internal/agents/env-1:claude-code:sess-focus-exited/focus", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	req.SetPathValue("id", "env-1:claude-code:sess-focus-exited")
	w = httptest.NewRecorder()
	rcv.HandleFocusAgent(w, req)
	if w.Code != http.StatusConflict {
		t.Errorf("got status %d, want %d", w.Code, http.StatusConflict)
	}

	// 4. 503 when Herdr socket is unavailable
	rcv.SetHerdrClient(herdr.NewClient("/nonexistent/herdr.sock"))
	req = httptest.NewRequest("POST", "/api/v1/internal/agents/env-1:claude-code:sess-focus/focus", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	req.SetPathValue("id", "env-1:claude-code:sess-focus")
	w = httptest.NewRecorder()
	rcv.HandleFocusAgent(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("got status %d, want %d", w.Code, http.StatusServiceUnavailable)
	}

	// 5. 200 OK when Herdr finds and focuses pane
	sockPath := fmt.Sprintf("/tmp/h-rcv-%d.sock", time.Now().UnixNano()%100000)
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to listen on unix socket: %v", err)
	}
	defer func() {
		_ = l.Close()
		_ = os.Remove(sockPath)
	}()

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				reader := bufio.NewReader(c)
				for {
					line, err := reader.ReadBytes('\n')
					if err != nil {
						return
					}
					var rpcReq struct {
						ID     string          `json:"id"`
						Method string          `json:"method"`
						Params json.RawMessage `json:"params"`
					}
					_ = json.Unmarshal(line, &rpcReq)
					var resp []byte
					switch rpcReq.Method {
					case "ping":
						resp, _ = json.Marshal(map[string]interface{}{"id": rpcReq.ID, "result": map[string]interface{}{"ok": true}})
					case "pane.list":
						resp, _ = json.Marshal(map[string]interface{}{
							"id": rpcReq.ID,
							"result": map[string]interface{}{
								"panes": []map[string]interface{}{
									{"pane_id": "w1:p1", "workspace_id": "w1", "tab_id": "w1:t1"},
								},
							},
						})
					case "pane.process_info":
						resp, _ = json.Marshal(map[string]interface{}{
							"id": rpcReq.ID,
							"result": map[string]interface{}{
								"process_info": map[string]interface{}{
									"pane_id": "w1:p1",
									"foreground_processes": []map[string]interface{}{
										{"pid": 54321, "name": "claude", "argv0": "claude", "cwd": "/work/proj"},
									},
								},
							},
						})
					case "pane.focus":
						resp, _ = json.Marshal(map[string]interface{}{"id": rpcReq.ID, "result": map[string]interface{}{"ok": true}})
					}
					resp = append(resp, '\n')
					_, _ = c.Write(resp)
				}
			}(conn)
		}
	}()

	rcv.SetHerdrClient(herdr.NewClient(sockPath))

	req = httptest.NewRequest("POST", "/api/v1/internal/agents/env-1:claude-code:sess-focus/focus", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	req.SetPathValue("id", "env-1:claude-code:sess-focus")
	w = httptest.NewRecorder()
	rcv.HandleFocusAgent(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	var res map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshaling focus response: %v", err)
	}
	if runtime.GOOS == "darwin" && calledExecCmd != "osascript" {
		t.Errorf("expected osascript to be called on darwin, got %q", calledExecCmd)
	}
	// 6. Method Not Allowed (GET)
	req = httptest.NewRequest("GET", "/api/v1/internal/agents/env-1:claude-code:sess-focus/focus", nil)
	w = httptest.NewRecorder()
	rcv.HandleFocusAgent(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("got status %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}

	// 7. Missing agent id
	req = httptest.NewRequest("POST", "/api/v1/internal/agents//focus", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	req.SetPathValue("id", "")
	w = httptest.NewRecorder()
	rcv.HandleFocusAgent(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", w.Code, http.StatusBadRequest)
	}

	// 8. Herdr pane not found (404)
	mockNotFoundClient := herdr.NewClientWithDialer(func(ctx context.Context) (net.Conn, error) {
		s1, s2 := net.Pipe()
		go func() {
			defer func() { _ = s2.Close() }()
			reader := bufio.NewReader(s2)
			for {
				line, err := reader.ReadBytes('\n')
				if err != nil {
					return
				}
				var rpcReq struct {
					ID     string `json:"id"`
					Method string `json:"method"`
				}
				_ = json.Unmarshal(line, &rpcReq)
				var resp []byte
				switch rpcReq.Method {
				case "ping":
					resp, _ = json.Marshal(map[string]interface{}{"id": rpcReq.ID, "result": map[string]interface{}{"ok": true}})
				case "pane.list":
					resp, _ = json.Marshal(map[string]interface{}{
						"id":     rpcReq.ID,
						"result": map[string]interface{}{"panes": []interface{}{}},
					})
				}
				resp = append(resp, '\n')
				_, _ = s2.Write(resp)
			}
		}()
		return s1, nil
	})
	rcv.SetHerdrClient(mockNotFoundClient)
	req = httptest.NewRequest("POST", "/api/v1/internal/agents/env-1:claude-code:sess-focus/focus", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	req.SetPathValue("id", "env-1:claude-code:sess-focus")
	w = httptest.NewRecorder()
	rcv.HandleFocusAgent(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want %d", w.Code, http.StatusNotFound)
	}

	// 9. Herdr focus error (500)
	mockFocusErrClient := herdr.NewClientWithDialer(func(ctx context.Context) (net.Conn, error) {
		s1, s2 := net.Pipe()
		go func() {
			defer func() { _ = s2.Close() }()
			reader := bufio.NewReader(s2)
			for {
				line, err := reader.ReadBytes('\n')
				if err != nil {
					return
				}
				var rpcReq struct {
					ID     string          `json:"id"`
					Method string          `json:"method"`
					Params json.RawMessage `json:"params"`
				}
				_ = json.Unmarshal(line, &rpcReq)
				var resp []byte
				switch rpcReq.Method {
				case "ping":
					resp, _ = json.Marshal(map[string]interface{}{"id": rpcReq.ID, "result": map[string]interface{}{"ok": true}})
				case "pane.list":
					resp, _ = json.Marshal(map[string]interface{}{
						"id": rpcReq.ID,
						"result": map[string]interface{}{
							"panes": []map[string]interface{}{
								{"pane_id": "w1:p1", "workspace_id": "w1", "tab_id": "w1:t1"},
							},
						},
					})
				case "pane.process_info":
					resp, _ = json.Marshal(map[string]interface{}{
						"id": rpcReq.ID,
						"result": map[string]interface{}{
							"process_info": map[string]interface{}{
								"pane_id": "w1:p1",
								"foreground_processes": []map[string]interface{}{
									{"pid": 54321, "name": "claude", "argv0": "claude", "cwd": "/work/proj"},
								},
							},
						},
					})
				case "pane.focus":
					resp, _ = json.Marshal(map[string]interface{}{
						"id":    rpcReq.ID,
						"error": map[string]interface{}{"code": -32000, "message": "focus failure"},
					})
				}
				resp = append(resp, '\n')
				_, _ = s2.Write(resp)
			}
		}()
		return s1, nil
	})
	rcv.SetHerdrClient(mockFocusErrClient)
	req = httptest.NewRequest("POST", "/api/v1/internal/agents/env-1:claude-code:sess-focus/focus", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	req.SetPathValue("id", "env-1:claude-code:sess-focus")
	w = httptest.NewRecorder()
	rcv.HandleFocusAgent(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want %d", w.Code, http.StatusInternalServerError)
	}
}
