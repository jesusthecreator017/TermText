package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Send    key.Binding
	Switch  key.Binding
	Scroll  key.Binding
	SignOut key.Binding
	Quit    key.Binding

	Up     key.Binding
	Down   key.Binding
	Select key.Binding
	Cancel key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Send:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "send")),
		Switch:  key.NewBinding(key.WithKeys("ctrl+k"), key.WithHelp("ctrl+k", "rooms")),
		Scroll:  key.NewBinding(key.WithKeys("pgup", "pgdown"), key.WithHelp("pgup/pgdn", "scroll")),
		SignOut: key.NewBinding(key.WithKeys("ctrl+l"), key.WithHelp("ctrl+l", "sign out")),
		Quit:    key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),

		Up:     key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "up")),
		Down:   key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "down")),
		Select: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Cancel: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	}
}

func (k keyMap) chatHelp() []key.Binding {
	return []key.Binding{k.Send, k.Switch, k.Scroll, k.SignOut, k.Quit}
}

func (k keyMap) overlayHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Select, k.Cancel}
}
