package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(filepath.Join(t.TempDir(), "notes"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func writeFile(t *testing.T, s *Store, rel, content string) {
	t.Helper()
	path := filepath.Join(s.Root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNewCreatesRoot(t *testing.T) {
	s := newStore(t)
	if info, err := os.Stat(s.Root); err != nil || !info.IsDir() {
		t.Fatalf("root not created: %v", err)
	}
}

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"Meeting with Dital": "Meeting with Dital",
		"a/b\\c:d":           "a-b-c-d",
		"  padded  ":         "padded",
		".hidden":            "hidden",
		"tab\there":          "tabhere",
	}
	for in, want := range cases {
		got, err := SanitizeName(in)
		if err != nil || got != want {
			t.Errorf("SanitizeName(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "   ", "...", ".."} {
		if _, err := SanitizeName(bad); !errors.Is(err, ErrInvalidName) {
			t.Errorf("SanitizeName(%q) err = %v; want ErrInvalidName", bad, err)
		}
	}
}

func TestCreateNote(t *testing.T) {
	s := newStore(t)
	rel, err := s.CreateNote("", "Meeting: Q4")
	if err != nil {
		t.Fatal(err)
	}
	if rel != "Meeting- Q4.md" {
		t.Fatalf("rel = %q", rel)
	}
	content, _, err := s.Read(rel)
	if err != nil {
		t.Fatal(err)
	}
	if content != "# Meeting: Q4\n\n" {
		t.Fatalf("content = %q", content)
	}
	if _, err := s.CreateNote("", "Meeting: Q4"); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate err = %v; want ErrExists", err)
	}
}

func TestCreateNoteInSubdir(t *testing.T) {
	s := newStore(t)
	dir, err := s.CreateDir("", "work")
	if err != nil {
		t.Fatal(err)
	}
	rel, err := s.CreateNote(dir, "todo")
	if err != nil {
		t.Fatal(err)
	}
	if rel != filepath.Join("work", "todo.md") {
		t.Fatalf("rel = %q", rel)
	}
	if _, err := s.CreateDir("", "work"); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate dir err = %v", err)
	}
}

func TestWriteIsAtomicAndPreservesMode(t *testing.T) {
	s := newStore(t)
	writeFile(t, s, "a.md", "old")
	if err := os.Chmod(filepath.Join(s.Root, "a.md"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write("a.md", "new"); err != nil {
		t.Fatal(err)
	}
	content, _, _ := s.Read("a.md")
	if content != "new" {
		t.Fatalf("content = %q", content)
	}
	info, _ := os.Stat(filepath.Join(s.Root, "a.md"))
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(s.Root)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func TestWriteReturnsNewModTime(t *testing.T) {
	s := newStore(t)
	writeFile(t, s, "a.md", "x")
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(s.Root, "a.md"), past, past); err != nil {
		t.Fatal(err)
	}
	mt, err := s.Write("a.md", "y")
	if err != nil {
		t.Fatal(err)
	}
	disk, _ := s.ModTime("a.md")
	if !mt.Equal(disk) || !mt.After(past) {
		t.Fatalf("modtime = %v, disk = %v", mt, disk)
	}
}

func TestRejectsPathsOutsideRoot(t *testing.T) {
	s := newStore(t)
	if _, err := s.Write("../escape.md", "x"); !errors.Is(err, ErrOutsideRoot) {
		t.Fatalf("err = %v", err)
	}
	if _, err := s.Trash(""); !errors.Is(err, ErrOutsideRoot) {
		t.Fatalf("trash root err = %v", err)
	}
}

func TestRename(t *testing.T) {
	s := newStore(t)
	writeFile(t, s, "work/a.md", "a")
	writeFile(t, s, "work/b.md", "b")

	newRel, err := s.Rename("work/a.md", "renamed")
	if err != nil {
		t.Fatal(err)
	}
	if newRel != filepath.Join("work", "renamed.md") {
		t.Fatalf("newRel = %q", newRel)
	}
	if _, err := s.Rename(newRel, "b"); !errors.Is(err, ErrExists) {
		t.Fatalf("clash err = %v", err)
	}
	dirRel, err := s.Rename("work", "jobs")
	if err != nil || dirRel != "jobs" {
		t.Fatalf("dir rename = %q, %v", dirRel, err)
	}
	if _, _, err := s.Read(filepath.Join("jobs", "b.md")); err != nil {
		t.Fatal(err)
	}
}

func TestRenameCaseOnly(t *testing.T) {
	s := newStore(t)
	writeFile(t, s, "note.md", "x")
	newRel, err := s.Rename("note.md", "Note")
	if err != nil || newRel != "Note.md" {
		t.Fatalf("rename = %q, %v", newRel, err)
	}
}

func TestTrash(t *testing.T) {
	s := newStore(t)
	writeFile(t, s, "work/a.md", "first")
	trashRel, err := s.Trash("work/a.md")
	if err != nil {
		t.Fatal(err)
	}
	if trashRel != filepath.Join(TrashDir, "work", "a.md") {
		t.Fatalf("trashRel = %q", trashRel)
	}
	if _, err := os.Stat(filepath.Join(s.Root, "work", "a.md")); !os.IsNotExist(err) {
		t.Fatalf("original still exists: %v", err)
	}

	writeFile(t, s, "work/a.md", "second")
	second, err := s.Trash("work/a.md")
	if err != nil {
		t.Fatal(err)
	}
	if second == trashRel {
		t.Fatal("second trash overwrote the first")
	}
	data, _ := os.ReadFile(filepath.Join(s.Root, trashRel))
	if string(data) != "first" {
		t.Fatalf("first trashed copy = %q", data)
	}
}

func TestTreeHidesDotfilesAndNonNotes(t *testing.T) {
	s := newStore(t)
	writeFile(t, s, "b.md", "")
	writeFile(t, s, "A.md", "")
	writeFile(t, s, "image.png", "")
	writeFile(t, s, ".obsidian/x.md", "")
	writeFile(t, s, ".trash/old.md", "")
	writeFile(t, s, "zdir/c.md", "")

	tree, err := s.Tree()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range tree.Children {
		names = append(names, c.Name)
	}
	if got := strings.Join(names, ","); got != "zdir,A,b" {
		t.Fatalf("children = %s", got)
	}
	if len(tree.Children[0].Children) != 1 || tree.Children[0].Children[0].Path != filepath.Join("zdir", "c.md") {
		t.Fatalf("zdir children wrong: %+v", tree.Children[0].Children)
	}
}

func TestNotesSkipsHidden(t *testing.T) {
	s := newStore(t)
	writeFile(t, s, "a.md", "")
	writeFile(t, s, "sub/b.md", "")
	writeFile(t, s, ".trash/c.md", "")
	notes, err := s.Notes()
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 {
		t.Fatalf("notes = %v", notes)
	}
}
