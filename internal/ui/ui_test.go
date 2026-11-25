package ui

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/muesli/termenv"

	"github.com/beladevo/margin/internal/store"
)

const never = time.Hour

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

type harness struct {
	t     *testing.T
	store *store.Store
	tm    *teatest.TestModel
}

func start(t *testing.T, files map[string]string, opts Options) *harness {
	t.Helper()
	s, err := store.New(filepath.Join(t.TempDir(), "notes"))
	if err != nil {
		t.Fatal(err)
	}
	for rel, content := range files {
		path := filepath.Join(s.Root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	opts.Store = s
	opts.GlamourStyle = "notty"
	if opts.AutosaveDelay == 0 {
		opts.AutosaveDelay = never
	}
	if opts.CheckInterval == 0 {
		opts.CheckInterval = never
	}
	m, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	return &harness{t: t, store: s, tm: tm}
}

func (h *harness) typeText(s string)        { h.tm.Type(s) }
func (h *harness) press(k tea.KeyType)      { h.tm.Send(tea.KeyMsg{Type: k}) }
func (h *harness) path(rel string) string   { return filepath.Join(h.store.Root, rel) }
func (h *harness) waitOutput(substr string) { h.waitOutputBytes([]byte(substr)) }

func (h *harness) waitOutputBytes(substr []byte) {
	h.t.Helper()
	teatest.WaitFor(h.t, h.tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, substr)
	}, teatest.WithDuration(3*time.Second), teatest.WithCheckInterval(10*time.Millisecond))
}

func (h *harness) waitFile(rel, want string) {
	h.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var got string
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(h.path(rel))
		got = string(data)
		if err == nil && got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.t.Fatalf("%s = %q, want %q", rel, got, want)
}

func (h *harness) final() Model {
	h.t.Helper()
	h.press(tea.KeyCtrlC)
	return h.tm.FinalModel(h.t, teatest.WithFinalTimeout(3*time.Second)).(Model)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestCreateNoteTypeAndEscSaves(t *testing.T) {
	h := start(t, nil, Options{})
	h.typeText("n")
	h.typeText("Hello")
	h.press(tea.KeyEnter)
	h.waitOutput("EDIT")
	h.typeText("world")
	h.press(tea.KeyEsc)
	h.waitFile("Hello.md", "# Hello\n\nworld")

	m := h.final()
	if m.mode != modeBrowse || m.current != "Hello.md" {
		t.Fatalf("mode = %v, current = %q", m.mode, m.current)
	}
}

func TestDuplicateNoteNameIsRejected(t *testing.T) {
	h := start(t, map[string]string{"Hello.md": "x"}, Options{})
	h.typeText("n")
	h.typeText("Hello")
	h.press(tea.KeyEnter)
	h.waitOutput("already exists")
	h.press(tea.KeyEsc)
	if got := readFile(t, h.path("Hello.md")); got != "x" {
		t.Fatalf("existing note changed: %q", got)
	}
	if m := h.final(); m.mode != modeBrowse {
		t.Fatalf("mode = %v", m.mode)
	}
}

func TestAutosaveWhileTyping(t *testing.T) {
	h := start(t, map[string]string{"a.md": "x"}, Options{AutosaveDelay: 50 * time.Millisecond})
	h.typeText("e")
	h.waitOutput("EDIT")
	h.typeText("yz")
	h.waitFile("a.md", "yzx")
	if m := h.final(); m.mode != modeEdit {
		t.Fatalf("autosave left edit mode: %v", m.mode)
	}
}

func TestQuitSavesUnsavedEdits(t *testing.T) {
	h := start(t, map[string]string{"a.md": "x"}, Options{})
	h.typeText("e")
	h.waitOutput("EDIT")
	h.typeText("z")
	h.waitOutput("modified")
	h.final()
	if got := readFile(t, h.path("a.md")); got != "zx" {
		t.Fatalf("a.md = %q", got)
	}
}

func TestExternalChangeRaisesConflictInsteadOfOverwriting(t *testing.T) {
	h := start(t, map[string]string{"a.md": "orig"}, Options{})
	h.typeText("e")
	h.waitOutput("EDIT")
	h.typeText("mine")
	h.waitOutput("modified")

	if err := os.WriteFile(h.path("a.md"), []byte("theirs"), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Minute)
	if err := os.Chtimes(h.path("a.md"), future, future); err != nil {
		t.Fatal(err)
	}

	h.press(tea.KeyEsc)
	h.waitOutput("File changed on disk")
	if got := readFile(t, h.path("a.md")); got != "theirs" {
		t.Fatalf("external change was overwritten: %q", got)
	}

	h.press(tea.KeyCtrlK)
	h.waitFile("a.md", "mineorig")
	if m := h.final(); m.conflict != conflictNone {
		t.Fatalf("conflict = %v", m.conflict)
	}
}

func TestConflictReloadTakesDiskVersion(t *testing.T) {
	h := start(t, map[string]string{"a.md": "orig"}, Options{})
	h.typeText("e")
	h.waitOutput("EDIT")
	h.typeText("mine")
	h.waitOutput("modified")
	if err := os.Remove(h.path("a.md")); err != nil {
		t.Fatal(err)
	}
	h.press(tea.KeyCtrlS)
	h.waitOutput("File was deleted on disk")
	h.press(tea.KeyCtrlR)

	m := h.final()
	if m.current != "" || m.mode != modeBrowse {
		t.Fatalf("current = %q, mode = %v", m.current, m.mode)
	}
	if _, err := os.Stat(h.path("a.md")); !os.IsNotExist(err) {
		t.Fatalf("deleted note was recreated: %v", err)
	}
}

func TestBrowseReloadsSilentlyOnExternalChange(t *testing.T) {
	h := start(t, map[string]string{"a.md": "first version"}, Options{CheckInterval: 20 * time.Millisecond})
	h.waitOutput("first version")
	if err := os.WriteFile(h.path("a.md"), []byte("second version"), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Minute)
	if err := os.Chtimes(h.path("a.md"), future, future); err != nil {
		t.Fatal(err)
	}
	h.waitOutput("second version")
	h.final()
}

func TestUndoRevertsTyping(t *testing.T) {
	h := start(t, map[string]string{"a.md": "x"}, Options{})
	h.typeText("e")
	h.waitOutput("EDIT")
	h.typeText("ab")
	h.waitOutput("modified")
	h.press(tea.KeyCtrlZ)
	h.waitOutput("saved")
	m := h.final()
	if m.editor.value() != "x" || m.dirty {
		t.Fatalf("value = %q, dirty = %v", m.editor.value(), m.dirty)
	}
	ed := newEditor()
	ed.load("x")
	ed.focus()
	ed.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	ed.undoChange()
	ed.redoChange()
	if ed.value() != "qx" {
		t.Fatalf("redo value = %q", ed.value())
	}
}

func TestDeleteMovesToTrash(t *testing.T) {
	h := start(t, map[string]string{"a.md": "x"}, Options{})
	h.waitOutput("a.md")
	h.typeText("d")
	h.waitOutput("Move \"a.md\" to trash?")
	h.typeText("y")
	h.waitOutput("Moved to")
	m := h.final()
	if m.current != "" {
		t.Fatalf("current = %q", m.current)
	}
	if got := readFile(t, h.path(filepath.Join(store.TrashDir, "a.md"))); got != "x" {
		t.Fatalf("trashed content = %q", got)
	}
}

func TestRenameFollowsOpenNote(t *testing.T) {
	h := start(t, map[string]string{"a.md": "x"}, Options{})
	h.waitOutput("a.md")
	h.typeText("r")
	h.press(tea.KeyBackspace)
	h.typeText("b")
	h.press(tea.KeyEnter)
	h.waitOutput("Renamed to")
	m := h.final()
	if m.current != "b.md" {
		t.Fatalf("current = %q", m.current)
	}
	if got := readFile(t, h.path("b.md")); got != "x" {
		t.Fatalf("b.md = %q", got)
	}
}

func TestNewFolderThenNoteInside(t *testing.T) {
	h := start(t, nil, Options{})
	h.typeText("N")
	h.typeText("work")
	h.press(tea.KeyEnter)
	h.waitOutput("Created folder")
	h.typeText("n")
	h.typeText("todo")
	h.press(tea.KeyEnter)
	h.waitOutput("EDIT")
	h.press(tea.KeyEsc)
	h.waitOutput("VIEW")
	if m := h.final(); m.current != filepath.Join("work", "todo.md") {
		t.Fatalf("current = %q", m.current)
	}
}

func TestFinderOpensFuzzyMatch(t *testing.T) {
	h := start(t, map[string]string{"alpha.md": "a", "work/beta.md": "b"}, Options{})
	h.press(tea.KeyCtrlP)
	h.waitOutput("Find note")
	h.typeText("bta")
	h.press(tea.KeyEnter)
	h.waitOutput("VIEW")
	if m := h.final(); m.current != filepath.Join("work", "beta.md") {
		t.Fatalf("current = %q", m.current)
	}
}

func TestSearchOpensNoteContainingText(t *testing.T) {
	h := start(t, map[string]string{"alpha.md": "nothing", "beta.md": "a needle here"}, Options{})
	h.press(tea.KeyCtrlF)
	h.waitOutput("Search notes")
	h.typeText("needle")
	h.waitOutput("beta:1")
	h.press(tea.KeyEnter)
	if m := h.final(); m.current != "beta.md" {
		t.Fatalf("current = %q", m.current)
	}
}

func TestRendersMarkdown(t *testing.T) {
	h := start(t, map[string]string{"a.md": "# Title\n\n- one\n- two\n"}, Options{})
	h.waitOutput("• one")
	h.final()
}

func TestRebase(t *testing.T) {
	sep := string(filepath.Separator)
	cases := []struct {
		rel, target, newTarget, want string
		ok                           bool
	}{
		{"a.md", "a.md", "b.md", "b.md", true},
		{"work" + sep + "a.md", "work", "jobs", "jobs" + sep + "a.md", true},
		{"workshop" + sep + "a.md", "work", "jobs", "", false},
		{"", "work", "jobs", "", false},
	}
	for _, c := range cases {
		got, ok := rebase(c.rel, c.target, c.newTarget)
		if got != c.want || ok != c.ok {
			t.Errorf("rebase(%q, %q, %q) = %q, %v", c.rel, c.target, c.newTarget, got, ok)
		}
	}
}
