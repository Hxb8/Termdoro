// Command pomodoro is a terminal Pomodoro timer with Discord Rich Presence,
// YouTube background-music support and local session tracking.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Hxb8/Termdoro/assets"
	"github.com/Hxb8/Termdoro/internal/app"
	"github.com/Hxb8/Termdoro/internal/audio"
	"github.com/Hxb8/Termdoro/internal/discord"
	"github.com/Hxb8/Termdoro/internal/store"
	"github.com/Hxb8/Termdoro/internal/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	dataDir := store.DataDir()

	a := app.New(dataDir, assets.RainSound)
	a.Player = audio.New()

	presence := discord.New(discord.AppID)
	_ = presence.Connect()
	defer func() { _ = presence.Close() }()

	model := ui.NewModel(a, presence)
	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
