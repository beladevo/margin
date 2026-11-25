package store

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type Match struct {
	Path string
	Line int
	Text string
}

func (s *Store) Search(query string, limit int) ([]Match, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil, nil
	}
	notes, err := s.Notes()
	if err != nil {
		return nil, err
	}
	var matches []Match
	for _, rel := range notes {
		if strings.Contains(strings.ToLower(Title(rel)), query) {
			matches = append(matches, Match{Path: rel})
		}
		found, err := searchFile(filepath.Join(s.Root, rel), rel, query)
		if err != nil {
			continue
		}
		matches = append(matches, found...)
		if limit > 0 && len(matches) >= limit {
			return matches[:limit], nil
		}
	}
	return matches, nil
}

func searchFile(path, rel, query string) ([]Match, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var matches []Match
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if strings.Contains(strings.ToLower(line), query) {
			matches = append(matches, Match{Path: rel, Line: n, Text: strings.TrimSpace(line)})
		}
	}
	return matches, sc.Err()
}
