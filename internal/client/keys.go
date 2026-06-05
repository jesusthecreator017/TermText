package client

import (
	"context"
	"encoding/base64"
	"net/http"
)

func (c *Client) SetPublicKey(ctx context.Context, token string, pub []byte) error {
	payload := map[string]string{"public_key": base64.StdEncoding.EncodeToString(pub)}
	return c.doJSON(ctx, http.MethodPut, "/me/public_key", token, payload, nil)
}

// GetPublicKey fetches another user's public key (32 bytes). Returns a nil slice
// with no error semantics handled by the caller via the returned error.
func (c *Client) GetPublicKey(ctx context.Context, token, userID string) ([]byte, string, error) {
	var body struct {
		Username  string `json:"username"`
		PublicKey string `json:"public_key"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/users/"+userID+"/public_key", token, nil, &body); err != nil {
		return nil, "", err
	}
	key, err := base64.StdEncoding.DecodeString(body.PublicKey)
	if err != nil {
		return nil, "", err
	}
	return key, body.Username, nil
}
