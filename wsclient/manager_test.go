package wsclient

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tencent-connect/botgo/openapi"
)

// TestManagerSingleReconnect verifies that Manager will reconnect and keep only one active connection
func TestManagerSingleReconnect(t *testing.T) {
	var mu sync.Mutex
	conns := make(map[*websocket.Conn]struct{})
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	h := func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		mu.Lock()
		conns[c] = struct{}{}
		mu.Unlock()

		for {
			if _, _, err := c.ReadMessage(); err != nil {
				mu.Lock()
				delete(conns, c)
				mu.Unlock()
				_ = c.Close()
				return
			}
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(h))
	defer srv.Close()

	wsURL := "ws" + srv.URL[4:]
	var api openapi.OpenAPI = nil
	var apiV2 openapi.OpenAPI = nil

	m := NewManager(wsURL, 12345, api, apiV2)
	m.SetInterval(1 * time.Second)
	m.SetReconnectWaitMS(100)
	m.Start()
	defer m.Stop()

	waitUntil := func(d time.Duration, cond func() bool) bool {
		deadline := time.Now().Add(d)
		for time.Now().Before(deadline) {
			if cond() {
				return true
			}
			time.Sleep(50 * time.Millisecond)
		}
		return false
	}

	// wait for initial connect
	if !waitUntil(3*time.Second, func() bool { mu.Lock(); n := len(conns); mu.Unlock(); return n == 1 }) {
		t.Fatal("expected 1 connection after initial connect")
	}

	// close server-side connection
	mu.Lock()
	for c := range conns {
		_ = c.Close()
		break
	}
	mu.Unlock()

	// wait for reconnect and ensure only single connection exists
	if !waitUntil(10*time.Second, func() bool { mu.Lock(); n := len(conns); mu.Unlock(); return n == 1 }) {
		mu.Lock()
		n := len(conns)
		mu.Unlock()
		t.Fatalf("expected reconnection and single active conn, got %d", n)
	}
}

// TestManagerRepeatedCycles simulates repeated closures and ensures single connection
func TestManagerRepeatedCycles(t *testing.T) {
	var mu sync.Mutex
	conns := make(map[*websocket.Conn]struct{})
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	h := func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		mu.Lock()
		conns[c] = struct{}{}
		mu.Unlock()

		for {
			if _, _, err := c.ReadMessage(); err != nil {
				mu.Lock()
				delete(conns, c)
				mu.Unlock()
				_ = c.Close()
				return
			}
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(h))
	defer srv.Close()

	wsURL := "ws" + srv.URL[4:]
	var api openapi.OpenAPI = nil
	var apiV2 openapi.OpenAPI = nil

	m := NewManager(wsURL, 12345, api, apiV2)
	m.SetInterval(1 * time.Second)
	m.SetReconnectWaitMS(100)
	m.Start()
	defer m.Stop()

	waitUntil := func(d time.Duration, cond func() bool) bool {
		deadline := time.Now().Add(d)
		for time.Now().Before(deadline) {
			if cond() {
				return true
			}
			time.Sleep(50 * time.Millisecond)
		}
		return false
	}

	if !waitUntil(3*time.Second, func() bool { mu.Lock(); n := len(conns); mu.Unlock(); return n == 1 }) {
		t.Fatal("expected 1 connection after initial connect")
	}

	for i := 0; i < 5; i++ {
		// close the active connection
		mu.Lock()
		for c := range conns {
			_ = c.Close()
			break
		}
		mu.Unlock()

		if !waitUntil(5*time.Second, func() bool { mu.Lock(); n := len(conns); mu.Unlock(); return n == 1 }) {
			mu.Lock()
			n := len(conns)
			mu.Unlock()
			t.Fatalf("cycle %d: expected single active conn after reconnect, got %d", i, n)
		}
	}
}

// TestManagerRapidCloseSequence stresses manager with quick closes
func TestManagerRapidCloseSequence(t *testing.T) {
	var mu sync.Mutex
	conns := make(map[*websocket.Conn]struct{})
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	h := func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		mu.Lock()
		conns[c] = struct{}{}
		mu.Unlock()

		for {
			if _, _, err := c.ReadMessage(); err != nil {
				mu.Lock()
				delete(conns, c)
				mu.Unlock()
				_ = c.Close()
				return
			}
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(h))
	defer srv.Close()

	wsURL := "ws" + srv.URL[4:]
	var api openapi.OpenAPI = nil
	var apiV2 openapi.OpenAPI = nil

	m := NewManager(wsURL, 12345, api, apiV2)
	m.SetInterval(500 * time.Millisecond)
	m.SetReconnectWaitMS(50)
	m.Start()
	defer m.Stop()

	waitUntil := func(d time.Duration, cond func() bool) bool {
		deadline := time.Now().Add(d)
		for time.Now().Before(deadline) {
			if cond() {
				return true
			}
			time.Sleep(20 * time.Millisecond)
		}
		return false
	}

	if !waitUntil(2*time.Second, func() bool { mu.Lock(); n := len(conns); mu.Unlock(); return n == 1 }) {
		t.Fatal("expected 1 connection after initial connect")
	}

	// quickly close multiple times
	for i := 0; i < 5; i++ {
		mu.Lock()
		for c := range conns {
			_ = c.Close()
			break
		}
		mu.Unlock()
		time.Sleep(100 * time.Millisecond)
	}

	if !waitUntil(10*time.Second, func() bool { mu.Lock(); n := len(conns); mu.Unlock(); return n == 1 }) {
		mu.Lock()
		n := len(conns)
		mu.Unlock()
		t.Fatalf("expected a single active connection after rapid closes, got %d", n)
	}
}
