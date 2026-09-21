package receiver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/classifier"
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
