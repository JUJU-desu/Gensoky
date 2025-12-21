package wsclient

import (
	"fmt"
	"sync"
	"time"

	"github.com/hoshinonyaruko/gensokyo/mylog"
	"github.com/tencent-connect/botgo/openapi"
)

// Manager is responsible for maintaining a single websocket connection to oneonebotv11 server.
// It centralizes connect/read/heartbeat/reconnect logic so that only one connection exists at a time
// and reconnects are serialized and observable.
type Manager struct {
	urlStr string
	botID  uint64
	api    openapi.OpenAPI
	apiv2  openapi.OpenAPI

	mu              sync.Mutex
	currentClient   *WebSocketClient
	stopCh          chan struct{}
	doneCh          chan struct{}
	reconnectWaitMS int // milliseconds to wait for old close to finish before dialing again
	interval        time.Duration
}

// NewManager creates a new Manager.
func NewManager(urlStr string, botID uint64, api openapi.OpenAPI, apiv2 openapi.OpenAPI) *Manager {
	return &Manager{
		urlStr:          urlStr,
		botID:           botID,
		api:             api,
		apiv2:           apiv2,
		stopCh:          make(chan struct{}),
		doneCh:          make(chan struct{}),
		reconnectWaitMS: 300, // default 300ms waiting for previous close
		interval:        5 * time.Second,
	}
}

// Start begins the manager's run loop in a goroutine.
func (m *Manager) Start() {
	go m.run()
}

// Stop signals the manager to stop and waits for cleanup.
func (m *Manager) Stop() {
	close(m.stopCh)
	<-m.doneCh
}

// SetReconnectWaitMS allows tuning wait time to detect old connection close
func (m *Manager) SetReconnectWaitMS(ms int) {
	m.reconnectWaitMS = ms
}

func (m *Manager) run() {
	defer close(m.doneCh)
	for {
		select {
		case <-m.stopCh:
			m.closeCurrent()
			return
		default:
			// try to ensure we have a client
			if m.currentClient == nil {
				m.connectLoop()
			} else {
				// wait for client to close
				m.waitCurrentClose()
			}
		}
	}
}

func (m *Manager) connectLoop() {
	for {
		select {
		case <-m.stopCh:
			return
		default:
			// attempt to dial
			mylog.Printf("Manager: attempting connect to %s", m.urlStr)
			client, err := NewWebSocketClient(m.urlStr, m.botID, m.api, m.apiv2, 1)
			if err == nil && client != nil {
				m.mu.Lock()
				m.currentClient = client
				m.mu.Unlock()
				mylog.Println("Manager: connected and managing new client")
				return
			}
			mylog.Printf("Manager: connect failed: %v, retrying in %s", err, m.interval)
			select {
			case <-time.After(m.interval):
			case <-m.stopCh:
				return
			}
		}
	}
}

// waitCurrentClose waits for current client's closeDone or stop signal
func (m *Manager) waitCurrentClose() {
	m.mu.Lock()
	client := m.currentClient
	m.mu.Unlock()

	if client == nil {
		return
	}

	select {
	case <-client.closeDone:
		mylog.Println("Manager: detected client closeDone")
		m.mu.Lock()
		m.currentClient = nil
		m.mu.Unlock()
	case <-m.stopCh:
		m.closeCurrent()
	}
}

// closeCurrent attempts to close current client safely
func (m *Manager) closeCurrent() {
	m.mu.Lock()
	client := m.currentClient
	m.currentClient = nil
	m.mu.Unlock()

	if client != nil {
		mylog.Println("Manager: closing current client")
		client.cancel()
		_ = client.conn.Close()
		// wait for reader to exit with timeout based on reconnectWaitMS
		select {
		case <-client.closeDone:
			mylog.Println("Manager: old client closed")
		case <-time.After(time.Duration(m.reconnectWaitMS) * time.Millisecond):
			mylog.Println("Manager: timeout waiting for old client to close")
		}
	}
}

// SetInterval sets the reconnect interval
func (m *Manager) SetInterval(d time.Duration) {
	m.interval = d
}

// GetActiveClient returns the currently managed client, if any
func (m *Manager) GetActiveClient() *WebSocketClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.currentClient
}

// For debug: String representation
func (m *Manager) String() string {
	return fmt.Sprintf("Manager(url=%s, botID=%d)", m.urlStr, m.botID)
}
