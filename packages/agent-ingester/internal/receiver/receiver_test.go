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
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/pricing"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/store"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/summarizer"
	"github.com/coder/websocket"
)

func TestReceiverWebSocketIngest(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	defer st.Close()

	priceEngine, err := pricing.NewPricingEngine()
	if err != nil {
		t.Fatalf("pricing engine: %v", err)
	}
	sumEngine, err := summarizer.New(summarizer.Config{NoSummarize: true})
	if err != nil {
		t.Fatalf("summarizer engine: %v", err)
	}

	cls := classifier.New(st, priceEngine, sumEngine)
	rcv := New(st, cls, nil, "test-token")

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

	// Send hello
	helloMsg := types.IObserverMessage{
		Type:  "hello",
		EnvID: "env1",
		Environment: &types.IEnvironmentInfo{
			ID:       "env1",
			Label:    "test-env",
			Hostname: "localhost",
			Platform: "darwin",
			User:     "test",
		},
	}
	helloBytes, _ := json.Marshal(helloMsg)
	if err := conn.Write(ctx, websocket.MessageText, helloBytes); err != nil {
		t.Fatalf("failed to write hello: %v", err)
	}

	// Send frame
	model := "claude-3-7-sonnet"
	frameMsg := types.IObserverMessage{
		Type: "frames",
		Frames: []types.ITelemetryFrame{
			{
				Version: "1.0",
				Seq:     1,
				EnvID:   "env1",
				Environment: types.IEnvironmentInfo{
					ID:       "env1",
					Label:    "test-env",
					Hostname: "localhost",
					Platform: "darwin",
					User:     "test",
				},
				AgentID:   "env1:claude-code:sess1",
				SessionID: "sess1",
				Harness:   types.HarnessClaudeCode,
				Timestamp: time.Now().UnixMilli(),
				EventType: "session_start",
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

	// Read ack
	_, ackBytes, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("failed to read ack: %v", err)
	}

	var ack types.IIngesterMessage
	if err := json.Unmarshal(ackBytes, &ack); err != nil {
		t.Fatalf("failed to parse ack: %v", err)
	}
	if ack.Type != "ack" || ack.UpTo != 1 {
		t.Fatalf("unexpected ack: %+v", ack)
	}
}
