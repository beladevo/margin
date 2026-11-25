package store

import (
	"path/filepath"
	"testing"
)

func TestSearch(t *testing.T) {
	s := newStore(t)
	writeFile(t, s, "groceries.md", "# Groceries\n\n- milk\n- Oat MILK\n")
	writeFile(t, s, "work/standup.md", "# Standup\n\nnothing here\n")
	writeFile(t, s, ".trash/old.md", "milk\n")

	matches, err := s.Search("milk", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Fatalf("matches = %+v", matches)
	}
	if matches[0].Line != 3 || matches[0].Text != "- milk" {
		t.Fatalf("first match = %+v", matches[0])
	}
	if matches[1].Line != 4 {
		t.Fatalf("second match = %+v", matches[1])
	}
}

func TestSearchMatchesTitles(t *testing.T) {
	s := newStore(t)
	writeFile(t, s, "work/standup.md", "nothing\n")
	matches, err := s.Search("STANDUP", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].Path != filepath.Join("work", "standup.md") || matches[0].Line != 0 {
		t.Fatalf("matches = %+v", matches)
	}
}

func TestSearchLimitAndEmpty(t *testing.T) {
	s := newStore(t)
	writeFile(t, s, "a.md", "x\nx\nx\n")
	if m, _ := s.Search("x", 2); len(m) != 2 {
		t.Fatalf("limit ignored: %d", len(m))
	}
	if m, _ := s.Search("  ", 0); m != nil {
		t.Fatalf("empty query returned %v", m)
	}
}
