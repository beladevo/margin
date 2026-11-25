package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/beladevo/margin/internal/config"
	"github.com/beladevo/margin/internal/store"
	"github.com/beladevo/margin/internal/ui"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "margin:", err)
		os.Exit(1)
	}
}

func run() error {
	defaultConfig, err := config.DefaultPath()
	if err != nil {
		return err
	}
	configPath := flag.String("config", defaultConfig, "path to config file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: margin [flags] [notes-dir]\n\nA terminal markdown notes editor.\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion {
		fmt.Println("margin", version)
		return nil
	}
	if flag.NArg() > 1 {
		flag.Usage()
		os.Exit(2)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	dir := cfg.NotesDir
	if flag.NArg() == 1 {
		dir = flag.Arg(0)
	}
	if dir, err = config.ExpandHome(dir); err != nil {
		return err
	}
	s, err := store.New(dir)
	if err != nil {
		return err
	}

	model, err := ui.New(ui.Options{
		Store:         s,
		GlamourStyle:  glamourStyle(cfg.Theme),
		AutosaveDelay: cfg.AutosaveDelay,
	})
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}

// glamourStyle resolves the theme before the program starts, because querying
// the terminal background once Bubble Tea owns stdin races with its input reader.
func glamourStyle(theme string) string {
	switch theme {
	case config.ThemeLight:
		lipgloss.SetHasDarkBackground(false)
		return "light"
	case config.ThemeDark:
		lipgloss.SetHasDarkBackground(true)
		return "dark"
	}
	if lipgloss.HasDarkBackground() {
		return "dark"
	}
	return "light"
}
