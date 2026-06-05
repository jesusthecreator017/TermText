// Package client talks to the TermText server: REST auth plus the chat WebSocket.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) BaseURL() string { return c.baseURL }

type authResponse struct {
	Token string `json:"token"`
	User  struct {
		Username string `json:"username"`
	} `json:"user"`
}

func (c *Client) Signup(ctx context.Context, username, password string) (token, name string, err error) {
	return c.authRequest(ctx, "/signup", username, password)
}

func (c *Client) Login(ctx context.Context, username, password string) (token, name string, err error) {
	return c.authRequest(ctx, "/login", username, password)
}

func (c *Client) authRequest(ctx context.Context, path, username, password string) (string, string, error) {
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", "", errors.New(serverError(resp))
	}

	var ar authResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return "", "", err
	}
	return ar.Token, ar.User.Username, nil
}

func (c *Client) Validate(ctx context.Context, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/me", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("session invalid")
	}

	var body struct {
		User struct {
			Username string `json:"username"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.User.Username, nil
}

func serverError(resp *http.Response) string {
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err == nil && body.Error != "" {
		return body.Error
	}
	return fmt.Sprintf("request failed (%d)", resp.StatusCode)
}
