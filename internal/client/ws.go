package client

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/coder/websocket"

	"github.com/jesusthecreator017/TermText/internal/wsproto"
)

type Conn struct {
	ws *websocket.Conn
}

func (c *Client) ConnectWS(ctx context.Context, token string) (*Conn, error) {
	wsURL := toWebSocketURL(c.baseURL) + "/ws?token=" + url.QueryEscape(token)
	ws, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return nil, err
	}
	return &Conn{ws: ws}, nil
}

func (c *Conn) Send(ctx context.Context, roomID, body string) error {
	return c.write(ctx, wsproto.TypeMessage, wsproto.MessagePayload{RoomID: roomID, Body: body})
}

func (c *Conn) SendTyping(ctx context.Context, roomID string) error {
	return c.write(ctx, wsproto.TypeTyping, wsproto.TypingPayload{RoomID: roomID})
}

func (c *Conn) SendRead(ctx context.Context, roomID, messageID string) error {
	return c.write(ctx, wsproto.TypeRead, wsproto.ReadPayload{RoomID: roomID, MessageID: messageID})
}

func (c *Conn) write(ctx context.Context, t wsproto.Type, payload any) error {
	env, err := wsproto.New(t, payload)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return c.ws.Write(ctx, websocket.MessageText, raw)
}

func (c *Conn) Read(ctx context.Context) (wsproto.Envelope, error) {
	_, data, err := c.ws.Read(ctx)
	if err != nil {
		return wsproto.Envelope{}, err
	}
	var env wsproto.Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return wsproto.Envelope{}, err
	}
	return env, nil
}

func (c *Conn) Close() error {
	return c.ws.Close(websocket.StatusNormalClosure, "bye")
}

func toWebSocketURL(httpURL string) string {
	switch {
	case strings.HasPrefix(httpURL, "https://"):
		return "wss://" + strings.TrimPrefix(httpURL, "https://")
	case strings.HasPrefix(httpURL, "http://"):
		return "ws://" + strings.TrimPrefix(httpURL, "http://")
	default:
		return httpURL
	}
}
