package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/beladevo/margin/internal/store"
)

const (
	minTreeWidth = 22
	maxTreeWidth = 36
)

func (m Model) treeWidth() int {
	return clamp(m.width/4, minTreeWidth, maxTreeWidth)
}

func (m Model) bodyHeight() int {
	h := m.height - 1
	if m.conflict != conflictNone {
		h--
	}
	return max(h, 3)
}

func (m *Model) layout() {
	if m.width == 0 {
		return
	}
	noteInnerW := max(m.width-m.treeWidth()-4, 10)
	innerH := m.bodyHeight() - 2
	m.paneWidth = noteInnerW
	m.viewport.Width = noteInnerW
	m.viewport.Height = innerH
	m.editor.setSize(noteInnerW, innerH)
	if noteInnerW != m.rendererWidth {
		m.rendererWidth = noteInnerW
		r, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle(m.opts.GlamourStyle),
			glamour.WithWordWrap(noteInnerW-2),
		)
		if err == nil {
			m.renderer = r
		}
		m.renderNote(false)
	}
}

func (m *Model) renderNote(toTop bool) {
	if m.current == "" {
		m.viewport.SetContent(welcome())
		m.viewport.GotoTop()
		return
	}
	out := m.saved
	if m.renderer != nil {
		var colors palette
		if rendered, err := m.renderer.Render(colors.colorize(m.saved)); err == nil {
			out = colors.paint(strings.Trim(rendered, "\n"))
		}
	}
	offset := m.viewport.YOffset
	m.viewport.SetContent(out)
	if toTop {
		m.viewport.GotoTop()
	} else {
		m.viewport.SetYOffset(offset)
	}
}

func welcome() string {
	return strings.Join([]string{
		styles.title.Render("margin"),
		"",
		styles.muted.Render("Pick a note on the left, or:"),
		"",
		"  " + styles.key.Render("n") + "       new note",
		"  " + styles.key.Render("N") + "       new folder",
		"  " + styles.key.Render("ctrl+p") + "  find a note",
		"  " + styles.key.Render("?") + "       all keys",
	}, "\n")
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	bodyH := m.bodyHeight()
	treeW := m.treeWidth()
	noteW := m.width - treeW

	treeFocused := m.focus == focusTree && m.mode == modeBrowse && m.overlay == overlayNone
	left := pane(treeFocused, treeW, bodyH).Render(m.tree.view(treeW-4, bodyH-2, treeFocused))
	right := pane(!treeFocused, noteW, bodyH).Render(m.noteView(noteW-4, bodyH-2))

	rows := []string{lipgloss.JoinHorizontal(lipgloss.Top, left, right)}
	if m.conflict != conflictNone {
		rows = append(rows, m.bannerView())
	}
	rows = append(rows, m.statusView())
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func pane(focused bool, w, h int) lipgloss.Style {
	s := styles.pane
	if focused {
		s = styles.paneFocused
	}
	return s.Width(w - 2).Height(h - 2).MaxHeight(h)
}

func (m Model) noteView(w, h int) string {
	switch m.overlay {
	case overlayFinder:
		return m.finderView(w, h)
	case overlaySearch:
		return m.searchView(w, h)
	case overlayHelp:
		return clip(helpView(), h)
	}
	if m.mode == modeEdit {
		return m.editor.ta.View()
	}
	return m.viewport.View()
}

func (m Model) finderView(w, h int) string {
	lines := []string{styles.overlayTitle.Render("Find note"), m.input.View(), ""}
	if len(m.finderResults) == 0 {
		lines = append(lines, styles.muted.Render("No matching notes"))
	}
	lines = append(lines, listWindow(len(m.finderResults), m.listCursor, h-len(lines), func(i int) string {
		rel := m.finderResults[i]
		return truncate(strings.TrimSuffix(rel, filepath.Ext(rel)), w-2)
	})...)
	return clip(strings.Join(lines, "\n"), h)
}

func (m Model) searchView(w, h int) string {
	lines := []string{styles.overlayTitle.Render("Search notes"), m.input.View(), ""}
	switch {
	case strings.TrimSpace(m.input.Value()) == "":
		lines = append(lines, styles.muted.Render("Type to search every note"))
	case len(m.searchResults) == 0:
		lines = append(lines, styles.muted.Render("No matches"))
	}
	lines = append(lines, listWindow(len(m.searchResults), m.listCursor, h-len(lines), func(i int) string {
		r := m.searchResults[i]
		loc := store.Title(r.Path)
		if r.Line == 0 {
			return truncate(loc+"  (title)", w-2)
		}
		return truncate(fmt.Sprintf("%s:%d  %s", loc, r.Line, r.Text), w-2)
	})...)
	return clip(strings.Join(lines, "\n"), h)
}

func listWindow(n, cursor, height int, render func(int) string) []string {
	if n == 0 || height <= 0 {
		return nil
	}
	start := clamp(cursor-height/2, 0, max(0, n-height))
	end := min(n, start+height)
	out := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		line := render(i)
		if i == cursor {
			out = append(out, styles.selected.Render("› "+line))
		} else {
			out = append(out, "  "+line)
		}
	}
	return out
}

func clip(s string, h int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:max(h, 0)]
	}
	return strings.Join(lines, "\n")
}

func helpView() string {
	section := func(title string, rows [][2]string) string {
		var b strings.Builder
		b.WriteString(styles.title.Render(title) + "\n")
		for _, r := range rows {
			fmt.Fprintf(&b, "  %s %s\n", styles.key.Render(fmt.Sprintf("%-10s", r[0])), r[1])
		}
		return b.String()
	}
	return strings.Join([]string{
		section("Browse", [][2]string{
			{"↑/↓ j/k", "move"},
			{"←/→ h/l", "collapse / expand, focus note"},
			{"enter", "open folder / edit note"},
			{"tab", "switch pane"},
			{"e", "edit note"},
			{"n / N", "new note / new folder"},
			{"r", "rename"},
			{"d", "move to trash"},
			{"ctrl+p", "find note by name"},
			{"ctrl+f /", "search inside notes"},
			{"q", "quit"},
		}),
		section("Edit", [][2]string{
			{"esc", "save and stop editing"},
			{"ctrl+s", "save now"},
			{"ctrl+z/y", "undo / redo"},
		}),
		section("Conflict", [][2]string{
			{"ctrl+r", "reload from disk"},
			{"ctrl+k", "keep my version"},
		}),
		styles.muted.Render("Notes autosave while you type. Press any key to close."),
	}, "\n")
}

func (m Model) bannerView() string {
	msg := "File changed on disk"
	if m.conflict == conflictDeleted {
		msg = "File was deleted on disk"
	}
	msg += " · ctrl+r reload · ctrl+k keep mine"
	return styles.banner.Width(m.width).Render(truncate(msg, m.width-2))
}

func (m Model) statusView() string {
	switch m.overlay {
	case overlayPrompt:
		label := map[promptKind]string{
			promptNewNote: "New note",
			promptNewDir:  "New folder",
			promptRename:  "Rename",
		}[m.promptKind]
		if m.promptKind != promptRename && m.promptDir != "" {
			label += " in " + m.promptDir
		}
		line := styles.mode.Render(label) + " " + m.input.View()
		if m.inputErr != "" {
			line += "  " + styles.errorText.Render(m.inputErr)
		}
		return truncate(line, m.width)
	case overlayConfirm:
		q := fmt.Sprintf("Move %q to trash?", filepath.Base(m.actionTarget))
		return truncate(styles.banner.Render("Delete")+" "+q+" "+styles.key.Render("y")+"/"+styles.key.Render("n"), m.width)
	}

	badge := "VIEW"
	if m.mode == modeEdit {
		badge = "EDIT"
	}
	left := styles.mode.Render(badge) + " "
	if m.current != "" {
		left += styles.status.Render(m.current)
		if m.mode == modeEdit {
			if m.dirty {
				left += styles.modified.Render(" ● modified")
			} else {
				left += styles.saved.Render(" ✓ saved")
			}
		}
	} else {
		left += styles.muted.Render(m.store.Root)
	}

	var right string
	switch {
	case m.status != "" && m.statusErr:
		right = styles.errorText.Render(m.status)
	case m.status != "":
		right = styles.saved.Render(m.status)
	case m.mode == modeEdit:
		right = styles.muted.Render("esc done · ctrl+s save · ctrl+z undo")
	default:
		right = styles.muted.Render("e edit · n new · ctrl+p find · ? help")
	}
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return truncate(left+" "+right, m.width)
	}
	return left + strings.Repeat(" ", gap) + right
}
