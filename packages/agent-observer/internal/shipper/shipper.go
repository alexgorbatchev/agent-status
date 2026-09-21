package shipper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/cenkalti/backoff/v4"
	"github.com/coder/websocket"
)

const (
	DefaultDispatchBufferSize = 100
)

var (
	ErrShipperStopped       = errors.New("shipper stopped")
	heartbeatInterval       = 10 * time.Second
	initialBackoff          = 500 * time.Millisecond
	sustainedHealthDuration = 5 * time.Second
	backoffRandomization    = 0.5
)

type AckCallback func()

type inFlightItem struct {
	frame *types.ITelemetryFrame
	onAck AckCallback
}

type Config struct {
	IngesterURL string
	BearerToken string
	Environment types.IEnvironmentInfo
	BufferSize  int
}

type Shipper struct {
	cfg Config

	mu           sync.Mutex
	lastSeq      int64
	lastAckedSeq int64
	ackCallbacks map[int64]AckCallback

	incomingCh chan inFlightItem
	stopCh     chan struct{}
	once       sync.Once
}

func NewShipper(cfg Config) *Shipper {
	bufSize := cfg.BufferSize
	if bufSize <= 0 {
		bufSize = DefaultDispatchBufferSize
	}
	return &Shipper{
		cfg:          cfg,
		ackCallbacks: make(map[int64]AckCallback),
		incomingCh:   make(chan inFlightItem, bufSize),
		stopCh:       make(chan struct{}),
	}
}

// EnqueueWithAck enqueues a frame for delivery, applying backpressure if the dispatch channel is full.
// When an ACK from the ingester covering frame.Seq is received, onAck is invoked.
func (s *Shipper) EnqueueWithAck(ctx context.Context, frame types.ITelemetryFrame, onAck AckCallback) error {
	s.mu.Lock()
	s.lastSeq++
	frame.Seq = s.lastSeq
	if onAck != nil {
		s.ackCallbacks[frame.Seq] = onAck
	}
	frameCopy := frame
	s.mu.Unlock()

	item := inFlightItem{
		frame: &frameCopy,
		onAck: onAck,
	}

	select {
	case s.incomingCh <- item:
		return nil
	case <-ctx.Done():
		if onAck != nil {
			s.mu.Lock()
			delete(s.ackCallbacks, frame.Seq)
			s.mu.Unlock()
		}
		return ctx.Err()
	case <-s.stopCh:
		if onAck != nil {
			s.mu.Lock()
			delete(s.ackCallbacks, frame.Seq)
			s.mu.Unlock()
		}
		return ErrShipperStopped
	}
}

// EnqueueContext enqueues a frame with context cancellation support.
func (s *Shipper) EnqueueContext(ctx context.Context, frame types.ITelemetryFrame) error {
	return s.EnqueueWithAck(ctx, frame, nil)
}

// Enqueue enqueues a frame without blocking on context cancellation.
func (s *Shipper) Enqueue(frame types.ITelemetryFrame) {
	_ = s.EnqueueWithAck(context.Background(), frame, nil)
}

// UnackedCount returns the number of frames currently in-flight awaiting ACK.
func (s *Shipper) UnackedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.ackCallbacks)
}

func (s *Shipper) Start(ctx context.Context) error {
	go s.runConnectionLoop(ctx)
	return nil
}

func (s *Shipper) Stop() {
	s.once.Do(func() {
		close(s.stopCh)
	})
}

func (s *Shipper) runConnectionLoop(ctx context.Context) {
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = initialBackoff
	expBackoff.Multiplier = 2.0
	expBackoff.MaxInterval = 30 * time.Second
	expBackoff.RandomizationFactor = backoffRandomization
	expBackoff.Reset()

	var backoffMu sync.Mutex
	resetBackoff := func() {
		backoffMu.Lock()
		expBackoff.Reset()
		backoffMu.Unlock()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		default:
		}

		logging.Info("ingester_connecting", "ingesterUrl", s.cfg.IngesterURL)
		err := s.connectAndStream(ctx, resetBackoff)
		if err != nil && ctx.Err() == nil {
			select {
			case <-s.stopCh:
			default:
				logging.Warn("ingester_disconnected", "ingesterUrl", s.cfg.IngesterURL, "err", err)
			}
		}

		backoffMu.Lock()
		nextInterval := expBackoff.NextBackOff()
		backoffMu.Unlock()
		if nextInterval == backoff.Stop {
			nextInterval = 30 * time.Second
		}

		select {
		case <-time.After(nextInterval):
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		}
	}
}

func (s *Shipper) connectAndStream(ctx context.Context, onConnected func()) error {
	connCtx, connCancel := context.WithCancel(ctx)
	defer connCancel()

	opts := &websocket.DialOptions{}
	if s.cfg.BearerToken != "" {
		opts.HTTPHeader = http.Header{
			"Authorization": []string{"Bearer " + s.cfg.BearerToken},
		}
	}

	conn, _, err := websocket.Dial(connCtx, s.cfg.IngesterURL, opts)
	if err != nil {
		return fmt.Errorf("dialing ingester %q: %w", s.cfg.IngesterURL, err)
	}
	defer func() {
		_ = conn.Close(websocket.StatusNormalClosure, "shutdown")
	}()

	var resetOnce sync.Once
	markHealthy := func() {
		if onConnected != nil {
			resetOnce.Do(onConnected)
		}
	}

	// 1. Send Hello Frame
	s.mu.Lock()
	envCopy := s.cfg.Environment
	helloMsg := types.IObserverMessage{
		Type:        "hello",
		EnvID:       envCopy.ID,
		Environment: &envCopy,
		Since:       s.lastAckedSeq,
		Dropped:     0,
	}
	s.mu.Unlock()

	helloBytes, err := json.Marshal(helloMsg)
	if err != nil {
		return fmt.Errorf("marshaling hello message: %w", err)
	}

	if err := conn.Write(connCtx, websocket.MessageText, helloBytes); err != nil {
		return fmt.Errorf("writing hello frame: %w", err)
	}

	logging.Info("ingester_connected", "ingesterUrl", s.cfg.IngesterURL)

	// 2. Start reader goroutine for acks and invoke callbacks
	errCh := make(chan error, 2)
	go func() {
		for {
			_, data, err := conn.Read(connCtx)
			if err != nil {
				errCh <- err
				return
			}

			var ack types.IIngesterMessage
			if err := json.Unmarshal(data, &ack); err == nil && ack.Type == "ack" {
				markHealthy()
				if ack.UpTo > 0 {
					s.mu.Lock()
					s.lastAckedSeq = max(s.lastAckedSeq, ack.UpTo)
					var toInvoke []AckCallback
					for seq, cb := range s.ackCallbacks {
						if seq <= ack.UpTo {
							if cb != nil {
								toInvoke = append(toInvoke, cb)
							}
							delete(s.ackCallbacks, seq)
						}
					}
					s.mu.Unlock()

					for _, cb := range toInvoke {
						cb()
					}
				}
			}
		}
	}()

	heartbeatTicker := time.NewTicker(heartbeatInterval)
	defer heartbeatTicker.Stop()

	var healthTimerCh <-chan time.Time
	if sustainedHealthDuration > 0 {
		healthTimer := time.NewTimer(sustainedHealthDuration)
		defer healthTimer.Stop()
		healthTimerCh = healthTimer.C
	} else {
		markHealthy()
	}

	// 3. Stream incoming frames and heartbeats
	for {
		select {
		case <-connCtx.Done():
			return connCtx.Err()
		case <-s.stopCh:
			return nil
		case err := <-errCh:
			return err
		case <-healthTimerCh:
			markHealthy()
		case item := <-s.incomingCh:
			msg := types.IObserverMessage{
				Type:   "frames",
				Frames: []types.ITelemetryFrame{*item.frame},
			}
			data, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			if err := conn.Write(connCtx, websocket.MessageText, data); err != nil {
				return fmt.Errorf("writing frame: %w", err)
			}
		case <-heartbeatTicker.C:
			// Send heartbeat if idle
			s.mu.Lock()
			envInfo := s.cfg.Environment
			envInfo.LastHeartbeatAt = time.Now().UnixMilli()
			hbMsg := types.IObserverMessage{
				Type:        "hello",
				EnvID:       envInfo.ID,
				Environment: &envInfo,
				Since:       s.lastAckedSeq,
				Dropped:     0,
			}
			s.mu.Unlock()

			if data, err := json.Marshal(hbMsg); err == nil {
				_ = conn.Write(connCtx, websocket.MessageText, data)
			}
		}
	}
}
