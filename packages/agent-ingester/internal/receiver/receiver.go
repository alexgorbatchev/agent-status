package receiver

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/classifier"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/herdr"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/notify"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/store"
	"github.com/coder/websocket"
)

type Receiver struct {
	store       *store.Store
	classifier  *classifier.Classifier
	publisher   *notify.Publisher
	bearerToken string
	mu          sync.RWMutex
	envNames    map[string]string
	herdrClient *herdr.Client
}

const maxMessageReadLimit = 64 * 1024 * 1024 // 64 MiB

func New(s *store.Store, c *classifier.Classifier, p *notify.Publisher, token string) *Receiver {
	return &Receiver{
		store:       s,
		classifier:  c,
		publisher:   p,
		bearerToken: token,
		envNames:    make(map[string]string),
		herdrClient: herdr.NewClient(""),
	}
}

func (r *Receiver) SetHerdrClient(c *herdr.Client) {
	r.herdrClient = c
}

func (r *Receiver) setEnvName(envID, name string) {
	if envID == "" || name == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.envNames == nil {
		r.envNames = make(map[string]string)
	}
	r.envNames[envID] = name
}

func (r *Receiver) getEnvName(envID string) string {
	if envID == "" {
		return ""
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.envNames[envID]
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
		logging.Error("ws_accept_error", "err", err)
		return
	}
	conn.SetReadLimit(maxMessageReadLimit)
	logging.Debug("observer_connected", "remoteAddr", req.RemoteAddr)
	defer func() {
		logging.Debug("observer_disconnected", "remoteAddr", req.RemoteAddr)
		_ = conn.Close(websocket.StatusInternalError, "closing connection")
	}()

	ctx := req.Context()
	var connEnvID string
	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			closeStatus := websocket.CloseStatus(err)
			if closeStatus != websocket.StatusNormalClosure && closeStatus != websocket.StatusGoingAway && ctx.Err() == nil {
				logging.Warn("observer_read_error", "err", err, "remoteAddr", req.RemoteAddr)
			}
			return
		}
		if typ != websocket.MessageText {
			continue
		}

		var msg types.IObserverMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			logging.Warn("observer_message_malformed", "err", err)
			r.sendError(ctx, conn, "malformed", "invalid json payload")
			continue
		}

		switch msg.Type {
		case "hello":
			connEnvID = msg.EnvID
			if connEnvID == "" && msg.Environment != nil {
				connEnvID = msg.Environment.ID
			}
			envName := connEnvID
			if msg.Environment != nil {
				if msg.Environment.Label != "" {
					envName = msg.Environment.Label
				} else if msg.Environment.Hostname != "" {
					envName = msg.Environment.Hostname
				}
			}
			if connEnvID != "" && envName != "" {
				r.setEnvName(connEnvID, envName)
			}
			logging.Debug("hello_received", "env", envName, "envId", connEnvID, "dropped", msg.Dropped, "since", msg.Since)
			if connEnvID != "" && msg.Since == 0 {
				_ = r.store.SaveCursor(connEnvID, 0)
			}
			if msg.Environment != nil {
				_ = r.store.UpsertEnvironment(*msg.Environment)
				if msg.Dropped > 0 {
					_ = r.store.IncrementDroppedFrames(connEnvID, msg.Dropped)
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
			envID := msg.EnvID
			if envID == "" && len(msg.Frames) > 0 {
				envID = msg.Frames[0].EnvID
			}
			if envID == "" {
				envID = connEnvID
			}
			envName := r.getEnvName(envID)
			if envName == "" {
				envName = envID
			}
			logging.Debug("frames_batch_received", "env", envName, "envId", envID, "count", len(msg.Frames))
			var highestSeq int64 = 0
			lastEnvID := envID

			for _, frame := range msg.Frames {
				if frame.Version != "1.0" {
					logging.Warn("frame_version_unsupported", "version", frame.Version, "envId", frame.EnvID)
					r.sendError(ctx, conn, "unsupported_version", "frame version must be 1.0")
					continue
				}

				if frame.EnvID != "" {
					lastEnvID = frame.EnvID
				} else {
					frame.EnvID = lastEnvID
				}
				if frame.Seq > highestSeq {
					highestSeq = frame.Seq
				}

				lastSeq, err := r.store.GetLastSeq(frame.EnvID)
				if err == nil && frame.Seq <= lastSeq {
					// Deduplicate frame
					logging.Debug("frame_deduplicated", "envId", frame.EnvID, "seq", frame.Seq, "lastSeq", lastSeq)
					continue
				}

				frameEnvName := frame.Environment.Label
				if frameEnvName == "" {
					frameEnvName = frame.Environment.Hostname
				}
				if frameEnvName == "" {
					frameEnvName = r.getEnvName(frame.EnvID)
				}
				if frameEnvName != "" && frame.EnvID != "" {
					r.setEnvName(frame.EnvID, frameEnvName)
				}

				logging.Debug("frame_ingested",
					"project", frame.Project.Name,
					"env", frameEnvName,
					"envId", frame.EnvID,
					"harness", frame.Harness,
					"sessionId", frame.SessionID,
					"agentId", frame.AgentID,
					"seq", frame.Seq,
					"eventType", frame.EventType,
				)

				// Process frame through deterministic classifier
				agent, err := r.classifier.ProcessFrame(frame)
				if err != nil {
					logging.Error("classifier_error", "err", err, "agentId", frame.AgentID)
					continue
				}

				_ = r.store.SaveCursor(frame.EnvID, frame.Seq)

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
				logging.Debug("ack_sent", "envId", lastEnvID, "upTo", highestSeq)
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

type visibilityRequest struct {
	IsHidden bool `json:"isHidden"`
}

func (r *Receiver) HandleFocusAgent(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if r.bearerToken != "" {
		auth := req.Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(auth, prefix) || subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(auth, prefix)), []byte(r.bearerToken)) != 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "unauthorized"})
			return
		}
	}

	id := req.PathValue("id")
	if id == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "missing agent id"})
		return
	}

	agent, err := r.store.GetAgent(id)
	if err != nil || agent == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "agent not found"})
		return
	}

	if agent.Activity.Lifecycle == types.LifecycleExited {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "agent is exited"})
		return
	}

	client := r.herdrClient
	if client == nil {
		client = herdr.NewClient("")
	}

	ctx := req.Context()
	if err := client.Ping(ctx); err != nil {
		logging.Warn("herdr_ping_failed", "err", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "herdr multiplexer daemon is not running"})
		return
	}

	pid := 0
	if agent.Activity.CurrentRun != nil {
		pid = agent.Activity.CurrentRun.PID
	}

	pane, err := client.FindPaneByPID(ctx, pid, agent.Workspace.CWD, string(agent.Harness))
	if err != nil {
		if errors.Is(err, herdr.ErrPaneNotFound) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "no matching herdr pane found for agent"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprintf("locating herdr pane: %v", err)})
		return
	}

	if err := client.FocusPane(ctx, pane.ID); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprintf("focusing herdr pane: %v", err)})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":          true,
		"paneId":      pane.ID,
		"workspaceId": pane.WorkspaceID,
		"tabId":       pane.TabID,
	})
}

func (r *Receiver) HandleSetVisibility(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPatch && req.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if r.bearerToken != "" {
		authHeader := req.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(r.bearerToken)) != 1 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	id := req.PathValue("id")
	if id == "" {
		http.Error(w, "missing agent id", http.StatusBadRequest)
		return
	}

	var body visibilityRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	now := time.Now().UnixMilli()
	var hiddenAt *int64
	if body.IsHidden {
		hiddenAt = &now
	}

	agent, err := r.store.SetAgentVisibility(id, body.IsHidden, hiddenAt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if r.publisher != nil && agent != nil {
		r.publisher.Publish(types.IServerEvent{
			Type:  "agent_upsert",
			At:    now,
			Agent: agent,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(agent)
}

func (r *Receiver) HandleDismissCompletion(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPatch && req.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if r.bearerToken != "" {
		authHeader := req.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(r.bearerToken)) != 1 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	id := req.PathValue("id")
	if id == "" {
		http.Error(w, "missing agent id", http.StatusBadRequest)
		return
	}

	var agent *types.IAgentState
	var err error
	if r.classifier != nil {
		agent, err = r.classifier.DismissCompletion(id)
	} else {
		agent, err = r.store.DismissAgentCompletion(id)
		if err == nil && r.publisher != nil && agent != nil {
			r.publisher.Broadcast(&types.IServerEvent{
				Type:  "agent_upsert",
				At:    time.Now().UnixMilli(),
				Agent: agent,
			})
		}
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(agent)
}

type patchAgentRequest struct {
	IsHidden              *bool `json:"isHidden"`
	DismissCompletion     *bool `json:"dismissCompletion"`
	AcknowledgeCompletion *bool `json:"acknowledgeCompletion"`
}

func (r *Receiver) HandlePatchAgent(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPatch && req.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if r.bearerToken != "" {
		authHeader := req.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(r.bearerToken)) != 1 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	id := req.PathValue("id")
	if id == "" {
		http.Error(w, "missing agent id", http.StatusBadRequest)
		return
	}

	var body patchAgentRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var agent *types.IAgentState
	var err error

	if (body.DismissCompletion != nil && *body.DismissCompletion) || (body.AcknowledgeCompletion != nil && *body.AcknowledgeCompletion) {
		if r.classifier != nil {
			agent, err = r.classifier.DismissCompletion(id)
		} else {
			agent, err = r.store.DismissAgentCompletion(id)
			if err == nil && r.publisher != nil && agent != nil {
				r.publisher.Broadcast(&types.IServerEvent{
					Type:  "agent_upsert",
					At:    time.Now().UnixMilli(),
					Agent: agent,
				})
			}
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
	}

	if body.IsHidden != nil {
		now := time.Now().UnixMilli()
		var hiddenAt *int64
		if *body.IsHidden {
			hiddenAt = &now
		}
		agent, err = r.store.SetAgentVisibility(id, *body.IsHidden, hiddenAt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if r.publisher != nil {
			event := types.IServerEvent{
				Type:  "agent_upsert",
				At:    now,
				Agent: agent,
			}
			r.publisher.Broadcast(&event)
		}
	}

	if agent == nil {
		http.Error(w, "no changes applied", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(agent)
}
