package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jesusthecreator017/TermText/internal/client"
	"github.com/jesusthecreator017/TermText/internal/tui"
)

func main() {
	defaultServer := os.Getenv("TERMTEXT_SERVER")
	if defaultServer == "" {
		defaultServer = "http://localhost:8080"
	}
	server := flag.String("server", defaultServer, "TermText server base URL")
	flag.Parse()

	c := client.New(*server)
	p := tea.NewProgram(tui.New(c), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
