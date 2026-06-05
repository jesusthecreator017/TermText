package handler

import (
	"errors"
	"net/http"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jesusthecreator017/TermText/internal/metrics"
	"github.com/jesusthecreator017/TermText/internal/server/respond"
)

func (h *Handler) ServeWS(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		respond.WithError(w, http.StatusUnauthorized, "missing token")
		return
	}
	claims, err := h.Tokens.Verify(token)
	if err != nil {
		respond.WithError(w, http.StatusUnauthorized, "invalid or expired token")
		return
	}

	var uuid pgtype.UUID
	if err := uuid.Scan(claims.UserID); err != nil {
		respond.WithError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	user, err := h.DB.GetUserByID(r.Context(), uuid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respond.WithError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		h.Logger.Error("ws: get user", "err", err)
		respond.WithError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		h.Logger.Warn("ws: accept failed", "err", err)
		return
	}

	metrics.WSConnections.Inc()
	defer metrics.WSConnections.Dec()

	h.Hub.Serve(r.Context(), conn, claims.UserID, user.Username)
}
