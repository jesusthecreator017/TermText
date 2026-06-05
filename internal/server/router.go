package server

import (
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/time/rate"

	"github.com/jesusthecreator017/TermText/internal/auth"
	"github.com/jesusthecreator017/TermText/internal/server/handler"
)

// Mount wires routes and returns the root handler with global middleware
// (request logging + panic recovery) applied.
func Mount(mux *http.ServeMux, h *handler.Handler, tm *auth.TokenMaker, logger *slog.Logger) http.Handler {
	requireAuth := AuthMiddleware(tm)

	// public + observability
	mux.Handle("GET /health", Metrics("/health")(http.HandlerFunc(h.GetHealthCheck)))
	mux.Handle("GET /metrics", promhttp.Handler())

	// auth endpoints: rate-limited per IP (token bucket: ~1/s, burst 5)
	authLimit := RateLimit(rate.Limit(1), 5)
	mux.Handle("POST /signup", authLimit(Metrics("/signup")(http.HandlerFunc(h.PostSignup))))
	mux.Handle("POST /login", authLimit(Metrics("/login")(http.HandlerFunc(h.PostLogin))))

	authed := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, requireAuth(Metrics(pattern)(fn)))
	}
	authed("GET /me", h.GetMe)
	authed("GET /rooms", h.GetRooms)
	authed("POST /rooms", h.PostRoom)
	authed("POST /rooms/{id}/join", h.JoinRoom)
	authed("GET /rooms/{id}/messages", h.GetRoomMessages)
	authed("POST /dms", h.PostDM)
	authed("GET /search", h.SearchMessages)
	authed("PUT /me/public_key", h.PutMyPublicKey)
	authed("GET /users/{id}/public_key", h.GetUserPublicKey)

	mux.HandleFunc("GET /ws", h.ServeWS)

	return RequestLogger(logger)(mux)
}
