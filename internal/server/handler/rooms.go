package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqlcgen "github.com/jesusthecreator017/TermText/internal/db/sqlc"
	"github.com/jesusthecreator017/TermText/internal/server/respond"
	"github.com/jesusthecreator017/TermText/internal/uid"
)

const generalRoomID = "00000000-0000-0000-0000-000000000001"

type roomResponse struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	DisplayName string `json:"display_name"`
	PeerID      string `json:"peer_id,omitempty"`
}

func peerString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return uid.String(u)
}

func (h *Handler) authedUser(r *http.Request) (string, bool) {
	return UserIDFromContext(r.Context())
}

// GetRooms lists the caller's rooms, auto-joining the shared "general" room.
func (h *Handler) GetRooms(w http.ResponseWriter, r *http.Request) {
	userIDStr, ok := h.authedUser(r)
	if !ok {
		respond.WithError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	userID, err := uid.Parse(userIDStr)
	if err != nil {
		respond.WithError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	general, _ := uid.Parse(generalRoomID)
	_ = h.DB.AddRoomMember(r.Context(), sqlcgen.AddRoomMemberParams{RoomID: general, UserID: userID})

	rows, err := h.DB.ListRoomsForUser(r.Context(), userID)
	if err != nil {
		h.Logger.Error("rooms: list", "err", err)
		respond.WithError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	out := make([]roomResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, roomResponse{
			ID:          uid.String(row.ID),
			Kind:        row.Kind,
			DisplayName: row.DisplayName,
			PeerID:      peerString(row.PeerID),
		})
	}
	respond.WithJson(w, http.StatusOK, respond.Envelope{"rooms": out})
}

func (h *Handler) PostRoom(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	if len(name) < 1 || len(name) > 64 {
		respond.WithError(w, http.StatusBadRequest, "room name must be 1-64 characters")
		return
	}

	room, err := h.DB.CreateRoom(r.Context(), sqlcgen.CreateRoomParams{Name: name, CreatedBy: userID})
	if err != nil {
		h.Logger.Error("rooms: create", "err", err)
		respond.WithError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if err := h.DB.AddRoomMember(r.Context(), sqlcgen.AddRoomMemberParams{RoomID: room.ID, UserID: userID}); err != nil {
		h.Logger.Error("rooms: add creator", "err", err)
		respond.WithError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	respond.WithJson(w, http.StatusCreated, respond.Envelope{
		"room": roomResponse{ID: uid.String(room.ID), Kind: room.Kind, DisplayName: room.Name},
	})
}

func (h *Handler) JoinRoom(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	roomID, err := uid.Parse(r.PathValue("id"))
	if err != nil {
		respond.WithError(w, http.StatusBadRequest, "invalid room id")
		return
	}
	if _, err := h.DB.GetRoomByID(r.Context(), roomID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respond.WithError(w, http.StatusNotFound, "room not found")
			return
		}
		h.Logger.Error("rooms: get", "err", err)
		respond.WithError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if err := h.DB.AddRoomMember(r.Context(), sqlcgen.AddRoomMemberParams{RoomID: roomID, UserID: userID}); err != nil {
		h.Logger.Error("rooms: join", "err", err)
		respond.WithError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	respond.WithJson(w, http.StatusOK, respond.Envelope{"status": "joined"})
}

// PostDM creates (or returns the existing) direct-message room with another user.
func (h *Handler) PostDM(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Username string `json:"username"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Username) == "" {
		respond.WithError(w, http.StatusBadRequest, "username is required")
		return
	}

	other, err := h.DB.GetUserByUsername(r.Context(), req.Username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respond.WithError(w, http.StatusNotFound, "user not found")
			return
		}
		h.Logger.Error("dm: get user", "err", err)
		respond.WithError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if uid.String(other.ID) == uid.String(userID) {
		respond.WithError(w, http.StatusBadRequest, "cannot DM yourself")
		return
	}

	key := dmKey(uid.String(userID), uid.String(other.ID))
	if existing, err := h.DB.GetDMByKey(r.Context(), &key); err == nil {
		respond.WithJson(w, http.StatusOK, respond.Envelope{
			"room": roomResponse{ID: uid.String(existing.ID), Kind: existing.Kind, DisplayName: other.Username, PeerID: uid.String(other.ID)},
		})
		return
	}

	room, err := h.DB.CreateDMRoom(r.Context(), sqlcgen.CreateDMRoomParams{DmKey: &key, CreatedBy: userID})
	if err != nil {
		h.Logger.Error("dm: create", "err", err)
		respond.WithError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if err := h.DB.AddRoomMember(r.Context(), sqlcgen.AddRoomMemberParams{RoomID: room.ID, UserID: userID}); err != nil {
		h.Logger.Error("dm: add self", "err", err)
		respond.WithError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if err := h.DB.AddRoomMember(r.Context(), sqlcgen.AddRoomMemberParams{RoomID: room.ID, UserID: other.ID}); err != nil {
		h.Logger.Error("dm: add other", "err", err)
		respond.WithError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	respond.WithJson(w, http.StatusCreated, respond.Envelope{
		"room": roomResponse{ID: uid.String(room.ID), Kind: room.Kind, DisplayName: other.Username, PeerID: uid.String(other.ID)},
	})
}

func (h *Handler) requireUser(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	idStr, ok := h.authedUser(r)
	if !ok {
		respond.WithError(w, http.StatusUnauthorized, "unauthorized")
		return pgtype.UUID{}, false
	}
	id, err := uid.Parse(idStr)
	if err != nil {
		respond.WithError(w, http.StatusUnauthorized, "unauthorized")
		return pgtype.UUID{}, false
	}
	return id, true
}

func dmKey(a, b string) string {
	if a < b {
		return a + ":" + b
	}
	return b + ":" + a
}
