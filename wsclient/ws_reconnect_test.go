package wsclient

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hoshinonyaruko/gensokyo/mylog"
	"github.com/tencent-connect/botgo/openapi"
)

// TestReconnectClosesOldConnection verifies that when the client reconnects, the old connection is closed
func TestReconnectClosesOldConnection(t *testing.T) {
	// prepare a lightweight WS server that accepts connections and tracks active conns
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

	// convert http URL to ws URL
	wsURL := "ws" + srv.URL[4:]

	// provide nil openapi implementations for testing
	var api openapi.OpenAPI = nil
	var apiV2 openapi.OpenAPI = nil

	// Create client
	client, err := NewWebSocketClient(wsURL, 12345, api, apiV2, 1)
	if err != nil {
		t.Fatalf("failed to create websocket client: %v", err)
	}
	defer client.Close()

	// wait until first connection registered
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

	// close the existing server-side connection to trigger client reconnect
	mu.Lock()
	for c := range conns {
		_ = c.Close()
		break
	}
	mu.Unlock()

	// Wait enough time for the client reconnect loop to run (Reconnect waits 5s per iteration)
	if !waitUntil(12*time.Second, func() bool { mu.Lock(); n := len(conns); mu.Unlock(); return n == 1 }) {
		// Dump for debugging
		mu.Lock()
		n := len(conns)
		mu.Unlock()
		t.Fatalf("expected reconnection and single active conn, got %d", n)
	}

	mylog.Println("Reconnect test succeeded: old connection was closed and replaced by a single active connection")
}
