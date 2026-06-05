package tui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jesusthecreator017/TermText/internal/client"
	"github.com/jesusthecreator017/TermText/internal/crypto"
	"github.com/jesusthecreator017/TermText/internal/wsproto"
)

func TestLoginSubmitRequiresBothFields(t *testing.T) {
	m := newLoginModel(client.New("http://x"), "")
	m.inputs[0].SetValue("alice")
	m.inputs[1].SetValue("")
	m, cmd := m.submit()
	if cmd != nil {
		t.Fatal("expected no command when password is empty")
	}
	if m.errMsg == "" {
		t.Fatal("expected validation error message")
	}
}

func TestChatAppendMessageStoresInRoomBuffer(t *testing.T) {
	m := newChatModel(nil, "", nil, "alice", nil)
	payload, _ := json.Marshal(wsproto.MessagePayload{
		RoomID:   "room-1",
		Username: "bob",
		Body:     "hello there",
		SentAt:   "2026-05-29T21:00:00Z",
	})
	m.appendEnvelope(wsproto.Envelope{Type: wsproto.TypeMessage, Payload: payload})

	rb := m.buffers["room-1"]
	if rb == nil || len(rb.msgs) != 1 {
		t.Fatalf("expected 1 message in room-1 buffer")
	}
	if rb.msgs[0].username != "bob" || rb.msgs[0].body != "hello there" {
		t.Fatalf("message fields wrong: %+v", rb.msgs[0])
	}
	if !m.unread["room-1"] {
		t.Fatal("expected room-1 to be marked unread when not active")
	}
}

func TestChatDedupesByMessageID(t *testing.T) {
	m := newChatModel(nil, "", nil, "alice", nil)
	payload, _ := json.Marshal(wsproto.MessagePayload{
		ID: "msg-1", RoomID: "room-1", Username: "bob", Body: "dup", SentAt: "2026-05-29T21:00:00Z",
	})
	env := wsproto.Envelope{Type: wsproto.TypeMessage, Payload: payload}
	m.appendEnvelope(env)
	m.appendEnvelope(env)
	if rb := m.buffers["room-1"]; rb == nil || len(rb.msgs) != 1 {
		t.Fatal("expected duplicate message id to be ignored")
	}
}

func TestChatIgnoresMalformedPayload(t *testing.T) {
	m := newChatModel(nil, "", nil, "alice", nil)
	m.appendEnvelope(wsproto.Envelope{Type: wsproto.TypeMessage, Payload: json.RawMessage(`not json`)})
	if len(m.buffers) != 0 {
		t.Fatalf("expected malformed payload to be ignored, got %d buffers", len(m.buffers))
	}
}

func TestChatPresenceTracking(t *testing.T) {
	m := newChatModel(nil, "", nil, "alice", nil)
	on, _ := json.Marshal(wsproto.PresencePayload{Username: "bob", Online: true})
	m.appendEnvelope(wsproto.Envelope{Type: wsproto.TypePresence, Payload: on})
	if !m.online["bob"] {
		t.Fatal("expected bob online")
	}
	off, _ := json.Marshal(wsproto.PresencePayload{Username: "bob", Online: false})
	m.appendEnvelope(wsproto.Envelope{Type: wsproto.TypePresence, Payload: off})
	if m.online["bob"] {
		t.Fatal("expected bob offline")
	}
}

func TestChatTypingExpires(t *testing.T) {
	m := newChatModel(nil, "", nil, "alice", nil)
	m.active = "room-1"
	typ, _ := json.Marshal(wsproto.TypingPayload{RoomID: "room-1", Username: "bob"})
	m.appendEnvelope(wsproto.Envelope{Type: wsproto.TypeTyping, Payload: typ})
	if got := m.activeTypers(); len(got) != 1 || got[0] != "bob" {
		t.Fatalf("expected bob typing, got %v", got)
	}
	m.typers["room-1"]["bob"] = time.Now().Add(-time.Second)
	m.pruneTypers()
	if len(m.activeTypers()) != 0 {
		t.Fatal("expected typing to expire")
	}
}

func TestChatSeenByCountsOthersOnLatest(t *testing.T) {
	m := newChatModel(nil, "", nil, "alice", nil)
	m.active = "room-1"
	rb := m.ensureBuffer("room-1")
	rb.insert(bufMsg{id: "m1", body: "x"})
	read, _ := json.Marshal(wsproto.ReadPayload{RoomID: "room-1", Username: "bob", MessageID: "m1"})
	m.appendEnvelope(wsproto.Envelope{Type: wsproto.TypeRead, Payload: read})
	if m.seenBy() != 1 {
		t.Fatalf("expected seen-by 1, got %d", m.seenBy())
	}
	mine, _ := json.Marshal(wsproto.ReadPayload{RoomID: "room-1", Username: "alice", MessageID: "m1"})
	m.appendEnvelope(wsproto.Envelope{Type: wsproto.TypeRead, Payload: mine})
	if m.seenBy() != 1 {
		t.Fatalf("self read should not count, got %d", m.seenBy())
	}
}

func TestChatDecryptsDMBody(t *testing.T) {
	myPub, myPriv, _ := crypto.GenerateKeypair()
	peerPub, peerPriv, _ := crypto.GenerateKeypair()

	m := newChatModel(nil, "", nil, "alice", myPriv)
	m.rooms = []client.Room{{ID: "dm1", Kind: "dm", DisplayName: "bob", PeerID: "bob-id"}}
	m.pubkeys["bob-id"] = peerPub

	sealed, _ := crypto.Seal("secret plan", myPub, peerPriv) // bob -> alice
	if got := m.decryptBody("dm1", sealed); got != "secret plan" {
		t.Fatalf("decryptBody = %q, want %q", got, "secret plan")
	}

	mine, _ := crypto.Seal("my reply", peerPub, myPriv) // alice's own message
	if got := m.decryptBody("dm1", mine); got != "my reply" {
		t.Fatalf("self decrypt = %q, want %q", got, "my reply")
	}
}

func TestChatLockedDMShowsPlaceholder(t *testing.T) {
	_, peerPriv, _ := crypto.GenerateKeypair()
	otherPub, _, _ := crypto.GenerateKeypair()
	m := newChatModel(nil, "", nil, "alice", nil) // locked: no private key
	m.rooms = []client.Room{{ID: "dm1", Kind: "dm", PeerID: "bob-id"}}
	sealed, _ := crypto.Seal("hi", otherPub, peerPriv)
	if got := m.decryptBody("dm1", sealed); !strings.Contains(got, "🔒") {
		t.Fatalf("expected lock placeholder, got %q", got)
	}
}

func TestChatNonDMBodyPassthrough(t *testing.T) {
	m := newChatModel(nil, "", nil, "alice", nil)
	m.rooms = []client.Room{{ID: "room1", Kind: "room"}}
	if got := m.decryptBody("room1", "plain text"); got != "plain text" {
		t.Fatalf("non-dm body should pass through, got %q", got)
	}
}

func TestChatCommandRouting(t *testing.T) {
	m := newChatModel(client.New("http://x"), "tok", nil, "alice", nil)
	m.textarea.SetValue("/bogus")
	if cmd := m.handleInput(); cmd != nil {
		t.Fatal("unknown command should not produce a command")
	}
	if !strings.Contains(m.notice, "unknown command") {
		t.Fatalf("expected unknown-command notice, got %q", m.notice)
	}
}
