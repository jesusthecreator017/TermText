package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqlcgen "github.com/jesusthecreator017/TermText/internal/db/sqlc"
	"github.com/jesusthecreator017/TermText/internal/server/respond"
	"github.com/jesusthecreator017/TermText/internal/uid"
)

const (
	defaultMessageLimit = 50
	maxMessageLimit     = 100
)

type messageResponse struct {
	ID       string `json:"id"`
	RoomID   string `json:"room_id"`
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Body     string `json:"body"`
	SentAt   string `json:"sent_at"`
}

func msgResp(id, roomID, userID pgtype.UUID, username, body string, createdAt pgtype.Timestamptz) messageResponse {
	return messageResponse{
		ID:       uid.String(id),
		RoomID:   uid.String(roomID),
		UserID:   uid.String(userID),
		Username: username,
		Body:     body,
		SentAt:   createdAt.Time.UTC().Format(time.RFC3339),
	}
}

// GetRoomMessages returns a page of a room's messages, oldest→newest, using
// keyset pagination via the ?before=<message_id> cursor.
func (h *Handler) GetRoomMessages(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	roomID, err := uid.Parse(r.PathValue("id"))
	if err != nil {
		respond.WithError(w, http.StatusBadRequest, "invalid room id")
		return
	}

	member, err := h.DB.IsRoomMember(r.Context(), sqlcgen.IsRoomMemberParams{RoomID: roomID, UserID: userID})
	if err != nil {
		h.Logger.Error("messages: membership", "err", err)
		respond.WithError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !member {
		respond.WithError(w, http.StatusForbidden, "not a member of this room")
		return
	}

	limit := parseLimit(r.URL.Query().Get("limit"))
	before := r.URL.Query().Get("before")

	var out []messageResponse
	if before == "" {
		rows, err := h.DB.ListRecentMessages(r.Context(), sqlcgen.ListRecentMessagesParams{RoomID: roomID, Limit: limit})
		if err != nil {
			h.Logger.Error("messages: recent", "err", err)
			respond.WithError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		out = make([]messageResponse, len(rows))
		for i, m := range rows {
			out[i] = msgResp(m.ID, m.RoomID, m.UserID, m.Username, m.Body, m.CreatedAt)
		}
	} else {
		cursorID, err := uid.Parse(before)
		if err != nil {
			respond.WithError(w, http.StatusBadRequest, "invalid cursor")
			return
		}
		cursor, err := h.DB.GetMessageByID(r.Context(), cursorID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				respond.WithError(w, http.StatusBadRequest, "invalid cursor")
				return
			}
			h.Logger.Error("messages: cursor lookup", "err", err)
			respond.WithError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		rows, err := h.DB.ListMessagesBefore(r.Context(), sqlcgen.ListMessagesBeforeParams{
			RoomID:    roomID,
			CreatedAt: cursor.CreatedAt,
			ID:        cursorID,
			Limit:     limit,
		})
		if err != nil {
			h.Logger.Error("messages: before", "err", err)
			respond.WithError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		out = make([]messageResponse, len(rows))
		for i, m := range rows {
			out[i] = msgResp(m.ID, m.RoomID, m.UserID, m.Username, m.Body, m.CreatedAt)
		}
	}

	reverse(out) // queries return newest-first; clients want oldest→newest
	respond.WithJson(w, http.StatusOK, respond.Envelope{"messages": out})
}

// SearchMessages runs a full-text search scoped to the caller's rooms.
func (h *Handler) SearchMessages(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	q := r.URL.Query().Get("q")
	if len(q) == 0 {
		respond.WithError(w, http.StatusBadRequest, "missing query")
		return
	}

	rows, err := h.DB.SearchMessages(r.Context(), sqlcgen.SearchMessagesParams{UserID: userID, PlaintoTsquery: q})
	if err != nil {
		h.Logger.Error("search", "err", err)
		respond.WithError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	out := make([]messageResponse, len(rows))
	for i, m := range rows {
		out[i] = msgResp(m.ID, m.RoomID, m.UserID, m.Username, m.Body, m.CreatedAt)
	}
	respond.WithJson(w, http.StatusOK, respond.Envelope{"results": out})
}

func parseLimit(s string) int32 {
	if s == "" {
		return defaultMessageLimit
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultMessageLimit
	}
	if n > maxMessageLimit {
		return maxMessageLimit
	}
	return int32(n)
}

func reverse(s []messageResponse) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}
