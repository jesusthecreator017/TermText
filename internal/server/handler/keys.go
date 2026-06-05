package handler

import (
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	sqlcgen "github.com/jesusthecreator017/TermText/internal/db/sqlc"
	"github.com/jesusthecreator017/TermText/internal/server/respond"
	"github.com/jesusthecreator017/TermText/internal/uid"
)

const publicKeyLen = 32

// PutMyPublicKey stores the caller's Curve25519 public key.
func (h *Handler) PutMyPublicKey(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req struct {
		PublicKey string `json:"public_key"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	key, err := base64.StdEncoding.DecodeString(req.PublicKey)
	if err != nil || len(key) != publicKeyLen {
		respond.WithError(w, http.StatusBadRequest, "public_key must be 32 base64-encoded bytes")
		return
	}
	if err := h.DB.SetUserPublicKey(r.Context(), sqlcgen.SetUserPublicKeyParams{ID: userID, PublicKey: key}); err != nil {
		h.Logger.Error("keys: set", "err", err)
		respond.WithError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	respond.WithJson(w, http.StatusOK, respond.Envelope{"status": "ok"})
}

// GetUserPublicKey returns another user's public key for DM encryption (TOFU).
func (h *Handler) GetUserPublicKey(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}
	targetID, err := uid.Parse(r.PathValue("id"))
	if err != nil {
		respond.WithError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	user, err := h.DB.GetUserByID(r.Context(), targetID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respond.WithError(w, http.StatusNotFound, "user not found")
			return
		}
		h.Logger.Error("keys: get user", "err", err)
		respond.WithError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if len(user.PublicKey) == 0 {
		respond.WithError(w, http.StatusNotFound, "user has no public key")
		return
	}
	respond.WithJson(w, http.StatusOK, respond.Envelope{
		"user_id":    uid.String(user.ID),
		"username":   user.Username,
		"public_key": base64.StdEncoding.EncodeToString(user.PublicKey),
	})
}
