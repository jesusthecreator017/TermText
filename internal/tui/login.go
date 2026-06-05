package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jesusthecreator017/TermText/internal/client"
)

type authResultMsg struct {
	token    string
	username string
	password string // kept in memory only, to unlock/derive the E2EE key
	signup   bool
	err      error
}

type loginModel struct {
	client     *client.Client
	inputs     []textinput.Model
	focus      int
	signup     bool
	submitting bool
	errMsg     string
}

func newLoginModel(c *client.Client, prefillUser string) loginModel {
	username := textinput.New()
	username.Placeholder = "username"
	username.SetValue(prefillUser)
	username.Focus()
	username.CharLimit = 32

	password := textinput.New()
	password.Placeholder = "password"
	password.EchoMode = textinput.EchoPassword
	password.EchoCharacter = '•'
	password.CharLimit = 128

	return loginModel{
		client: c,
		inputs: []textinput.Model{username, password},
	}
}

func (m loginModel) update(msg tea.Msg) (loginModel, tea.Cmd) {
	switch msg := msg.(type) {
	case authResultMsg:
		m.submitting = false
		if msg.err != nil {
			m.errMsg = msg.err.Error()
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "shift+tab", "up", "down":
			m.focus = (m.focus + 1) % len(m.inputs)
			for i := range m.inputs {
				if i == m.focus {
					m.inputs[i].Focus()
				} else {
					m.inputs[i].Blur()
				}
			}
			return m, nil
		case "ctrl+t":
			m.signup = !m.signup
			m.errMsg = ""
			return m, nil
		case "enter":
			if m.submitting {
				return m, nil
			}
			return m.submit()
		}
	}

	var cmd tea.Cmd
	m.inputs[m.focus], cmd = m.inputs[m.focus].Update(msg)
	return m, cmd
}

func (m loginModel) submit() (loginModel, tea.Cmd) {
	username := m.inputs[0].Value()
	password := m.inputs[1].Value()
	if username == "" || password == "" {
		m.errMsg = "username and password are required"
		return m, nil
	}
	m.submitting = true
	m.errMsg = ""

	c := m.client
	signup := m.signup
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		var token, name string
		var err error
		if signup {
			token, name, err = c.Signup(ctx, username, password)
		} else {
			token, name, err = c.Login(ctx, username, password)
		}
		return authResultMsg{token: token, username: name, password: password, signup: signup, err: err}
	}
}

func (m loginModel) view() string {
	title := "Log in"
	action := "log in"
	if m.signup {
		title = "Sign up"
		action = "sign up"
	}

	var b lipgloss.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Bold(true)
	out := b.Render("TermText — "+title) + "\n\n"
	out += m.inputs[0].View() + "\n"
	out += m.inputs[1].View() + "\n\n"

	if m.submitting {
		out += lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("…contacting server") + "\n"
	} else if m.errMsg != "" {
		out += lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(m.errMsg) + "\n"
	}

	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(
		"\nenter: " + action + " · tab: switch field · ctrl+t: toggle login/signup · ctrl+c: quit")
	return out + hint
}
