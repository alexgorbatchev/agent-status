package receiver

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/classifier"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/notify"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/store"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/types"
	"github.com/coder/websocket"
)

type Receiver struct {
	store       *store.Store
	classifier  *classifier.Classifier
	publisher   *notify.Publisher
	bearerToken string
	mu          sync.Mutex
}

func New(s *store.Store, c *classifier.Classifier, p *notify.Publisher, token string) *Receiver {
	return &Receiver{
		store:       s,
		classifier:  c,
		publisher:   p,
		bearerToken: token,
	}
}

func (r *Receiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path != "/ws/ingest" {
		http.NotFound(w, req)
		return
	}

	authHeader := req.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	providedToken := strings.TrimPrefix(authHeader, "Bearer ")
	if subtle.ConstantTimeCompare([]byte(providedToken), []byte(r.bearerToken)) != 1 {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := websocket.Accept(w, req, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		slog.Error("websocket accept error", "err", err)
		return
	}
	defer conn.Close(websocket.StatusInternalError, "closing connection")

	ctx := req.Context()
	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			if websocket.CloseStatus(err) != -1 {
				return
			}
			return
		}
		if typ != websocket.MessageText {
			continue
		}

		var msg types.IObserverMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			r.sendError(ctx, conn, "malformed", "invalid json payload")
			continue
		}

		switch msg.Type {
		case "hello":
			if msg.Environment != nil {
				_ = r.store.UpsertEnvironment(*msg.Environment)
				if msg.Dropped > 0 {
					_ = r.store.IncrementDroppedFrames(msg.EnvID, msg.Dropped)
				}
				if r.publisher != nil {
					r.publisher.Publish(types.IServerEvent{
						Type:        "environment_upsert",
						At:          time.Now().UnixMilli(),
						Environment: msg.Environment,
					})
				}
			}

		case "frames":
			var highestSeq int64 = 0
			var lastEnvID string

			for _, frame := range msg.Frames {
				if frame.Version != "1.0" {
					r.sendError(ctx, conn, "unsupported_version", "frame version must be 1.0")
					continue
				}

				lastEnvID = frame.EnvID
				lastSeq, err := r.store.GetLastSeq(frame.EnvID)
				if err == nil && frame.Seq <= lastSeq {
					// Deduplicate frame
					continue
				}

				// Process frame through deterministic classifier
				agent, err := r.classifier.ProcessFrame(frame)
				if err != nil {
					slog.Error("classifier error", "err", err, "agentId", frame.AgentID)
					continue
				}

				_ = r.store.SaveCursor(frame.EnvID, frame.Seq)
				if frame.Seq > highestSeq {
					highestSeq = frame.Seq
				}

				// Broadcast updates to server / UI
				if r.publisher != nil && agent != nil {
					now := time.Now().UnixMilli()
					r.publisher.Publish(types.IServerEvent{
						Type:  "agent_upsert",
						At:    now,
						Agent: agent,
					})
					r.publisher.Publish(types.IServerEvent{
						Type:    "log_append",
						At:      now,
						AgentID: agent.ID,
						Event:   &frame,
					})
				}
			}

			if highestSeq > 0 && lastEnvID != "" {
				r.sendAck(ctx, conn, highestSeq)
			}
		}
	}
}

func (r *Receiver) sendAck(ctx context.Context, conn *websocket.Conn, seq int64) {
	ack := types.IIngesterMessage{
		Type: "ack",
		UpTo: seq,
	}
	bytes, _ := json.Marshal(ack)
	_ = conn.Write(ctx, websocket.MessageText, bytes)
}

func (r *Receiver) sendError(ctx context.Context, conn *websocket.Conn, code, msg string) {
	errMsg := types.IIngesterMessage{
		Type:    "error",
		Code:    code,
		Message: msg,
	}
	bytes, _ := json.Marshal(errMsg)
	_ = conn.Write(ctx, websocket.MessageText, bytes)
}
