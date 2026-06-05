// Package tui implements the TermText terminal client (BubbleTea).
package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jesusthecreator017/TermText/internal/client"
	"github.com/jesusthecreator017/TermText/internal/crypto"
)

type state int

const (
	stateLogin state = iota
	stateConnecting
	stateChat
)

type sessionValidMsg struct {
	token    string
	username string
	ok       bool
}

type connectedMsg struct {
	conn     *client.Conn
	username string
	token    string
	err      error
}

type keyReadyMsg struct {
	priv  *[crypto.KeySize]byte
	token string
	user  string
}

type Model struct {
	client *client.Client
	state  state

	login loginModel
	chat  chatModel
	spin  spinner.Model
	priv  *[crypto.KeySize]byte

	width  int
	height int
	fatal  string
}

func New(c *client.Client) Model {
	prefill := ""
	if s, err := client.LoadSession(); err == nil && s.Server == c.BaseURL() {
		prefill = s.Username
	}
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colAccent)
	return Model{
		client: c,
		state:  stateLogin,
		login:  newLoginModel(c, prefill),
		spin:   sp,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(tea.EnterAltScreen, m.spin.Tick, resumeCmd(m.client))
}

func resumeCmd(c *client.Client) tea.Cmd {
	return func() tea.Msg {
		s, err := client.LoadSession()
		if err != nil || s.Token == "" || s.Server != c.BaseURL() {
			return sessionValidMsg{ok: false}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		name, err := c.Validate(ctx, s.Token)
		if err != nil {
			return sessionValidMsg{ok: false}
		}
		return sessionValidMsg{token: s.Token, username: name, ok: true}
	}
}

func connectCmd(c *client.Client, token, username string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		conn, err := c.ConnectWS(ctx, token)
		return connectedMsg{conn: conn, username: username, token: token, err: err}
	}
}

// keySetupCmd provisions the user's E2EE key: signup (and first login since the
// feature shipped) generates + uploads + stores; later logins load locally.
func keySetupCmd(c *client.Client, token, username, password string, signup bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		bootstrap := func() *[crypto.KeySize]byte {
			pub, priv, err := crypto.GenerateKeypair()
			if err != nil {
				return nil
			}
			if err := c.SetPublicKey(ctx, token, pub[:]); err != nil {
				return nil
			}
			if err := client.SaveKeys(username, password, pub, priv); err != nil {
				return nil
			}
			return priv
		}

		if signup || !client.HasKeys(username) {
			return keyReadyMsg{priv: bootstrap(), token: token, user: username}
		}
		_, priv, err := client.LoadKeys(username, password)
		if err != nil {
			priv = nil // corrupt key file → operate locked
		}
		return keyReadyMsg{priv: priv, token: token, user: username}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.state == stateChat {
			var cmd tea.Cmd
			m.chat, cmd = m.chat.update(msg)
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			if m.chat.conn != nil {
				m.chat.conn.Close()
			}
			return m, tea.Quit
		case "ctrl+l":
			if m.state == stateChat {
				return m.signOut(), nil
			}
		}

	case sessionValidMsg:
		if msg.ok {
			m.state = stateConnecting
			return m, connectCmd(m.client, msg.token, msg.username)
		}
		return m, nil

	case authResultMsg:
		var cmd tea.Cmd
		m.login, cmd = m.login.update(msg)
		if msg.err == nil && msg.token != "" {
			_ = client.SaveSession(client.Session{
				Server:   m.client.BaseURL(),
				Token:    msg.token,
				Username: msg.username,
			})
			m.state = stateConnecting
			return m, keySetupCmd(m.client, msg.token, msg.username, msg.password, msg.signup)
		}
		return m, cmd

	case keyReadyMsg:
		m.priv = msg.priv
		return m, connectCmd(m.client, msg.token, msg.user)

	case connectedMsg:
		if msg.err != nil {
			m.state = stateLogin
			m.login.errMsg = "connect failed: " + msg.err.Error()
			m.login.submitting = false
			return m, nil
		}
		m.chat = newChatModel(m.client, msg.token, msg.conn, msg.username, m.priv)
		m.state = stateChat
		var cmd tea.Cmd
		if m.width > 0 {
			m.chat, cmd = m.chat.update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		}
		return m, tea.Batch(cmd, readCmd(msg.conn), m.chat.initCmd())
	}

	switch m.state {
	case stateLogin:
		var cmd tea.Cmd
		m.login, cmd = m.login.update(msg)
		return m, cmd
	case stateChat:
		var cmd tea.Cmd
		m.chat, cmd = m.chat.update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) signOut() Model {
	prefill := m.chat.username
	if m.chat.conn != nil {
		m.chat.conn.Close()
	}
	_ = client.DeleteSession()
	m.chat = chatModel{}
	m.login = newLoginModel(m.client, prefill)
	m.state = stateLogin
	return m
}

func (m Model) View() string {
	switch m.state {
	case stateConnecting:
		return m.spin.View() + " Connecting…  (ctrl+c to quit)"
	case stateChat:
		return m.chat.view()
	default:
		return m.login.view()
	}
}
