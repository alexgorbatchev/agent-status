package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/cors"

	"github.com/alexgorbatchev/agent-status/packages/agent-server/internal/store"
	"github.com/alexgorbatchev/agent-status/packages/agent-server/internal/stream"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

type StoreInterface interface {
	GetStatus(ctx context.Context) (*types.StatusResponse, error)
	GetProjects(ctx context.Context) ([]types.ProjectGroup, error)
	GetEnvironments(ctx context.Context) ([]types.IEnvironmentInfo, error)
	GetAgents(ctx context.Context, filter store.AgentFilter) ([]types.IAgentState, error)
	GetAgentByID(ctx context.Context, id string) (*types.IAgentState, error)
	GetAgentLogs(ctx context.Context, agentID string, limit int, beforeID int64, subagentID string) ([]types.ITelemetryFrame, error)
}

type ServerConfig struct {
	Store       StoreInterface
	Hub         *stream.Hub
	AuthToken   string
	CORSOrigins []string
	Logger      *slog.Logger
}

type Server struct {
	cfg    ServerConfig
	store  StoreInterface
	hub    *stream.Hub
	logger *slog.Logger
	router chi.Router
}

func NewServer(cfg ServerConfig) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if len(cfg.CORSOrigins) == 0 {
		cfg.CORSOrigins = []string{"http://localhost:3000"}
	}

	s := &Server{
		cfg:    cfg,
		store:  cfg.Store,
		hub:    cfg.Hub,
		logger: cfg.Logger,
	}
	s.setupRoutes()
	return s
}

func (s *Server) Router() chi.Router {
	return s.router
}

func (s *Server) setupRoutes() {
	r := chi.NewRouter()

	// CORS options
	c := cors.New(cors.Options{
		AllowedOrigins:   s.cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	})

	r.Use(c.Handler)
	r.Use(chimiddleware.Recoverer)

	// Auth protection
	r.Use(BearerAuth(s.cfg.AuthToken))

	// Streaming endpoints
	r.Get("/api/v1/events", s.handleEventsSSE)
	r.Get("/ws/events", s.handleEventsWS)
	r.Get("/api/v1/agents/{id}/live-tail", s.handleAgentLiveTailSSE)

	// REST API with 15s timeout
	r.Group(func(r chi.Router) {
		r.Use(chimiddleware.Timeout(15 * time.Second))

		r.Get("/api/v1/status", s.handleGetStatus)
		r.Get("/api/v1/projects", s.handleGetProjects)
		r.Get("/api/v1/environments", s.handleGetEnvironments)
		r.Get("/api/v1/agents", s.handleGetAgents)
		r.Get("/api/v1/agents/{id}", s.handleGetAgentByID)
		r.Get("/api/v1/agents/{id}/logs", s.handleGetAgentLogs)
	})

	s.router = r
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.store.GetStatus(r.Context())
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to get status", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to get status")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleGetProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.store.GetProjects(r.Context())
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to get projects", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to get projects")
		return
	}
	writeJSON(w, http.StatusOK, projects)
}

func (s *Server) handleGetEnvironments(w http.ResponseWriter, r *http.Request) {
	envs, err := s.store.GetEnvironments(r.Context())
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to get environments", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to get environments")
		return
	}
	writeJSON(w, http.StatusOK, envs)
}

func (s *Server) handleGetAgents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := store.AgentFilter{
		Project:     q.Get("project"),
		Environment: q.Get("environment"),
		Lifecycle:   types.LifecycleState(q.Get("lifecycle")),
		Activity:    types.ActivityState(q.Get("activity")),
	}

	agents, err := s.store.GetAgents(r.Context(), filter)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to get agents", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to get agents")
		return
	}
	writeJSON(w, http.StatusOK, agents)
}

func (s *Server) handleGetAgentByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if unescaped, err := url.PathUnescape(id); err == nil && unescaped != "" {
		id = unescaped
	}
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing agent id")
		return
	}

	agent, err := s.store.GetAgentByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "agent not found")
			return
		}
		s.logger.ErrorContext(r.Context(), "failed to get agent by id", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to get agent")
		return
	}

	writeJSON(w, http.StatusOK, agent)
}

func (s *Server) handleGetAgentLogs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if unescaped, err := url.PathUnescape(id); err == nil && unescaped != "" {
		id = unescaped
	}
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing agent id")
		return
	}

	q := r.URL.Query()
	limit := 100
	if lStr := q.Get("limit"); lStr != "" {
		if parsed, err := strconv.Atoi(lStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	var beforeID int64
	if bStr := q.Get("before"); bStr != "" {
		if parsed, err := strconv.ParseInt(bStr, 10, 64); err == nil && parsed > 0 {
			beforeID = parsed
		}
	}

	subagentID := q.Get("subagent")

	logs, err := s.store.GetAgentLogs(r.Context(), id, limit, beforeID, subagentID)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to get agent logs", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to get agent logs")
		return
	}

	writeJSON(w, http.StatusOK, logs)
}

func (s *Server) handleEventsSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// 1. Synthesize and send initial snapshot from SQLite
	envs, err := s.store.GetEnvironments(r.Context())
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to load environments for sse snapshot", "err", err)
		return
	}
	agents, err := s.store.GetAgents(r.Context(), store.AgentFilter{})
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to load agents for sse snapshot", "err", err)
		return
	}

	snapshot := &types.IServerEvent{
		Type:         "snapshot",
		At:           time.Now().UnixMilli(),
		Agents:       agents,
		Environments: envs,
	}

	if data, err := json.Marshal(snapshot); err == nil {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	// 2. Subscribe to Hub events
	eventsCh := s.hub.SubscribeGlobal()
	defer s.hub.UnsubscribeGlobal(eventsCh)

	heartbeatTicker := time.NewTicker(stream.HeartbeatInterval)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeatTicker.C:
			_, _ = fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		case event, ok := <-eventsCh:
			if !ok {
				return
			}
			if data, err := json.Marshal(event); err == nil {
				_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}
		}
	}
}

func (s *Server) handleEventsWS(w http.ResponseWriter, r *http.Request) {
	opts := &websocket.AcceptOptions{
		OriginPatterns: s.cfg.CORSOrigins,
	}
	conn, err := websocket.Accept(w, r, opts)
	if err != nil {
		s.logger.WarnContext(r.Context(), "websocket accept failed", "err", err)
		return
	}
	defer conn.CloseNow()

	ctx := conn.CloseRead(r.Context())

	// 1. Synthesize and send initial snapshot
	envs, err := s.store.GetEnvironments(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to load environments for ws snapshot", "err", err)
		return
	}
	agents, err := s.store.GetAgents(ctx, store.AgentFilter{})
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to load agents for ws snapshot", "err", err)
		return
	}

	snapshot := &types.IServerEvent{
		Type:         "snapshot",
		At:           time.Now().UnixMilli(),
		Agents:       agents,
		Environments: envs,
	}

	if data, err := json.Marshal(snapshot); err == nil {
		if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
			return
		}
	}

	// 2. Subscribe to Hub events
	eventsCh := s.hub.SubscribeGlobal()
	defer s.hub.UnsubscribeGlobal(eventsCh)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-eventsCh:
			if !ok {
				return
			}
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
				return
			}
		}
	}
}

func (s *Server) handleAgentLiveTailSSE(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")
	if unescaped, err := url.PathUnescape(agentID); err == nil && unescaped != "" {
		agentID = unescaped
	}
	if agentID == "" {
		writeError(w, http.StatusBadRequest, "missing agent id")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	framesCh := s.hub.SubscribeAgentTail(agentID)
	defer s.hub.UnsubscribeAgentTail(agentID, framesCh)

	heartbeatTicker := time.NewTicker(stream.HeartbeatInterval)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeatTicker.C:
			_, _ = fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		case frame, ok := <-framesCh:
			if !ok {
				return
			}
			if data, err := json.Marshal(frame); err == nil {
				_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}
		}
	}
}
