// Package conn wraps the WebSocket client that talks to the SPORE server.
// It owns the read/write loops, reconnection, and keepalive ping.
package conn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/yumlevi/acorn-cli/go/internal/proto"
)

type Client struct {
	serverURL string
	teamKey   string
	user      string

	mu     sync.Mutex
	ws     *websocket.Conn
	closed bool

	// Inbound messages routed into this channel. The Bubble Tea model
	// consumes it via a tea.Cmd that blocks on <-In.
	In chan proto.In
}

func New(serverURL, teamKey, user string) *Client {
	return &Client{
		serverURL: serverURL,
		teamKey:   teamKey,
		user:      user,
		In:        make(chan proto.In, 256),
	}
}

// Dial opens the WebSocket and authenticates via the acorn team key. Returns
// the ready-to-use client, or an error if the handshake failed.
//
// The SPORE server expects the acorn key in the Sec-WebSocket-Protocol or
// a custom header depending on the deployment; keeping this simple with a
// custom header for now.
func (c *Client) Dial(ctx context.Context) error {
	u, err := url.Parse(c.serverURL)
	if err != nil {
		return fmt.Errorf("bad server url: %w", err)
	}
	header := http.Header{}
	header.Set("X-Acorn-Team-Key", c.teamKey)
	header.Set("X-Acorn-User", c.user)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	ws, resp, err := dialer.DialContext(ctx, u.String(), header)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("ws dial %s failed: %w (status %d)", u.String(), err, resp.StatusCode)
		}
		return fmt.Errorf("ws dial %s failed: %w", u.String(), err)
	}

	c.mu.Lock()
	c.ws = ws
	c.closed = false
	c.mu.Unlock()

	go c.readLoop()
	go c.pingLoop()
	return nil
}

func (c *Client) readLoop() {
	defer func() {
		c.mu.Lock()
		c.closed = true
		if c.ws != nil {
			_ = c.ws.Close()
		}
		c.mu.Unlock()
		close(c.In)
	}()
	for {
		c.mu.Lock()
		ws := c.ws
		closed := c.closed
		c.mu.Unlock()
		if ws == nil || closed {
			return
		}
		_, data, err := ws.ReadMessage()
		if err != nil {
			if !errors.Is(err, websocket.ErrCloseSent) {
				// Push an error frame so the UI can surface the disconnect.
				select {
				case c.In <- proto.In{Type: "conn:error", Error: err.Error()}:
				default:
				}
			}
			return
		}
		var m proto.In
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		m.Raw = json.RawMessage(data)
		select {
		case c.In <- m:
		default:
			// Drop frames if the UI is backed up. Chat deltas are the only
			// high-frequency path and dropping a few deltas is fine.
		}
	}
}

func (c *Client) pingLoop() {
	t := time.NewTicker(25 * time.Second)
	defer t.Stop()
	for range t.C {
		c.mu.Lock()
		ws := c.ws
		closed := c.closed
		c.mu.Unlock()
		if ws == nil || closed {
			return
		}
		_ = c.sendRaw(proto.Out{Type: "ping"})
	}
}

// Send marshals and writes a message.
func (c *Client) Send(m proto.Out) error {
	return c.sendRaw(m)
}

func (c *Client) sendRaw(m proto.Out) error {
	c.mu.Lock()
	ws := c.ws
	closed := c.closed
	c.mu.Unlock()
	if ws == nil || closed {
		return errors.New("websocket closed")
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return ws.WriteMessage(websocket.TextMessage, data)
}

// Close shuts down the connection.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if c.ws != nil {
		_ = c.ws.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye"))
		_ = c.ws.Close()
		c.ws = nil
	}
}
