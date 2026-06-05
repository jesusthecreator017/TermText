package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

type Message struct {
	ID       string `json:"id"`
	RoomID   string `json:"room_id"`
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Body     string `json:"body"`
	SentAt   string `json:"sent_at"`
}

// RoomMessages fetches a page of a room's messages, oldest→newest. before is an
// optional message-id cursor for loading older pages.
func (c *Client) RoomMessages(ctx context.Context, token, roomID, before string, limit int) ([]Message, error) {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	if before != "" {
		q.Set("before", before)
	}
	path := "/rooms/" + roomID + "/messages"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}

	var body struct {
		Messages []Message `json:"messages"`
	}
	if err := c.doJSON(ctx, http.MethodGet, path, token, nil, &body); err != nil {
		return nil, err
	}
	return body.Messages, nil
}

func (c *Client) Search(ctx context.Context, token, query string) ([]Message, error) {
	path := "/search?q=" + url.QueryEscape(query)
	var body struct {
		Results []Message `json:"results"`
	}
	if err := c.doJSON(ctx, http.MethodGet, path, token, nil, &body); err != nil {
		return nil, err
	}
	return body.Results, nil
}
