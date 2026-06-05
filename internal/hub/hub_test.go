package hub_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/jesusthecreator017/TermText/internal/hub"
	"github.com/jesusthecreator017/TermText/internal/wsproto"
)

// fakeRouter stamps the sender identity and delivers messages to a fixed
// recipient set. Presence/Snapshot are no-ops for these delivery tests.
type fakeRouter struct {
	recipients []string
}

func (f *fakeRouter) Inbound(_ context.Context, sender hub.Identity, env wsproto.Envelope) []hub.Delivery {
	if env.Type != wsproto.TypeMessage {
		return nil
	}
	var p wsproto.MessagePayload
	_ = json.Unmarshal(env.Payload, &p)
	out, _ := wsproto.New(wsproto.TypeMessage, wsproto.MessagePayload{
		RoomID:   p.RoomID,
		UserID:   sender.UserID,
		Username: sender.Username,
		Body:     p.Body,
		SentAt:   time.Now().UTC().Format(time.RFC3339),
	})
	raw, _ := json.Marshal(out)
	return []hub.Delivery{{Recipients: f.recipients, Payload: raw}}
}

func (f *fakeRouter) Presence(context.Context, hub.Identity, bool) []hub.Delivery { return nil }
func (f *fakeRouter) Snapshot(context.Context, hub.Identity, []string) []hub.Delivery {
	return nil
}

func newTestServer(t *testing.T, router hub.Router) *httptest.Server {
	t.Helper()
	h := hub.New(router, nil)
	ctx, cancel := context.WithCancel(context.Background())
	go h.Run(ctx)
	t.Cleanup(cancel)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		uid := r.URL.Query().Get("uid")
		h.Serve(r.Context(), conn, uid, uid)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func dial(t *testing.T, srv *httptest.Server, uid string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "?uid=" + uid
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("dial %s: %v", uid, err)
	}
	t.Cleanup(func() { conn.CloseNow() })
	return conn
}

func sendMessage(t *testing.T, conn *websocket.Conn, roomID, body string) {
	t.Helper()
	env, _ := wsproto.New(wsproto.TypeMessage, wsproto.MessagePayload{RoomID: roomID, Body: body})
	raw, _ := json.Marshal(env)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, raw); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func readMessage(t *testing.T, conn *websocket.Conn) wsproto.MessagePayload {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var env wsproto.Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.Type != wsproto.TypeMessage {
		t.Fatalf("type = %q, want message", env.Type)
	}
	var p wsproto.MessagePayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return p
}

func TestDeliveryReachesRecipients(t *testing.T) {
	srv := newTestServer(t, &fakeRouter{recipients: []string{"alice", "bob"}})
	alice := dial(t, srv, "alice")
	bob := dial(t, srv, "bob")
	time.Sleep(100 * time.Millisecond)

	sendMessage(t, alice, "room-1", "hello room")

	for name, conn := range map[string]*websocket.Conn{"alice": alice, "bob": bob} {
		got := readMessage(t, conn)
		if got.Body != "hello room" {
			t.Errorf("%s body = %q, want %q", name, got.Body, "hello room")
		}
		if got.RoomID != "room-1" {
			t.Errorf("%s room = %q, want room-1", name, got.RoomID)
		}
	}
}

func TestDeliverySkipsNonRecipients(t *testing.T) {
	// router only addresses alice; bob must not receive.
	srv := newTestServer(t, &fakeRouter{recipients: []string{"alice"}})
	alice := dial(t, srv, "alice")
	bob := dial(t, srv, "bob")
	time.Sleep(100 * time.Millisecond)

	sendMessage(t, alice, "room-1", "private-ish")

	if got := readMessage(t, alice); got.Body != "private-ish" {
		t.Fatalf("alice body = %q", got.Body)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, _, err := bob.Read(ctx); err == nil {
		t.Fatal("bob should not have received a message")
	}
}
