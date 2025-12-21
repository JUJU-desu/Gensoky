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

// TestRepeatedReconnectCycles ensures that repeated server-side connection closures
// will not leave multiple active connections on the server side (client closes old conns)
func TestRepeatedReconnectCycles(t *testing.T) {
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

		// read until error, then remove
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

	client, err := NewWebSocketClient(wsURL, 12345, api, apiV2, 1)
	if err != nil {
		t.Fatalf("failed to create websocket client: %v", err)
	}
	defer client.Close()

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

	if !waitUntil(2*time.Second, func() bool { mu.Lock(); n := len(conns); mu.Unlock(); return n == 1 }) {
		t.Fatal("expected 1 connection after initial connect")
	}

	// repeat close/connect cycle several times
	for i := 0; i < 3; i++ {
		// close the active connection
		mu.Lock()
		for c := range conns {
			_ = c.Close()
			break
		}
		mu.Unlock()

		// wait for reconnect to re-establish single connection
		if !waitUntil(15*time.Second, func() bool { mu.Lock(); n := len(conns); mu.Unlock(); return n == 1 }) {
			mu.Lock()
			n := len(conns)
			mu.Unlock()
			t.Fatalf("cycle %d: expected reconnection and single active conn, got %d", i, n)
		}
	}
}

// TestRapidCloseSequence ensures that quickly closing multiple times does not create concurrent active conns
func TestRapidCloseSequence(t *testing.T) {
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

		// read until error, then remove
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

	client, err := NewWebSocketClient(wsURL, 12345, api, apiV2, 1)
	if err != nil {
		t.Fatalf("failed to create websocket client: %v", err)
	}
	defer client.Close()

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

	if !waitUntil(2*time.Second, func() bool { mu.Lock(); n := len(conns); mu.Unlock(); return n == 1 }) {
		t.Fatal("expected 1 connection after initial connect")
	}

	// quickly close the connection multiple times
	for i := 0; i < 3; i++ {
		mu.Lock()
		for c := range conns {
			_ = c.Close()
			break
		}
		mu.Unlock()
		// small pause before next close
		time.Sleep(200 * time.Millisecond)
	}

	// ensure client recovers with single connection
	if !waitUntil(20*time.Second, func() bool { mu.Lock(); n := len(conns); mu.Unlock(); return n == 1 }) {
		mu.Lock()
		n := len(conns)
		mu.Unlock()
		t.Fatalf("expected single active connection after rapid closes, got %d", n)
	}
}
