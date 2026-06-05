package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqlcgen "github.com/jesusthecreator017/TermText/internal/db/sqlc"
	"github.com/jesusthecreator017/TermText/internal/hub"
	"github.com/jesusthecreator017/TermText/internal/metrics"
	"github.com/jesusthecreator017/TermText/internal/uid"
	"github.com/jesusthecreator017/TermText/internal/wsproto"
)

type messageRouter struct {
	db     sqlcgen.Querier
	logger *slog.Logger
}

func newMessageRouter(db sqlcgen.Querier, logger *slog.Logger) *messageRouter {
	return &messageRouter{db: db, logger: logger}
}

func (mr *messageRouter) Inbound(ctx context.Context, sender hub.Identity, env wsproto.Envelope) []hub.Delivery {
	switch env.Type {
	case wsproto.TypeMessage:
		return mr.handleMessage(ctx, sender, env.Payload)
	case wsproto.TypeTyping:
		return mr.handleTyping(ctx, sender, env.Payload)
	case wsproto.TypeRead:
		return mr.handleRead(ctx, sender, env.Payload)
	default:
		return nil
	}
}

func (mr *messageRouter) handleMessage(ctx context.Context, sender hub.Identity, raw json.RawMessage) []hub.Delivery {
	var p wsproto.MessagePayload
	if err := json.Unmarshal(raw, &p); err != nil || p.Body == "" || p.RoomID == "" {
		return nil
	}
	room, ok := mr.requireMember(ctx, sender, p.RoomID)
	if !ok {
		return mr.errTo(sender.UserID, "not a member of this room")
	}
	senderID, _ := uid.Parse(sender.UserID)

	msg, err := mr.db.CreateMessage(ctx, sqlcgen.CreateMessageParams{RoomID: room, UserID: senderID, Body: p.Body})
	if err != nil {
		mr.logger.Error("router: save message", "err", err)
		return mr.errTo(sender.UserID, "internal error")
	}
	metrics.MessagesTotal.Inc()
	payload := marshalEnv(wsproto.TypeMessage, wsproto.MessagePayload{
		ID:       uid.String(msg.ID),
		RoomID:   p.RoomID,
		UserID:   sender.UserID,
		Username: sender.Username,
		Body:     p.Body,
		SentAt:   msg.CreatedAt.Time.UTC().Format(time.RFC3339),
	})
	return []hub.Delivery{{Recipients: mr.memberIDs(ctx, room), Payload: payload}}
}

func (mr *messageRouter) handleTyping(ctx context.Context, sender hub.Identity, raw json.RawMessage) []hub.Delivery {
	var p wsproto.TypingPayload
	if err := json.Unmarshal(raw, &p); err != nil || p.RoomID == "" {
		return nil
	}
	room, ok := mr.requireMember(ctx, sender, p.RoomID)
	if !ok {
		return nil
	}
	payload := marshalEnv(wsproto.TypeTyping, wsproto.TypingPayload{
		RoomID:   p.RoomID,
		UserID:   sender.UserID,
		Username: sender.Username,
	})
	// ephemeral: fan out to everyone except the typist
	return []hub.Delivery{{Recipients: mr.memberIDsExcept(ctx, room, sender.UserID), Payload: payload}}
}

func (mr *messageRouter) handleRead(ctx context.Context, sender hub.Identity, raw json.RawMessage) []hub.Delivery {
	var p wsproto.ReadPayload
	if err := json.Unmarshal(raw, &p); err != nil || p.RoomID == "" || p.MessageID == "" {
		return nil
	}
	room, ok := mr.requireMember(ctx, sender, p.RoomID)
	if !ok {
		return nil
	}
	senderID, _ := uid.Parse(sender.UserID)
	msgID, err := uid.Parse(p.MessageID)
	if err != nil {
		return nil
	}
	if err := mr.db.UpdateLastRead(ctx, sqlcgen.UpdateLastReadParams{
		RoomID:            room,
		UserID:            senderID,
		LastReadMessageID: msgID,
	}); err != nil {
		mr.logger.Error("router: update last read", "err", err)
		return nil
	}
	payload := marshalEnv(wsproto.TypeRead, wsproto.ReadPayload{
		RoomID:    p.RoomID,
		UserID:    sender.UserID,
		Username:  sender.Username,
		MessageID: p.MessageID,
	})
	return []hub.Delivery{{Recipients: mr.memberIDs(ctx, room), Payload: payload}}
}

func (mr *messageRouter) Presence(ctx context.Context, user hub.Identity, online bool) []hub.Delivery {
	uID, err := uid.Parse(user.UserID)
	if err != nil {
		return nil
	}
	peers, err := mr.db.ListRoomPeers(ctx, uID)
	if err != nil {
		mr.logger.Error("router: list peers", "err", err)
		return nil
	}
	recipients := make([]string, 0, len(peers))
	for _, p := range peers {
		recipients = append(recipients, uid.String(p.ID))
	}
	payload := marshalEnv(wsproto.TypePresence, wsproto.PresencePayload{
		UserID:   user.UserID,
		Username: user.Username,
		Online:   online,
	})
	return []hub.Delivery{{Recipients: recipients, Payload: payload}}
}

func (mr *messageRouter) Snapshot(ctx context.Context, user hub.Identity, onlineUserIDs []string) []hub.Delivery {
	uID, err := uid.Parse(user.UserID)
	if err != nil {
		return nil
	}
	peers, err := mr.db.ListRoomPeers(ctx, uID)
	if err != nil {
		mr.logger.Error("router: snapshot peers", "err", err)
		return nil
	}
	onlineSet := make(map[string]bool, len(onlineUserIDs))
	for _, id := range onlineUserIDs {
		onlineSet[id] = true
	}

	var ds []hub.Delivery
	for _, p := range peers {
		pid := uid.String(p.ID)
		if !onlineSet[pid] {
			continue
		}
		payload := marshalEnv(wsproto.TypePresence, wsproto.PresencePayload{
			UserID:   pid,
			Username: p.Username,
			Online:   true,
		})
		ds = append(ds, hub.Delivery{Recipients: []string{user.UserID}, Payload: payload})
	}
	return ds
}

func (mr *messageRouter) requireMember(ctx context.Context, sender hub.Identity, roomID string) (pgtype.UUID, bool) {
	room, err := uid.Parse(roomID)
	if err != nil {
		return pgtype.UUID{}, false
	}
	senderID, err := uid.Parse(sender.UserID)
	if err != nil {
		return pgtype.UUID{}, false
	}
	member, err := mr.db.IsRoomMember(ctx, sqlcgen.IsRoomMemberParams{RoomID: room, UserID: senderID})
	if err != nil {
		mr.logger.Error("router: membership", "err", err)
		return pgtype.UUID{}, false
	}
	return room, member
}

func (mr *messageRouter) memberIDs(ctx context.Context, room pgtype.UUID) []string {
	ids, err := mr.db.ListRoomMemberIDs(ctx, room)
	if err != nil {
		mr.logger.Error("router: list members", "err", err)
		return nil
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, uid.String(id))
	}
	return out
}

func (mr *messageRouter) memberIDsExcept(ctx context.Context, room pgtype.UUID, exclude string) []string {
	all := mr.memberIDs(ctx, room)
	out := make([]string, 0, len(all))
	for _, id := range all {
		if id != exclude {
			out = append(out, id)
		}
	}
	return out
}

func (mr *messageRouter) errTo(userID, msg string) []hub.Delivery {
	return []hub.Delivery{{Recipients: []string{userID}, Payload: marshalEnv(wsproto.TypeError, wsproto.ErrorPayload{Message: msg})}}
}

func marshalEnv(t wsproto.Type, payload any) []byte {
	env, err := wsproto.New(t, payload)
	if err != nil {
		return nil
	}
	raw, err := json.Marshal(env)
	if err != nil {
		return nil
	}
	return raw
}
