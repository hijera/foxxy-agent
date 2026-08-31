package mcp

import (
	"sync"
	"testing"
)

// TestClientCloseConcurrent guards the sync.Once in Close: teardown paths
// (CloseAll, reload parking, probe) are unsynchronised, and the old
// select-default close could panic when two of them raced.
func TestClientCloseConcurrent(t *testing.T) {
	c := NewStaticClient("race", nil)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := c.Close(); err != nil {
				t.Errorf("Close: %v", err)
			}
		}()
	}
	wg.Wait()
	select {
	case <-c.done:
	default:
		t.Fatal("done channel not closed after Close")
	}
}

// TestStdioTransportCloseConcurrent exercises the transport-level close that
// Client.Close forwards to; the process handle is nil-safe on purpose so the
// test needs no subprocess.
func TestStdioTransportCloseConcurrent(t *testing.T) {
	tr := &stdioTransport{
		msgs:   make(chan []byte, 1),
		done:   make(chan struct{}),
		cancel: func() {},
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := tr.Close(); err != nil {
				t.Errorf("Close: %v", err)
			}
		}()
	}
	wg.Wait()
	select {
	case <-tr.done:
	default:
		t.Fatal("done channel not closed after Close")
	}
}

// TestHTTPTransportsCloseConcurrent covers the two remote transports' Close.
func TestHTTPTransportsCloseConcurrent(t *testing.T) {
	streamable := &streamableHTTPTransport{msgs: make(chan []byte, 1), closed: make(chan struct{})}
	sse := &sseTransport{msgs: make(chan []byte, 1), closed: make(chan struct{}), cancel: func() {}}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _ = streamable.Close() }()
		go func() { defer wg.Done(); _ = sse.Close() }()
	}
	wg.Wait()

	select {
	case <-streamable.closed:
	default:
		t.Fatal("streamable http closed channel not closed after Close")
	}
	select {
	case <-sse.closed:
	default:
		t.Fatal("sse closed channel not closed after Close")
	}
}
