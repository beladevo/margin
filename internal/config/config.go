package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

const (
	ThemeAuto  = "auto"
	ThemeDark  = "dark"
	ThemeLight = "light"
)

type Config struct {
	NotesDir      string        `toml:"notes_dir"`
	Theme         string        `toml:"theme"`
	AutosaveDelay time.Duration `toml:"autosave_delay"`
}

func Default() Config {
	return Config{
		NotesDir:      "~/notes",
		Theme:         ThemeAuto,
		AutosaveDelay: time.Second,
	}
}

func DefaultPath() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "margin", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "margin", "config.toml"), nil
}

func Load(path string) (Config, error) {
	cfg := Default()
	if _, err := toml.DecodeFile(path, &cfg); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return cfg, fmt.Errorf("reading %s: %w", path, err)
	}
	switch cfg.Theme {
	case ThemeAuto, ThemeDark, ThemeLight:
	default:
		return cfg, fmt.Errorf("theme must be auto, dark or light, got %q", cfg.Theme)
	}
	if cfg.AutosaveDelay <= 0 {
		return cfg, fmt.Errorf("autosave_delay must be positive, got %s", cfg.AutosaveDelay)
	}
	return cfg, nil
}

func ExpandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~")), nil
}
