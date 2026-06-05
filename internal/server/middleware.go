package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/jesusthecreator017/TermText/internal/auth"
	"github.com/jesusthecreator017/TermText/internal/metrics"
	"github.com/jesusthecreator017/TermText/internal/server/handler"
	"github.com/jesusthecreator017/TermText/internal/server/respond"
)

type ctxKey string

const requestIDKey ctxKey = "requestID"

func AuthMiddleware(tm *auth.TokenMaker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || token == "" {
				respond.WithError(w, http.StatusUnauthorized, "missing or malformed authorization header")
				return
			}
			claims, err := tm.Verify(token)
			if err != nil {
				respond.WithError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}
			ctx := handler.WithUserID(r.Context(), claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// statusRecorder captures the response status code for access logging. Unwrap
// lets http.ResponseController reach the underlying writer so capabilities like
// http.Hijacker (needed by the WebSocket upgrade) keep working.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func newRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}

// RequestLogger assigns a request ID, recovers panics, and emits one structured
// access log line per request. WebSocket upgrades (/ws) skip the status wrapper
// since the connection is hijacked.
func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			id := newRequestID()
			ctx := context.WithValue(r.Context(), requestIDKey, id)
			w.Header().Set("X-Request-ID", id)

			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered",
						"err", rec, "request_id", id, "method", r.Method, "path", r.URL.Path)
					respond.WithError(w, http.StatusInternalServerError, "internal server error")
				}
			}()

			next.ServeHTTP(rec, r.WithContext(ctx))

			logger.Info("request",
				"request_id", id,
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"duration_ms", time.Since(start).Milliseconds(),
				"remote", clientIP(r),
			)
		})
	}
}

// clientIP extracts the best-effort client IP, honoring X-Forwarded-For (set by
// Fly's proxy) before falling back to RemoteAddr.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first, _, _ := strings.Cut(xff, ",")
		return strings.TrimSpace(first)
	}
	host := r.RemoteAddr
	if i := strings.LastIndexByte(host, ':'); i >= 0 {
		return host[:i]
	}
	return host
}

// Metrics records request count and latency. The label uses the matched route
// pattern (not the raw path) to keep cardinality bounded.
func Metrics(pattern string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			metrics.RequestsTotal.WithLabelValues(r.Method, pattern, strconv.Itoa(rec.status)).Inc()
			metrics.RequestDuration.WithLabelValues(r.Method, pattern).Observe(time.Since(start).Seconds())
		})
	}
}

// ipRateLimiter hands out a token-bucket limiter per client IP.
type ipRateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*rate.Limiter
	rate     rate.Limit
	burst    int
}

func newIPRateLimiter(r rate.Limit, burst int) *ipRateLimiter {
	return &ipRateLimiter{buckets: make(map[string]*rate.Limiter), rate: r, burst: burst}
}

func (l *ipRateLimiter) limiter(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	lim, ok := l.buckets[ip]
	if !ok {
		lim = rate.NewLimiter(l.rate, l.burst)
		l.buckets[ip] = lim
	}
	return lim
}

// RateLimit throttles requests per client IP, returning 429 when exceeded.
func RateLimit(r rate.Limit, burst int) func(http.Handler) http.Handler {
	limiter := newIPRateLimiter(r, burst)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if !limiter.limiter(clientIP(req)).Allow() {
				respond.WithError(w, http.StatusTooManyRequests, "rate limit exceeded, slow down")
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}
