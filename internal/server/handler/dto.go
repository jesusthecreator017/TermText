package handler

import (
	"regexp"
	"time"

	sqlcgen "github.com/jesusthecreator017/TermText/internal/db/sqlc"
	"github.com/jesusthecreator017/TermText/internal/uid"
)

type credentialsRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type userResponse struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	CreatedAt string `json:"created_at"`
}

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)

func validateCredentials(c credentialsRequest) string {
	if !usernameRe.MatchString(c.Username) {
		return "username must be 3-32 characters of letters, digits, or underscores"
	}
	if len(c.Password) < 8 || len(c.Password) > 128 {
		return "password must be between 8 and 128 characters"
	}
	return ""
}

func toUserResponse(u sqlcgen.User) userResponse {
	return userResponse{
		ID:        uid.String(u.ID),
		Username:  u.Username,
		CreatedAt: u.CreatedAt.Time.Format(time.RFC3339),
	}
}
