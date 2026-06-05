package handler_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jesusthecreator017/TermText/internal/auth"
	sqlcgen "github.com/jesusthecreator017/TermText/internal/db/sqlc"
	"github.com/jesusthecreator017/TermText/internal/hub"
	"github.com/jesusthecreator017/TermText/internal/server"
	"github.com/jesusthecreator017/TermText/internal/server/handler"
)

const testKey = "7f1c6cddfa31603d6b6809e286fc2de30ff50bcd3152cb2fc1d4220317ad56e5"

type fakeDB struct {
	sqlcgen.Querier // unimplemented methods panic if called
	byID            map[[16]byte]sqlcgen.User
	byName          map[string]sqlcgen.User
}

func newFakeDB() *fakeDB {
	return &fakeDB{
		byID:   map[[16]byte]sqlcgen.User{},
		byName: map[string]sqlcgen.User{},
	}
}

func (f *fakeDB) CreateUser(_ context.Context, arg sqlcgen.CreateUserParams) (sqlcgen.User, error) {
	key := strings.ToLower(arg.Username)
	if _, exists := f.byName[key]; exists {
		return sqlcgen.User{}, &pgconn.PgError{Code: "23505"}
	}
	var raw [16]byte
	rand.Read(raw[:])
	u := sqlcgen.User{
		ID:           pgtype.UUID{Bytes: raw, Valid: true},
		Username:     arg.Username,
		PasswordHash: arg.PasswordHash,
		CreatedAt:    pgtype.Timestamptz{Time: time.Now(), Valid: true},
		UpdatedAt:    pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	f.byID[raw] = u
	f.byName[key] = u
	return u, nil
}

func (f *fakeDB) GetUserByID(_ context.Context, id pgtype.UUID) (sqlcgen.User, error) {
	if u, ok := f.byID[id.Bytes]; ok {
		return u, nil
	}
	return sqlcgen.User{}, pgx.ErrNoRows
}

func (f *fakeDB) GetUserByUsername(_ context.Context, username string) (sqlcgen.User, error) {
	if u, ok := f.byName[strings.ToLower(username)]; ok {
		return u, nil
	}
	return sqlcgen.User{}, pgx.ErrNoRows
}

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	tm, err := auth.NewTokenMaker(testKey, time.Hour)
	if err != nil {
		t.Fatalf("NewTokenMaker: %v", err)
	}
	h := handler.New(newFakeDB(), nil, tm, auth.DefaultParams(), hub.New(nil, nil))
	mux := http.NewServeMux()
	return server.Mount(mux, h, tm, slog.Default())
}

func doJSON(t *testing.T, srv http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func TestSignupHappyPath(t *testing.T) {
	srv := newTestServer(t)
	rec := doJSON(t, srv, "POST", "/signup", `{"username":"alice","password":"hunter2pass"}`, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "password_hash") {
		t.Fatal("response leaked password_hash")
	}
	var got map[string]any
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got["token"] == nil || got["token"] == "" {
		t.Fatal("expected a token in response")
	}
}

func TestSignupDuplicate(t *testing.T) {
	srv := newTestServer(t)
	body := `{"username":"alice","password":"hunter2pass"}`
	doJSON(t, srv, "POST", "/signup", body, "")
	rec := doJSON(t, srv, "POST", "/signup", body, "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestSignupInvalid(t *testing.T) {
	srv := newTestServer(t)
	cases := []string{
		`{"username":"ab","password":"hunter2pass"}`,
		`{"username":"alice","password":"short"}`,
		`{"username":"alice","password":"hunter2pass","extra":1}`,
		`not json`,
	}
	for _, body := range cases {
		rec := doJSON(t, srv, "POST", "/signup", body, "")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %q: status = %d, want 400", body, rec.Code)
		}
	}
}

func TestLoginAndMe(t *testing.T) {
	srv := newTestServer(t)
	doJSON(t, srv, "POST", "/signup", `{"username":"alice","password":"hunter2pass"}`, "")

	rec := doJSON(t, srv, "POST", "/login", `{"username":"alice","password":"hunter2pass"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var login map[string]any
	json.Unmarshal(rec.Body.Bytes(), &login)
	token, _ := login["token"].(string)
	if token == "" {
		t.Fatal("no token from login")
	}

	me := doJSON(t, srv, "GET", "/me", "", token)
	if me.Code != http.StatusOK {
		t.Fatalf("me status = %d, want 200; body=%s", me.Code, me.Body.String())
	}
	if strings.Contains(me.Body.String(), "password_hash") {
		t.Fatal("/me leaked password_hash")
	}
}

func TestLoginWrongCredentials(t *testing.T) {
	srv := newTestServer(t)
	doJSON(t, srv, "POST", "/signup", `{"username":"alice","password":"hunter2pass"}`, "")

	wrongPass := doJSON(t, srv, "POST", "/login", `{"username":"alice","password":"wrongpass1"}`, "")
	noUser := doJSON(t, srv, "POST", "/login", `{"username":"bob","password":"hunter2pass"}`, "")
	for _, rec := range []*httptest.ResponseRecorder{wrongPass, noUser} {
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	}
	if wrongPass.Body.String() != noUser.Body.String() {
		t.Error("login error messages differ between wrong-password and unknown-user")
	}
}

func TestMeUnauthorized(t *testing.T) {
	srv := newTestServer(t)
	for _, token := range []string{"", "garbage"} {
		rec := doJSON(t, srv, "GET", "/me", "", token)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("token %q: status = %d, want 401", token, rec.Code)
		}
	}
}
