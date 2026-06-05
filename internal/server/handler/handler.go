package handler

import (
	"log/slog"

	"github.com/jesusthecreator017/TermText/internal/auth"
	sqlcgen "github.com/jesusthecreator017/TermText/internal/db/sqlc"
	"github.com/jesusthecreator017/TermText/internal/hub"
)

type Handler struct {
	DB     sqlcgen.Querier
	Logger *slog.Logger
	Tokens *auth.TokenMaker
	Argon  auth.Params
	Hub    *hub.Hub
}

func New(db sqlcgen.Querier, logger *slog.Logger, tokens *auth.TokenMaker, argon auth.Params, h *hub.Hub) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{DB: db, Logger: logger, Tokens: tokens, Argon: argon, Hub: h}
}
