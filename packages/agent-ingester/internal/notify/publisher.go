package notify

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/types"
)

type client struct {
	conn net.Conn
	ch   chan []byte
	done chan struct{}
}

type Publisher struct {
	sockPath string
	listener net.Listener
	clients  map[*client]struct{}
	mu       sync.Mutex
	done     chan struct{}
}

func NewPublisher(sockPath string) (*Publisher, error) {
	if sockPath == "" {
		runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
		if runtimeDir == "" {
			runtimeDir = "/tmp"
		}
		sockPath = filepath.Join(runtimeDir, "agent-status", "notify.sock")
	}

	dir := filepath.Dir(sockPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("creating notify socket directory %s: %w", dir, err)
	}

	// Remove any leftover socket file
	_ = os.Remove(sockPath)

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("listening on unix socket %s: %w", sockPath, err)
	}
	_ = os.Chmod(sockPath, 0600)

	p := &Publisher{
		sockPath: sockPath,
		listener: listener,
		clients:  make(map[*client]struct{}),
		done:     make(chan struct{}),
	}

	go p.acceptLoop()

	return p, nil
}

func (p *Publisher) acceptLoop() {
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			select {
			case <-p.done:
				return
			default:
				continue
			}
		}

		c := &client{
			conn: conn,
			ch:   make(chan []byte, 100),
			done: make(chan struct{}),
		}

		p.mu.Lock()
		p.clients[c] = struct{}{}
		p.mu.Unlock()

		go p.writeLoop(c)
	}
}

func (p *Publisher) writeLoop(c *client) {
	defer func() {
		p.mu.Lock()
		delete(p.clients, c)
		p.mu.Unlock()
		_ = c.conn.Close()
		close(c.done)
	}()

	for {
		select {
		case <-p.done:
			return
		case msg, ok := <-c.ch:
			if !ok {
				return
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
			if _, err := c.conn.Write(append(msg, '\n')); err != nil {
				return
			}
		}
	}
}

func (p *Publisher) Publish(event types.IServerEvent) {
	msg, err := json.Marshal(event)
	if err != nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for c := range p.clients {
		select {
		case c.ch <- msg:
		case <-time.After(2 * time.Second):
			// Drop client if blocked for >2 seconds
			_ = c.conn.Close()
			delete(p.clients, c)
		}
	}
}

func (p *Publisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	close(p.done)
	for c := range p.clients {
		_ = c.conn.Close()
	}
	err := p.listener.Close()
	_ = os.Remove(p.sockPath)
	return err
}
