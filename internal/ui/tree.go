package ui

import (
	"path/filepath"
	"strings"

	"github.com/beladevo/margin/internal/store"
)

type treeRow struct {
	node  *store.Node
	depth int
}

type tree struct {
	root     *store.Node
	expanded map[string]bool
	rows     []treeRow
	cursor   int
	offset   int
}

func newTree() tree {
	return tree{expanded: map[string]bool{}}
}

func (t *tree) setRoot(root *store.Node) {
	selected := t.selectedPath()
	t.root = root
	t.rebuild()
	t.selectPath(selected)
}

func (t *tree) rebuild() {
	t.rows = t.rows[:0]
	if t.root != nil {
		t.walk(t.root.Children, 0)
	}
	t.cursor = clamp(t.cursor, 0, len(t.rows)-1)
}

func (t *tree) walk(nodes []*store.Node, depth int) {
	for _, n := range nodes {
		t.rows = append(t.rows, treeRow{node: n, depth: depth})
		if n.IsDir && t.expanded[n.Path] {
			t.walk(n.Children, depth+1)
		}
	}
}

func (t *tree) selected() *store.Node {
	if t.cursor < 0 || t.cursor >= len(t.rows) {
		return nil
	}
	return t.rows[t.cursor].node
}

func (t *tree) selectedPath() string {
	if n := t.selected(); n != nil {
		return n.Path
	}
	return ""
}

func (t *tree) selectPath(rel string) bool {
	if rel == "" {
		return false
	}
	for dir := filepath.Dir(rel); dir != "." && dir != "/"; dir = filepath.Dir(dir) {
		t.expanded[dir] = true
	}
	t.rebuild()
	for i, r := range t.rows {
		if r.node.Path == rel {
			t.cursor = i
			return true
		}
	}
	return false
}

func (t *tree) move(delta int) {
	t.cursor = clamp(t.cursor+delta, 0, len(t.rows)-1)
}

func (t *tree) toggle() {
	n := t.selected()
	if n == nil || !n.IsDir {
		return
	}
	t.expanded[n.Path] = !t.expanded[n.Path]
	t.rebuild()
}

func (t *tree) collapseOrParent() {
	n := t.selected()
	if n == nil {
		return
	}
	if n.IsDir && t.expanded[n.Path] {
		t.toggle()
		return
	}
	if parent := filepath.Dir(n.Path); parent != "." {
		t.selectPath(parent)
	}
}

// targetDir is the folder new notes and folders go into: the selected folder,
// or the folder containing the selected note.
func (t *tree) targetDir() string {
	n := t.selected()
	switch {
	case n == nil:
		return ""
	case n.IsDir:
		return n.Path
	}
	if dir := filepath.Dir(n.Path); dir != "." {
		return dir
	}
	return ""
}

func (t *tree) renamePrefix(oldRel, newRel string) {
	next := map[string]bool{}
	for p, v := range t.expanded {
		if p == oldRel || strings.HasPrefix(p, oldRel+string(filepath.Separator)) {
			p = newRel + strings.TrimPrefix(p, oldRel)
		}
		next[p] = v
	}
	t.expanded = next
}

func (t *tree) view(width, height int, focused bool) string {
	if len(t.rows) == 0 {
		return styles.muted.Render("No notes yet.\nPress n to create one.")
	}
	if t.cursor < t.offset {
		t.offset = t.cursor
	}
	if t.cursor >= t.offset+height {
		t.offset = t.cursor - height + 1
	}
	t.offset = clamp(t.offset, 0, max(0, len(t.rows)-height))

	var b strings.Builder
	end := min(len(t.rows), t.offset+height)
	for i := t.offset; i < end; i++ {
		r := t.rows[i]
		icon := "  "
		if r.node.IsDir {
			icon = "▸ "
			if t.expanded[r.node.Path] {
				icon = "▾ "
			}
		}
		line := truncate(strings.Repeat("  ", r.depth)+icon+r.node.Name, width)
		style := styles.treeNote
		if r.node.IsDir {
			style = styles.treeDir
		}
		if i == t.cursor {
			style = styles.treeCursorBlur
			if focused {
				style = styles.treeCursor
			}
			line += strings.Repeat(" ", max(0, width-lipglossWidth(line)))
		}
		b.WriteString(style.Render(line))
		if i < end-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
