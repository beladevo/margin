package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg != Default() {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestLoadOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	writeConfig(t, path, "notes_dir = \"/tmp/vault\"\ntheme = \"light\"\nautosave_delay = \"250ms\"\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := Config{NotesDir: "/tmp/vault", Theme: ThemeLight, AutosaveDelay: 250 * time.Millisecond}
	if cfg != want {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestLoadRejectsBadValues(t *testing.T) {
	for _, body := range []string{`theme = "neon"`, `autosave_delay = "0s"`, `notes_dir = `} {
		path := filepath.Join(t.TempDir(), "config.toml")
		writeConfig(t, path, body)
		if _, err := Load(path); err == nil {
			t.Errorf("Load(%q) succeeded", body)
		}
	}
}

func TestDefaultPathHonorsXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	got, _ := DefaultPath()
	if got != filepath.Join("/xdg", "margin", "config.toml") {
		t.Fatalf("path = %q", got)
	}
}

func TestExpandHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	if got, _ := ExpandHome("~/notes"); got != filepath.Join(home, "notes") {
		t.Fatalf("got %q", got)
	}
	if got, _ := ExpandHome("/abs"); got != "/abs" {
		t.Fatalf("got %q", got)
	}
}

func writeConfig(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
