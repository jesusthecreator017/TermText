package hub

import (
	"context"
	"log/slog"
	"time"

	"github.com/coder/websocket"

	"github.com/jesusthecreator017/TermText/internal/wsproto"
)

// Identity is the authenticated owner of a connection.
type Identity struct {
	UserID   string
	Username string
}

// Delivery is a marshaled envelope addressed to a set of user IDs.
type Delivery struct {
	Recipients []string
	Payload    []byte
}

// Router validates and addresses events. It is implemented by the server layer
// (which owns the database); the hub stays a pure delivery registry and owns
// only connection liveness.
type Router interface {
	Inbound(ctx context.Context, sender Identity, env wsproto.Envelope) []Delivery
	Presence(ctx context.Context, user Identity, online bool) []Delivery
	Snapshot(ctx context.Context, user Identity, onlineUserIDs []string) []Delivery
}

type registerReq struct {
	c     *Client
	reply chan registerReply
}
type registerReply struct {
	wasFirst bool
	online   []string
}
type unregisterReq struct {
	c     *Client
	reply chan bool // wasLast
}

type Hub struct {
	register   chan registerReq
	unregister chan unregisterReq
	deliveries chan Delivery
	clients    map[string]map[*Client]struct{} // userID -> live connections
	router     Router
	done       chan struct{}
	logger     *slog.Logger
}

func New(router Router, logger *slog.Logger) *Hub {
	if logger == nil {
		logger = slog.Default()
	}
	return &Hub{
		register:   make(chan registerReq),
		unregister: make(chan unregisterReq),
		deliveries: make(chan Delivery, 64),
		clients:    make(map[string]map[*Client]struct{}),
		router:     router,
		done:       make(chan struct{}),
		logger:     logger,
	}
}

func (h *Hub) Run(ctx context.Context) {
	defer close(h.done)
	for {
		select {
		case <-ctx.Done():
			for _, conns := range h.clients {
				for c := range conns {
					c.cancel()
				}
			}
			return
		case rr := <-h.register:
			conns := h.clients[rr.c.userID]
			wasFirst := len(conns) == 0
			if conns == nil {
				conns = make(map[*Client]struct{})
				h.clients[rr.c.userID] = conns
			}
			conns[rr.c] = struct{}{}
			online := make([]string, 0, len(h.clients))
			for uid := range h.clients {
				online = append(online, uid)
			}
			rr.reply <- registerReply{wasFirst: wasFirst, online: online}
			h.logger.Info("client connected", "user", rr.c.username)
		case ur := <-h.unregister:
			wasLast := false
			if conns, ok := h.clients[ur.c.userID]; ok {
				delete(conns, ur.c)
				if len(conns) == 0 {
					delete(h.clients, ur.c.userID)
					wasLast = true
				}
			}
			ur.reply <- wasLast
			h.logger.Info("client disconnected", "user", ur.c.username)
		case d := <-h.deliveries:
			for _, uid := range d.Recipients {
				for c := range h.clients[uid] {
					select {
					case c.send <- d.Payload:
					default:
						h.logger.Warn("dropping slow client", "user", c.username)
						c.cancel()
					}
				}
			}
		}
	}
}

func (h *Hub) deliver(d Delivery) {
	if len(d.Recipients) == 0 || len(d.Payload) == 0 {
		return
	}
	select {
	case h.deliveries <- d:
	case <-h.done:
	}
}

func (h *Hub) dispatch(ds []Delivery) {
	for _, d := range ds {
		h.deliver(d)
	}
}

// Serve registers a websocket connection and blocks for its lifetime, emitting
// presence on the user's first/last connection and a presence snapshot to the
// newly connected client. Router DB calls run here (not on the hub goroutine).
func (h *Hub) Serve(ctx context.Context, conn *websocket.Conn, userID, username string) {
	c := newClient(h, conn, userID, username)
	id := Identity{UserID: userID, Username: username}

	reply := make(chan registerReply, 1)
	select {
	case h.register <- registerReq{c: c, reply: reply}:
	case <-h.done:
		conn.Close(websocket.StatusGoingAway, "server shutting down")
		return
	}
	rr := <-reply

	if rr.wasFirst {
		h.emitPresence(id, true)
	}
	h.emitSnapshot(id, rr.online)

	c.run(ctx)

	ureply := make(chan bool, 1)
	select {
	case h.unregister <- unregisterReq{c: c, reply: ureply}:
	case <-h.done:
		return
	}
	if <-ureply {
		h.emitPresence(id, false)
	}
}

func (h *Hub) emitPresence(id Identity, online bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h.dispatch(h.router.Presence(ctx, id, online))
}

func (h *Hub) emitSnapshot(id Identity, online []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h.dispatch(h.router.Snapshot(ctx, id, online))
}
