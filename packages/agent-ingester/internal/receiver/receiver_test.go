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
		pub.Close()
		st.Close()
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
	defer conn.Close(websocket.StatusNormalClosure, "done")

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
