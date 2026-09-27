package stream

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-server/internal/store"
)

type StoreReader interface {
	GetEnvironments(ctx context.Context) ([]types.IEnvironmentInfo, error)
	GetAgents(ctx context.Context, filter store.AgentFilter) ([]types.IAgentState, error)
}

type SubscriberConfig struct {
	SocketPath    string
	ReconnectWait time.Duration
	Logger        *logging.Logger
}

type Subscriber struct {
	cfg    SubscriberConfig
	store  StoreReader
	hub    *Hub
	logger *logging.Logger
}

func DefaultSocketPath() string {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = "/tmp"
	}
	return filepath.Join(runtimeDir, "agent-status", "notify.sock")
}

func NewSubscriber(cfg SubscriberConfig, store StoreReader, hub *Hub) *Subscriber {
	if cfg.SocketPath == "" {
		cfg.SocketPath = DefaultSocketPath()
	}
	if cfg.ReconnectWait <= 0 {
		cfg.ReconnectWait = 1 * time.Second
	}
	logger := cfg.Logger
	if logger == nil {
		logger = logging.With()
	}
	return &Subscriber{
		cfg:    cfg,
		store:  store,
		hub:    hub,
		logger: logger,
	}
}

// reconcileSnapshot queries SQLite to build a full snapshot of all active environments and agents,
// then broadcasts it to the hub via BroadcastSnapshot.
func (s *Subscriber) reconcileSnapshot(ctx context.Context) error {
	envs, err := s.store.GetEnvironments(ctx)
	if err != nil {
		return fmt.Errorf("reconciling environments: %w", err)
	}

	agents, err := s.store.GetAgents(ctx, store.AgentFilter{})
	if err != nil {
		return fmt.Errorf("reconciling agents: %w", err)
	}

	s.hub.BroadcastSnapshot(agents, envs)
	return nil
}

// Run starts the subscriber loop, connecting to the unix domain socket with automatic reconnect.
// It exits when ctx is canceled.
func (s *Subscriber) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		s.logger.InfoContext(ctx, "notify_socket_connecting", "path", s.cfg.SocketPath)
		conn, err := (&net.Dialer{}).DialContext(ctx, "unix", s.cfg.SocketPath)
		if err != nil {
			s.logger.DebugContext(ctx, "notify_socket_connect_failed", "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(s.cfg.ReconnectWait):
				continue
			}
		}

		s.logger.InfoContext(ctx, "notify_socket_connected")
		s.readLoop(ctx, conn)

		select {
		case <-ctx.Done():
			return
		case <-time.After(s.cfg.ReconnectWait):
		}
	}
}

func (s *Subscriber) readLoop(ctx context.Context, conn net.Conn) {
	defer func() {
		_ = conn.Close()
	}()

	// Close conn if context canceled
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	scanner := bufio.NewScanner(conn)
	// Allocate scanner buffer up to 16MB for large telemetry events
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 16*1024*1024)

	reconciled := false
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var event types.IServerEvent
		if err := json.Unmarshal(line, &event); err != nil {
			s.logger.WarnContext(ctx, "notify_event_malformed", "err", err, "line", string(line))
			continue
		}

		if event.Type == "hello" {
			s.logger.InfoContext(ctx, "notify_socket_ready")
			if err := s.reconcileSnapshot(ctx); err != nil {
				s.logger.ErrorContext(ctx, "notify_snapshot_resync_failed", "err", err)
			} else {
				reconciled = true
			}
			continue
		}

		if !reconciled {
			if err := s.reconcileSnapshot(ctx); err != nil {
				s.logger.ErrorContext(ctx, "notify_snapshot_resync_failed", "err", err)
			} else {
				reconciled = true
			}
		}

		s.hub.Broadcast(&event)
	}

	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
		s.logger.WarnContext(ctx, "notify_socket_read_error", "err", err)
	}
}
