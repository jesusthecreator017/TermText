package hub

import (
	"context"
	"encoding/json"
	"time"

	"github.com/coder/websocket"

	"github.com/jesusthecreator017/TermText/internal/wsproto"
)

const (
	sendBuffer   = 16
	pingInterval = 30 * time.Second
	writeTimeout = 10 * time.Second
)

type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	userID   string
	username string
	cancel   context.CancelFunc
}

func newClient(h *Hub, conn *websocket.Conn, userID, username string) *Client {
	return &Client{
		hub:      h,
		conn:     conn,
		send:     make(chan []byte, sendBuffer),
		userID:   userID,
		username: username,
	}
}

func (c *Client) run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	defer cancel()

	go c.writeLoop(ctx)
	c.readLoop(ctx)
	c.conn.CloseNow()
}

func (c *Client) readLoop(ctx context.Context) {
	id := Identity{UserID: c.userID, Username: c.username}
	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			return
		}

		var env wsproto.Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			continue
		}
		switch env.Type {
		case wsproto.TypeMessage, wsproto.TypeTyping, wsproto.TypeRead:
			c.hub.dispatch(c.hub.router.Inbound(ctx, id, env))
		default:
			// ignore unknown / server-only types
		}
	}
}

func (c *Client) writeLoop(ctx context.Context) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case data := <-c.send:
			if err := c.write(ctx, data); err != nil {
				c.cancel()
				return
			}
		case <-ticker.C:
			pctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := c.conn.Ping(pctx)
			cancel()
			if err != nil {
				c.cancel()
				return
			}
		}
	}
}

func (c *Client) write(ctx context.Context, data []byte) error {
	wctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return c.conn.Write(wctx, websocket.MessageText, data)
}
