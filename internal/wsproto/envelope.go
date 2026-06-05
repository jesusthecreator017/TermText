// Package wsproto defines the WebSocket message envelope shared by the server
// and the TUI client.
package wsproto

import "encoding/json"

type Type string

const (
	TypeMessage  Type = "message"
	TypePresence Type = "presence"
	TypeTyping   Type = "typing"
	TypeRead     Type = "read"
	TypeError    Type = "error"
)

type Envelope struct {
	Type    Type            `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type MessagePayload struct {
	ID       string `json:"id"`
	RoomID   string `json:"room_id"`
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Body     string `json:"body"`
	SentAt   string `json:"sent_at"`
}

type PresencePayload struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Online   bool   `json:"online"`
}

type TypingPayload struct {
	RoomID   string `json:"room_id"`
	UserID   string `json:"user_id"`
	Username string `json:"username"`
}

type ReadPayload struct {
	RoomID    string `json:"room_id"`
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	MessageID string `json:"message_id"`
}

type ErrorPayload struct {
	Message string `json:"message"`
}

func New(t Type, payload any) (Envelope, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{Type: t, Payload: raw}, nil
}
