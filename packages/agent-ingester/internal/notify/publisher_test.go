package notify

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

// shortTempDir creates a short temp dir under /tmp to prevent exceeding sockaddr_un sun_path limit on macOS (104 chars)
func shortTempDir(t *testing.T) string {
	t.Helper()
	var b [4]byte
	_, _ = rand.Read(b[:])
	dir := filepath.Join("/tmp", "as-"+hex.EncodeToString(b[:]))
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestNewPublisher_DefaultPath(t *testing.T) {
	tempDir := shortTempDir(t)
	t.Setenv("XDG_RUNTIME_DIR", tempDir)

	pub, err := NewPublisher("")
	if err != nil {
		t.Fatalf("NewPublisher(\"\") failed: %v", err)
	}
	defer func() {
		_ = pub.Close()
	}()

	expectedSock := filepath.Join(tempDir, "agent-status", "notify.sock")
	if pub.sockPath != expectedSock {
		t.Errorf("got sockPath %q, want %q", pub.sockPath, expectedSock)
	}

	info, err := os.Stat(expectedSock)
	if err != nil {
		t.Fatalf("stat socket file failed: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("socket permissions = %o, want 0600", perm)
	}
}

func TestNewPublisher_FallbackTmp(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	tempDir := shortTempDir(t)
	sockPath := filepath.Join(tempDir, "test-status", "notify.sock")

	pub, err := NewPublisher(sockPath)
	if err != nil {
		t.Fatalf("NewPublisher(custom) failed: %v", err)
	}
	defer func() {
		_ = pub.Close()
	}()

	if pub.sockPath != sockPath {
		t.Errorf("got sockPath %q, want %q", pub.sockPath, sockPath)
	}
}

func TestPublisher_BroadcastToClients(t *testing.T) {
	tempDir := shortTempDir(t)
	sockPath := filepath.Join(tempDir, "notify.sock")

	pub, err := NewPublisher(sockPath)
	if err != nil {
		t.Fatalf("NewPublisher() failed: %v", err)
	}
	defer func() {
		_ = pub.Close()
	}()

	conn1, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("net.Dial 1 failed: %v", err)
	}
	defer func() {
		_ = conn1.Close()
	}()

	conn2, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("net.Dial 2 failed: %v", err)
	}
	defer func() {
		_ = conn2.Close()
	}()

	for i := 0; i < 50; i++ {
		pub.mu.Lock()
		count := len(pub.clients)
		pub.mu.Unlock()
		if count == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	r1 := bufio.NewReader(conn1)
	r2 := bufio.NewReader(conn2)

	// Verify both clients receive initial hello frame upon registration
	_ = conn1.SetReadDeadline(time.Now().Add(2 * time.Second))
	helloLine1, err := r1.ReadBytes('\n')
	if err != nil {
		t.Fatalf("client 1 failed to read hello: %v", err)
	}
	var hello1Evt types.IServerEvent
	if err := json.Unmarshal(helloLine1, &hello1Evt); err != nil {
		t.Fatalf("client 1 hello unmarshal error: %v", err)
	}
	if hello1Evt.Type != "hello" {
		t.Errorf("client 1 expected hello event, got %q", hello1Evt.Type)
	}

	_ = conn2.SetReadDeadline(time.Now().Add(2 * time.Second))
	helloLine2, err := r2.ReadBytes('\n')
	if err != nil {
		t.Fatalf("client 2 failed to read hello: %v", err)
	}
	var hello2Evt types.IServerEvent
	if err := json.Unmarshal(helloLine2, &hello2Evt); err != nil {
		t.Fatalf("client 2 hello unmarshal error: %v", err)
	}
	if hello2Evt.Type != "hello" {
		t.Errorf("client 2 expected hello event, got %q", hello2Evt.Type)
	}

	event := types.IServerEvent{
		Type: "test_event",
		At:   1234567890,
	}
	pub.Publish(event)

	_ = conn1.SetReadDeadline(time.Now().Add(2 * time.Second))
	line1, err := r1.ReadBytes('\n')
	if err != nil {
		t.Fatalf("client 1 failed to read: %v", err)
	}
	var got1 types.IServerEvent
	if err := json.Unmarshal(line1, &got1); err != nil {
		t.Fatalf("client 1 json unmarshal error: %v", err)
	}
	if got1.Type != "test_event" || got1.At != 1234567890 {
		t.Errorf("client 1 got %+v, want %+v", got1, event)
	}

	_ = conn2.SetReadDeadline(time.Now().Add(2 * time.Second))
	line2, err := r2.ReadBytes('\n')
	if err != nil {
		t.Fatalf("client 2 failed to read: %v", err)
	}
	var got2 types.IServerEvent
	if err := json.Unmarshal(line2, &got2); err != nil {
		t.Fatalf("client 2 json unmarshal error: %v", err)
	}
	if got2.Type != "test_event" || got2.At != 1234567890 {
		t.Errorf("client 2 got %+v, want %+v", got2, event)
	}
}

func TestPublisher_Broadcast(t *testing.T) {
	tempDir := shortTempDir(t)
	sockPath := filepath.Join(tempDir, "notify.sock")

	pub, err := NewPublisher(sockPath)
	if err != nil {
		t.Fatalf("NewPublisher() failed: %v", err)
	}
	defer func() {
		_ = pub.Close()
	}()

	// Nil publisher broadcast should not panic
	var nilPub *Publisher
	nilPub.Broadcast(nil)
	pub.Broadcast(nil)

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("net.Dial failed: %v", err)
	}
	defer func() { _ = conn.Close() }()

	for i := 0; i < 50; i++ {
		pub.mu.Lock()
		count := len(pub.clients)
		pub.mu.Unlock()
		if count == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	r := bufio.NewReader(conn)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = r.ReadBytes('\n') // read hello
	if err != nil {
		t.Fatalf("reading hello failed: %v", err)
	}

	event := types.IServerEvent{
		Type: "broadcast_event",
		At:   987654321,
	}
	pub.Broadcast(&event)

	line, err := r.ReadBytes('\n')
	if err != nil {
		t.Fatalf("reading from client failed: %v", err)
	}

	var got types.IServerEvent
	if err := json.Unmarshal(line, &got); err != nil {
		t.Fatalf("unmarshaling event failed: %v", err)
	}
	if got.Type != "broadcast_event" || got.At != 987654321 {
		t.Errorf("got %+v, want %+v", got, event)
	}
}

func TestPublisher_ClientSaturationAndDrop(t *testing.T) {
	tempDir := shortTempDir(t)
	sockPath := filepath.Join(tempDir, "notify.sock")

	pub, err := NewPublisher(sockPath)
	if err != nil {
		t.Fatalf("NewPublisher() failed: %v", err)
	}
	defer func() {
		_ = pub.Close()
	}()

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("net.Dial failed: %v", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	for i := 0; i < 50; i++ {
		pub.mu.Lock()
		count := len(pub.clients)
		pub.mu.Unlock()
		if count == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Saturate socket buffer by sending large messages without the client reading
	largePayload := make([]byte, 64*1024)
	for i := range largePayload {
		largePayload[i] = 'A'
	}

	// Keep publishing until Publish blocks and drops the client (saturates socket buffer + channel)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 2000; i++ {
			pub.Publish(types.IServerEvent{
				Type: string(largePayload),
			})
			pub.mu.Lock()
			count := len(pub.clients)
			pub.mu.Unlock()
			if count == 0 {
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for client drop")
	}

	pub.mu.Lock()
	clientCount := len(pub.clients)
	pub.mu.Unlock()

	if clientCount != 0 {
		t.Errorf("expected 0 clients after saturation, got %d", clientCount)
	}
}

func TestPublisher_CloseCleanup(t *testing.T) {
	tempDir := shortTempDir(t)
	sockPath := filepath.Join(tempDir, "notify.sock")

	pub, err := NewPublisher(sockPath)
	if err != nil {
		t.Fatalf("NewPublisher() failed: %v", err)
	}

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("net.Dial failed: %v", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	for i := 0; i < 50; i++ {
		pub.mu.Lock()
		count := len(pub.clients)
		pub.mu.Unlock()
		if count == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := pub.Close(); err != nil {
		t.Errorf("pub.Close() error = %v", err)
	}

	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Errorf("expected socket file %q to be removed, err = %v", sockPath, err)
	}
}

func TestNewPublisher_InvalidDirectory(t *testing.T) {
	tempFile, err := os.CreateTemp("", "not-a-dir")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	defer func() {
		_ = os.Remove(tempFile.Name())
	}()
	_ = tempFile.Close()

	invalidSock := filepath.Join(tempFile.Name(), "subdir", "notify.sock")
	_, err = NewPublisher(invalidSock)
	if err == nil {
		t.Errorf("expected error for invalid directory path, got nil")
	}
}
