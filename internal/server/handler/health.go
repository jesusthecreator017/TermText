package handler

import (
	"net/http"

	"github.com/jesusthecreator017/TermText/internal/server/respond"
)

func (h *Handler) GetHealthCheck(w http.ResponseWriter, r *http.Request) {
	respond.WithJson(w, http.StatusOK, respond.Envelope{"status": "ok"})
}
