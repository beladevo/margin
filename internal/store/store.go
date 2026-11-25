package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	NoteExt  = ".md"
	TrashDir = ".trash"
)

var (
	ErrExists      = errors.New("already exists")
	ErrInvalidName = errors.New("invalid name")
	ErrOutsideRoot = errors.New("path is outside the notes folder")
)

type Store struct {
	Root string
}

type Node struct {
	Name     string
	Path     string
	IsDir    bool
	Children []*Node
}

func New(root string) (*Store, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	return &Store{Root: abs}, nil
}

func (s *Store) Abs(rel string) (string, error) {
	if rel == "" || rel == "." {
		return s.Root, nil
	}
	if !filepath.IsLocal(rel) {
		return "", ErrOutsideRoot
	}
	return filepath.Join(s.Root, rel), nil
}

func (s *Store) Tree() (*Node, error) {
	root := &Node{Name: filepath.Base(s.Root), IsDir: true}
	return root, s.fill(root)
}

func (s *Store) fill(n *Node) error {
	dir, err := s.Abs(n.Path)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		rel := filepath.Join(n.Path, name)
		switch {
		case e.IsDir():
			child := &Node{Name: name, Path: rel, IsDir: true}
			if err := s.fill(child); err != nil {
				return err
			}
			n.Children = append(n.Children, child)
		case strings.EqualFold(filepath.Ext(name), NoteExt):
			n.Children = append(n.Children, &Node{Name: Title(rel), Path: rel})
		}
	}
	sort.SliceStable(n.Children, func(i, j int) bool {
		a, b := n.Children[i], n.Children[j]
		if a.IsDir != b.IsDir {
			return a.IsDir
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	return nil
}

func (s *Store) Notes() ([]string, error) {
	var notes []string
	err := filepath.WalkDir(s.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != s.Root && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(path), NoteExt) {
			rel, err := filepath.Rel(s.Root, path)
			if err != nil {
				return err
			}
			notes = append(notes, rel)
		}
		return nil
	})
	return notes, err
}

func Title(rel string) string {
	base := filepath.Base(rel)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func (s *Store) Read(rel string) (string, time.Time, error) {
	path, err := s.Abs(rel)
	if err != nil {
		return "", time.Time{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", time.Time{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", time.Time{}, err
	}
	return string(data), info.ModTime(), nil
}

func (s *Store) ModTime(rel string) (time.Time, error) {
	path, err := s.Abs(rel)
	if err != nil {
		return time.Time{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

// Write replaces the file via a temp file in the same directory and a rename,
// so a crash mid-write never leaves a truncated note behind.
func (s *Store) Write(rel, content string) (time.Time, error) {
	path, err := s.Abs(rel)
	if err != nil {
		return time.Time{}, err
	}
	mode := fs.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".margin-*.tmp")
	if err != nil {
		return time.Time{}, err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return time.Time{}, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return time.Time{}, err
	}
	if err := tmp.Close(); err != nil {
		return time.Time{}, err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return time.Time{}, err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return time.Time{}, err
	}
	return s.ModTime(rel)
}

func SanitizeName(name string) (string, error) {
	name = strings.Map(func(r rune) rune {
		switch {
		case r == '/' || r == '\\' || r == ':':
			return '-'
		case r < 0x20 || r == 0x7f:
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	name = strings.TrimLeft(name, ".")
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrInvalidName
	}
	return name, nil
}

func (s *Store) CreateNote(dir, title string) (string, error) {
	name, err := SanitizeName(title)
	if err != nil {
		return "", err
	}
	rel := filepath.Join(dir, name+NoteExt)
	path, err := s.Abs(rel)
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return "", fmt.Errorf("%q %w", name, ErrExists)
	}
	if err != nil {
		return "", err
	}
	_, werr := fmt.Fprintf(f, "# %s\n\n", strings.TrimSpace(title))
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	return rel, werr
}

func (s *Store) CreateDir(parent, name string) (string, error) {
	clean, err := SanitizeName(name)
	if err != nil {
		return "", err
	}
	rel := filepath.Join(parent, clean)
	path, err := s.Abs(rel)
	if err != nil {
		return "", err
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("%q %w", clean, ErrExists)
		}
		return "", err
	}
	return rel, nil
}

func (s *Store) Rename(rel, newName string) (string, error) {
	src, err := s.Abs(rel)
	if err != nil || src == s.Root {
		return "", ErrOutsideRoot
	}
	info, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	clean, err := SanitizeName(newName)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		clean += filepath.Ext(rel)
	}
	newRel := filepath.Join(filepath.Dir(rel), clean)
	if newRel == rel {
		return rel, nil
	}
	dst, err := s.Abs(newRel)
	if err != nil {
		return "", err
	}
	// Case-only renames on case-insensitive filesystems report dst as existing.
	if _, err := os.Stat(dst); err == nil && !strings.EqualFold(newRel, rel) {
		return "", fmt.Errorf("%q %w", clean, ErrExists)
	}
	if err := os.Rename(src, dst); err != nil {
		return "", err
	}
	return newRel, nil
}

func (s *Store) Trash(rel string) (string, error) {
	src, err := s.Abs(rel)
	if err != nil || src == s.Root {
		return "", ErrOutsideRoot
	}
	if _, err := os.Stat(src); err != nil {
		return "", err
	}
	trashRel := filepath.Join(TrashDir, rel)
	dst := filepath.Join(s.Root, trashRel)
	if _, err := os.Stat(dst); err == nil {
		ext := filepath.Ext(rel)
		stamp := time.Now().Format("20060102-150405.000")
		trashRel = filepath.Join(TrashDir, strings.TrimSuffix(rel, ext)+" "+stamp+ext)
		dst = filepath.Join(s.Root, trashRel)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(src, dst); err != nil {
		return "", err
	}
	return trashRel, nil
}
