package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
)

type Room struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	DisplayName string `json:"display_name"`
	PeerID      string `json:"peer_id,omitempty"`
}

func (c *Client) ListRooms(ctx context.Context, token string) ([]Room, error) {
	return c.listRooms(ctx, token, "/rooms")
}

// ListPublicRooms returns joinable public rooms the caller is not in.
func (c *Client) ListPublicRooms(ctx context.Context, token string) ([]Room, error) {
	return c.listRooms(ctx, token, "/rooms/public")
}

func (c *Client) listRooms(ctx context.Context, token, path string) ([]Room, error) {
	var body struct {
		Rooms []Room `json:"rooms"`
	}
	if err := c.doJSON(ctx, http.MethodGet, path, token, nil, &body); err != nil {
		return nil, err
	}
	return body.Rooms, nil
}

func (c *Client) CreateRoom(ctx context.Context, token, name string) (Room, error) {
	return c.roomMutation(ctx, "/rooms", token, map[string]string{"name": name})
}

func (c *Client) CreateDM(ctx context.Context, token, username string) (Room, error) {
	return c.roomMutation(ctx, "/dms", token, map[string]string{"username": username})
}

// JoinRoom joins a public room by UUID or name, returning the joined room.
func (c *Client) JoinRoom(ctx context.Context, token, ident string) (Room, error) {
	var body struct {
		Room Room `json:"room"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/rooms/"+url.PathEscape(ident)+"/join", token, nil, &body); err != nil {
		return Room{}, err
	}
	return body.Room, nil
}

func (c *Client) roomMutation(ctx context.Context, path, token string, payload map[string]string) (Room, error) {
	var body struct {
		Room Room `json:"room"`
	}
	if err := c.doJSON(ctx, http.MethodPost, path, token, payload, &body); err != nil {
		return Room{}, err
	}
	return body.Room, nil
}

func (c *Client) doJSON(ctx context.Context, method, path, token string, payload any, dst any) error {
	var reader *bytes.Reader
	if payload != nil {
		data, _ := json.Marshal(payload)
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return errors.New(serverError(resp))
	}
	if dst != nil {
		return json.NewDecoder(resp.Body).Decode(dst)
	}
	return nil
}
