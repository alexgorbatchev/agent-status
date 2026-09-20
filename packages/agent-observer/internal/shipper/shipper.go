package shipper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/cenkalti/backoff/v4"
	"github.com/coder/websocket"
	"github.com/gammazero/deque"
)

const MaxRingBufferSize = 1000

var (
	heartbeatInterval = 10 * time.Second
	initialBackoff    = 500 * time.Millisecond
)

type Config struct {
	IngesterURL string
	BearerToken string
	Environment types.IEnvironmentInfo
}

type Shipper struct {
	cfg Config

	mu           sync.Mutex
	ringBuffer   deque.Deque[*types.ITelemetryFrame]
	lastSeq      int64
	lastAckedSeq int64
	droppedCount int64

	incomingCh chan *types.ITelemetryFrame
	stopCh     chan struct{}
	once       sync.Once
}

func NewShipper(cfg Config) *Shipper {
	return &Shipper{
		cfg:        cfg,
		incomingCh: make(chan *types.ITelemetryFrame, 500),
		stopCh:     make(chan struct{}),
	}
}

func (s *Shipper) Enqueue(frame types.ITelemetryFrame) {
	s.mu.Lock()
	s.lastSeq++
	frame.Seq = s.lastSeq
	frameCopy := frame

	if s.ringBuffer.Len() >= MaxRingBufferSize {
		s.ringBuffer.PopFront()
		s.droppedCount++
	}
	s.ringBuffer.PushBack(&frameCopy)
	s.mu.Unlock()

	select {
	case s.incomingCh <- &frameCopy:
	case <-s.stopCh:
	default:
	}
}

func (s *Shipper) UnackedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ringBuffer.Len()
}

func (s *Shipper) DroppedCount() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.droppedCount
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
	expBackoff.RandomizationFactor = 0.5

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		default:
		}

		logging.Info("ingester_connecting", "ingesterUrl", s.cfg.IngesterURL)
		err := s.connectAndStream(ctx, expBackoff.Reset)
		if err != nil && ctx.Err() == nil {
			select {
			case <-s.stopCh:
			default:
				logging.Warn("ingester_disconnected", "ingesterUrl", s.cfg.IngesterURL, "err", err)
			}
		}

		nextInterval := expBackoff.NextBackOff()
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

	// 1. Send Hello Frame
	s.mu.Lock()
	envCopy := s.cfg.Environment
	helloMsg := types.IObserverMessage{
		Type:        "hello",
		EnvID:       envCopy.ID,
		Environment: &envCopy,
		Since:       s.lastAckedSeq,
		Dropped:     s.droppedCount,
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
	if onConnected != nil {
		onConnected()
	}

	// 2. Replay unacknowledged frames
	s.mu.Lock()
	var unackedFrames []types.ITelemetryFrame
	for i := 0; i < s.ringBuffer.Len(); i++ {
		f := s.ringBuffer.At(i)
		if f != nil {
			unackedFrames = append(unackedFrames, *f)
		}
	}
	s.mu.Unlock()

	if len(unackedFrames) > 0 {
		replayMsg := types.IObserverMessage{
			Type:   "frames",
			Frames: unackedFrames,
		}
		if data, err := json.Marshal(replayMsg); err == nil {
			_ = conn.Write(connCtx, websocket.MessageText, data)
		}
	}

	// 3. Start reader goroutine for acks
	errCh := make(chan error, 2)
	go func() {
		for {
			_, data, err := conn.Read(connCtx)
			if err != nil {
				errCh <- err
				return
			}

			var ack types.IIngesterMessage
			if err := json.Unmarshal(data, &ack); err == nil && ack.Type == "ack" && ack.UpTo > 0 {
				s.mu.Lock()
				s.lastAckedSeq = max(s.lastAckedSeq, ack.UpTo)
				for s.ringBuffer.Len() > 0 {
					front := s.ringBuffer.Front()
					if front != nil && front.Seq <= ack.UpTo {
						s.ringBuffer.PopFront()
					} else {
						break
					}
				}
				s.mu.Unlock()
			}
		}
	}()

	heartbeatTicker := time.NewTicker(heartbeatInterval)
	defer heartbeatTicker.Stop()

	// 4. Stream incoming frames and heartbeats
	for {
		select {
		case <-connCtx.Done():
			return connCtx.Err()
		case <-s.stopCh:
			return nil
		case err := <-errCh:
			return err
		case frame := <-s.incomingCh:
			msg := types.IObserverMessage{
				Type:   "frames",
				Frames: []types.ITelemetryFrame{*frame},
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
				Dropped:     s.droppedCount,
			}
			s.mu.Unlock()

			if data, err := json.Marshal(hbMsg); err == nil {
				_ = conn.Write(connCtx, websocket.MessageText, data)
			}
		}
	}
}
