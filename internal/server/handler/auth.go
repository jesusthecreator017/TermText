package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/jesusthecreator017/TermText/internal/auth"
	sqlcgen "github.com/jesusthecreator017/TermText/internal/db/sqlc"
	"github.com/jesusthecreator017/TermText/internal/server/respond"
	"github.com/jesusthecreator017/TermText/internal/uid"
)

func (h *Handler) PostSignup(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if msg := validateCredentials(req); msg != "" {
		respond.WithError(w, http.StatusBadRequest, msg)
		return
	}

	hash, err := auth.HashPassword(req.Password, h.Argon)
	if err != nil {
		h.Logger.Error("signup: hash password", "err", err)
		respond.WithError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	user, err := h.DB.CreateUser(r.Context(), sqlcgen.CreateUserParams{
		Username:     req.Username,
		PasswordHash: hash,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			respond.WithError(w, http.StatusConflict, "username already taken")
			return
		}
		h.Logger.Error("signup: create user", "err", err)
		respond.WithError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	h.respondWithToken(w, http.StatusCreated, user)
}

func (h *Handler) PostLogin(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Username == "" || req.Password == "" {
		respond.WithError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	user, err := h.DB.GetUserByUsername(r.Context(), req.Username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respond.WithError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		h.Logger.Error("login: get user", "err", err)
		respond.WithError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	ok, err := auth.VerifyPassword(req.Password, user.PasswordHash)
	if err != nil {
		h.Logger.Error("login: verify password", "err", err)
		respond.WithError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !ok {
		respond.WithError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	h.respondWithToken(w, http.StatusOK, user)
}

func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
	id, ok := UserIDFromContext(r.Context())
	if !ok {
		respond.WithError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	userID, err := uid.Parse(id)
	if err != nil {
		respond.WithError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	user, err := h.DB.GetUserByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respond.WithError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		h.Logger.Error("me: get user", "err", err)
		respond.WithError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	respond.WithJson(w, http.StatusOK, respond.Envelope{"user": toUserResponse(user)})
}

func (h *Handler) respondWithToken(w http.ResponseWriter, status int, user sqlcgen.User) {
	token, err := h.Tokens.Issue(uid.String(user.ID))
	if err != nil {
		h.Logger.Error("issue token", "err", err)
		respond.WithError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	respond.WithJson(w, status, respond.Envelope{
		"user":  toUserResponse(user),
		"token": token,
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		respond.WithError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}
