package auth

import (
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"aidanwoods.dev/go-paseto"
)

const tokenIssuer = "termtext"

var ErrInvalidToken = errors.New("auth: invalid token")

type TokenMaker struct {
	key      paseto.V4SymmetricKey
	duration time.Duration
}

type Claims struct {
	UserID    string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

func NewTokenMaker(keyHex string, ttl time.Duration) (*TokenMaker, error) {
	b, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("auth: decode paseto key: %w", err)
	}
	key, err := paseto.V4SymmetricKeyFromBytes(b)
	if err != nil {
		return nil, fmt.Errorf("auth: paseto key: %w", err)
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &TokenMaker{key: key, duration: ttl}, nil
}

func (m *TokenMaker) Issue(userID string) (string, error) {
	now := time.Now()
	t := paseto.NewToken()
	t.SetIssuer(tokenIssuer)
	t.SetSubject(userID)
	t.SetIssuedAt(now)
	t.SetNotBefore(now)
	t.SetExpiration(now.Add(m.duration))
	return t.V4Encrypt(m.key, nil), nil
}

func (m *TokenMaker) Verify(token string) (*Claims, error) {
	parser := paseto.NewParser()
	parser.AddRule(paseto.NotExpired())
	parser.AddRule(paseto.IssuedBy(tokenIssuer))

	parsed, err := parser.ParseV4Local(m.key, token, nil)
	if err != nil {
		return nil, ErrInvalidToken
	}

	sub, err := parsed.GetSubject()
	if err != nil || sub == "" {
		return nil, ErrInvalidToken
	}
	iat, err := parsed.GetIssuedAt()
	if err != nil {
		return nil, ErrInvalidToken
	}
	exp, err := parsed.GetExpiration()
	if err != nil {
		return nil, ErrInvalidToken
	}

	return &Claims{UserID: sub, IssuedAt: iat, ExpiresAt: exp}, nil
}
