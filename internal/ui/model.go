package ui

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/sahilm/fuzzy"

	"github.com/beladevo/margin/internal/store"
)

type Options struct {
	Store         *store.Store
	GlamourStyle  string
	AutosaveDelay time.Duration
	CheckInterval time.Duration
}

type mode int

const (
	modeBrowse mode = iota
	modeEdit
)

type focus int

const (
	focusTree focus = iota
	focusNote
)

type overlay int

const (
	overlayNone overlay = iota
	overlayPrompt
	overlayConfirm
	overlayFinder
	overlaySearch
	overlayHelp
)

type promptKind int

const (
	promptNewNote promptKind = iota
	promptNewDir
	promptRename
)

type conflict int

const (
	conflictNone conflict = iota
	conflictChanged
	conflictDeleted
)

type (
	autosaveMsg  struct{ gen int }
	checkDiskMsg struct{}
)

const searchLimit = 200

type Model struct {
	opts  Options
	store *store.Store

	width, height int
	mode          mode
	focus         focus
	overlay       overlay

	tree          tree
	viewport      viewport.Model
	editor        editor
	renderer      *glamour.TermRenderer
	rendererWidth int
	paneWidth     int

	current  string
	saved    string
	modTime  time.Time
	dirty    bool
	editGen  int
	conflict conflict

	status    string
	statusErr bool
	quitArmed bool
	mouseOn   bool

	input        textinput.Model
	inputErr     string
	promptKind   promptKind
	promptDir    string
	actionTarget string

	finderAll     []string
	finderResults []string
	searchResults []store.Match
	listCursor    int
}

func New(opts Options) (Model, error) {
	if opts.AutosaveDelay <= 0 {
		opts.AutosaveDelay = time.Second
	}
	if opts.CheckInterval <= 0 {
		opts.CheckInterval = time.Second
	}
	if opts.GlamourStyle == "" {
		opts.GlamourStyle = "dark"
	}
	vp := viewport.New(0, 0)
	vp.KeyMap.HalfPageDown = key.NewBinding(key.WithKeys("ctrl+d"))
	vp.KeyMap.HalfPageUp = key.NewBinding(key.WithKeys("ctrl+u"))
	vp.KeyMap.PageDown = key.NewBinding(key.WithKeys("pgdown", " "))
	vp.KeyMap.PageUp = key.NewBinding(key.WithKeys("pgup"))

	m := Model{
		opts:     opts,
		store:    opts.Store,
		tree:     newTree(),
		viewport: vp,
		editor:   newEditor(),
	}
	if err := m.refreshTree(); err != nil {
		return m, err
	}
	if n := m.tree.selected(); n != nil && !n.IsDir {
		m.openNote(n.Path)
	}
	return m, nil
}

func (m Model) Init() tea.Cmd {
	return m.checkDiskTick()
}

func (m Model) checkDiskTick() tea.Cmd {
	return tea.Tick(m.opts.CheckInterval, func(time.Time) tea.Msg { return checkDiskMsg{} })
}

func (m Model) autosaveTick() tea.Cmd {
	gen := m.editGen
	return tea.Tick(m.opts.AutosaveDelay, func(time.Time) tea.Msg { return autosaveMsg{gen: gen} })
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case autosaveMsg:
		if msg.gen == m.editGen {
			m.save()
		}
	case checkDiskMsg:
		m.checkDisk()
		cmd = m.checkDiskTick()
	case tea.KeyMsg:
		var model tea.Model
		model, cmd = m.handleKey(msg)
		m = model.(Model)
	case tea.MouseMsg:
		if m.mode == modeEdit && m.overlay == overlayNone {
			m.editor.mouse(msg, msg.X-m.treeWidth()-2, msg.Y-1)
		}
	default:
		if m.mode == modeEdit {
			if changed, _ := m.editor.update(msg); changed {
				cmd = m.markDirty()
			}
		}
	}
	m.layout()
	if wantMouse := m.mode == modeEdit; wantMouse != m.mouseOn {
		m.mouseOn = wantMouse
		toggle := tea.DisableMouse
		if wantMouse {
			toggle = tea.EnableMouseCellMotion
		}
		cmd = tea.Batch(cmd, toggle)
	}
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k != "ctrl+c" {
		m.quitArmed = false
	}
	m.status, m.statusErr = "", false

	if k == "ctrl+c" {
		if m.mode == modeEdit && m.overlay == overlayNone && m.editor.copySelection() {
			m.setStatus("Copied")
			return m, nil
		}
		return m.quit()
	}
	switch m.overlay {
	case overlayPrompt:
		return m.updatePrompt(msg)
	case overlayConfirm:
		return m.updateConfirm(msg)
	case overlayFinder:
		return m.updateFinder(msg)
	case overlaySearch:
		return m.updateSearch(msg)
	case overlayHelp:
		m.overlay = overlayNone
		return m, nil
	}
	if m.conflict != conflictNone {
		switch k {
		case "ctrl+r":
			m.resolveReload()
			return m, nil
		case "ctrl+k":
			m.resolveKeepMine()
			return m, nil
		}
	}
	if m.mode == modeEdit {
		return m.updateEdit(msg)
	}
	return m.updateBrowse(msg)
}

func (m Model) quit() (tea.Model, tea.Cmd) {
	if m.mode == modeEdit && m.dirty {
		m.save()
	}
	if m.dirty && !m.quitArmed {
		m.quitArmed = true
		m.setError("Unsaved changes could not be saved. Press ctrl+c again to quit anyway.")
		return m, nil
	}
	return m, tea.Quit
}

func (m Model) updateBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m.quit()
	case "?":
		m.overlay = overlayHelp
		return m, nil
	case "ctrl+p":
		return m, m.openFinder()
	case "ctrl+f", "/":
		return m, m.openSearch()
	case "tab":
		if m.focus == focusTree && m.current != "" {
			m.focus = focusNote
		} else {
			m.focus = focusTree
		}
		return m, nil
	case "n":
		return m, m.openPrompt(promptNewNote, "")
	case "N":
		return m, m.openPrompt(promptNewDir, "")
	case "r":
		if n := m.tree.selected(); n != nil {
			m.actionTarget = n.Path
			return m, m.openPrompt(promptRename, n.Name)
		}
		return m, nil
	case "d":
		if n := m.tree.selected(); n != nil {
			m.actionTarget = n.Path
			m.overlay = overlayConfirm
		}
		return m, nil
	case "e":
		return m, m.enterEdit()
	}

	if m.focus == focusNote {
		switch msg.String() {
		case "esc", "h", "left":
			m.focus = focusTree
			return m, nil
		case "enter":
			return m, m.enterEdit()
		case "g", "home":
			m.viewport.GotoTop()
			return m, nil
		case "G", "end":
			m.viewport.GotoBottom()
			return m, nil
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "up", "k":
		m.tree.move(-1)
		m.previewSelected()
	case "down", "j":
		m.tree.move(1)
		m.previewSelected()
	case "g", "home":
		m.tree.move(-len(m.tree.rows))
		m.previewSelected()
	case "G", "end":
		m.tree.move(len(m.tree.rows))
		m.previewSelected()
	case "pgup":
		m.tree.move(-10)
		m.previewSelected()
	case "pgdown":
		m.tree.move(10)
		m.previewSelected()
	case "h", "left":
		m.tree.collapseOrParent()
		m.previewSelected()
	case "l", "right":
		if n := m.tree.selected(); n != nil {
			if n.IsDir {
				if !m.tree.expanded[n.Path] {
					m.tree.toggle()
				}
			} else {
				m.focus = focusNote
			}
		}
	case "enter", " ":
		if n := m.tree.selected(); n != nil {
			if n.IsDir {
				m.tree.toggle()
			} else {
				return m, m.enterEdit()
			}
		}
	}
	return m, nil
}

func (m *Model) previewSelected() {
	if n := m.tree.selected(); n != nil && !n.IsDir && n.Path != m.current {
		m.openNote(n.Path)
	}
}

func (m Model) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.editor.hasSelection() {
			m.editor.selecting = false
			return m, nil
		}
		m.save()
		if !m.dirty {
			m.exitEdit()
		}
		return m, nil
	case "ctrl+s":
		if !m.dirty {
			m.setStatus("No changes")
		}
		m.save()
		return m, nil
	case "ctrl+z":
		if m.editor.undoChange() {
			return m, m.markDirty()
		}
		return m, nil
	case "ctrl+y":
		if m.editor.redoChange() {
			return m, m.markDirty()
		}
		return m, nil
	case "ctrl+p", "ctrl+f":
		m.save()
		if m.dirty {
			return m, nil
		}
		m.exitEdit()
		if msg.String() == "ctrl+p" {
			return m, m.openFinder()
		}
		return m, m.openSearch()
	}
	changed, cmd := m.editor.update(msg)
	if changed {
		return m, tea.Batch(cmd, m.markDirty())
	}
	return m, cmd
}

func (m *Model) markDirty() tea.Cmd {
	m.dirty = m.editor.value() != m.saved
	m.editGen++
	if !m.dirty {
		return nil
	}
	return m.autosaveTick()
}

func (m *Model) enterEdit() tea.Cmd {
	if m.current == "" {
		return nil
	}
	m.mode = modeEdit
	m.focus = focusNote
	m.editor.load(m.saved)
	m.dirty = false
	return m.editor.focus()
}

func (m *Model) exitEdit() {
	m.mode = modeBrowse
	m.editor.blur()
	m.renderNote(false)
	_ = m.refreshTree()
}

func (m *Model) openNote(rel string) {
	content, mt, err := m.store.Read(rel)
	if err != nil {
		m.setError(err.Error())
		return
	}
	m.current = rel
	m.saved = content
	m.modTime = mt
	m.dirty = false
	m.conflict = conflictNone
	m.renderNote(true)
}

func (m *Model) closeNote() {
	if m.mode == modeEdit {
		m.mode = modeBrowse
		m.editor.blur()
	}
	m.current, m.saved, m.modTime = "", "", time.Time{}
	m.dirty = false
	m.conflict = conflictNone
	m.focus = focusTree
	m.renderNote(true)
}

// save writes the editor contents, refusing to clobber a file that changed on
// disk since it was read; that case raises the conflict banner instead.
func (m *Model) save() {
	if m.mode != modeEdit || m.current == "" || !m.dirty {
		return
	}
	if m.conflict != conflictNone {
		m.setError("Not saved: resolve the conflict first (ctrl+r reload · ctrl+k keep mine)")
		return
	}
	disk, err := m.store.ModTime(m.current)
	if errors.Is(err, fs.ErrNotExist) {
		m.conflict = conflictDeleted
		return
	}
	if err != nil {
		m.setError(err.Error())
		return
	}
	if !disk.Equal(m.modTime) {
		m.conflict = conflictChanged
		return
	}
	m.write()
}

func (m *Model) write() {
	content := m.editor.value()
	mt, err := m.store.Write(m.current, content)
	if err != nil {
		m.setError("Save failed: " + err.Error())
		return
	}
	m.saved = content
	m.modTime = mt
	m.dirty = false
	m.setStatus("Saved")
}

func (m *Model) checkDisk() {
	if m.current == "" {
		if m.mode == modeBrowse && m.overlay == overlayNone {
			_ = m.refreshTree()
		}
		return
	}
	mt, err := m.store.ModTime(m.current)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if m.mode == modeEdit && m.dirty {
			m.conflict = conflictDeleted
			return
		}
		name := store.Title(m.current)
		m.closeNote()
		m.setStatus(fmt.Sprintf("%q was removed on disk", name))
	case err != nil:
		return
	case !mt.Equal(m.modTime):
		if m.mode == modeEdit && m.dirty {
			m.conflict = conflictChanged
			return
		}
		m.reloadFromDisk()
		m.setStatus("Reloaded: changed on disk")
	}
	if m.mode == modeBrowse && m.overlay == overlayNone {
		_ = m.refreshTree()
	}
}

func (m *Model) reloadFromDisk() {
	content, mt, err := m.store.Read(m.current)
	if err != nil {
		m.setError(err.Error())
		return
	}
	m.saved, m.modTime = content, mt
	m.dirty = false
	m.conflict = conflictNone
	if m.mode == modeEdit {
		row := m.editor.cur.row
		m.editor.load(content)
		m.editor.gotoLine(row)
	}
	m.renderNote(false)
}

func (m *Model) resolveReload() {
	if m.conflict == conflictDeleted {
		m.closeNote()
		_ = m.refreshTree()
		return
	}
	m.reloadFromDisk()
	m.setStatus("Reloaded from disk")
}

func (m *Model) resolveKeepMine() {
	m.conflict = conflictNone
	if path, err := m.store.Abs(m.current); err == nil {
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
	}
	m.write()
	_ = m.refreshTree()
}

func (m *Model) refreshTree() error {
	root, err := m.store.Tree()
	if err != nil {
		return err
	}
	m.tree.setRoot(root)
	return nil
}

func (m *Model) openPrompt(kind promptKind, initial string) tea.Cmd {
	m.promptKind = kind
	m.promptDir = m.tree.targetDir()
	m.inputErr = ""
	ti := textinput.New()
	ti.Prompt = ""
	ti.SetValue(initial)
	ti.CursorEnd()
	m.input = ti
	m.overlay = overlayPrompt
	return m.input.Focus()
}

func (m Model) updatePrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.overlay = overlayNone
		return m, nil
	case "enter":
		return m.submitPrompt()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.inputErr = ""
	return m, cmd
}

func (m Model) submitPrompt() (tea.Model, tea.Cmd) {
	value := m.input.Value()
	switch m.promptKind {
	case promptNewNote:
		rel, err := m.store.CreateNote(m.promptDir, value)
		if err != nil {
			m.inputErr = err.Error()
			return m, nil
		}
		m.overlay = overlayNone
		_ = m.refreshTree()
		m.tree.selectPath(rel)
		m.openNote(rel)
		cmd := m.enterEdit()
		m.editor.cursorToEnd()
		return m, cmd
	case promptNewDir:
		rel, err := m.store.CreateDir(m.promptDir, value)
		if err != nil {
			m.inputErr = err.Error()
			return m, nil
		}
		m.overlay = overlayNone
		m.tree.expanded[rel] = true
		_ = m.refreshTree()
		m.tree.selectPath(rel)
		m.setStatus(fmt.Sprintf("Created folder %q", filepath.Base(rel)))
	case promptRename:
		newRel, err := m.store.Rename(m.actionTarget, value)
		if err != nil {
			m.inputErr = err.Error()
			return m, nil
		}
		m.overlay = overlayNone
		m.tree.renamePrefix(m.actionTarget, newRel)
		if moved, ok := rebase(m.current, m.actionTarget, newRel); ok {
			m.current = moved
		}
		_ = m.refreshTree()
		m.tree.selectPath(newRel)
		m.setStatus(fmt.Sprintf("Renamed to %q", store.Title(newRel)))
	}
	return m, nil
}

func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.overlay = overlayNone
		trashRel, err := m.store.Trash(m.actionTarget)
		if err != nil {
			m.setError(err.Error())
			return m, nil
		}
		if _, ok := rebase(m.current, m.actionTarget, ""); ok {
			m.closeNote()
		}
		_ = m.refreshTree()
		m.setStatus("Moved to " + trashRel)
	case "n", "N", "esc", "q":
		m.overlay = overlayNone
	}
	return m, nil
}

// rebase reports whether rel is target or inside it, and returns rel moved
// under newTarget.
func rebase(rel, target, newTarget string) (string, bool) {
	if rel == "" {
		return "", false
	}
	if rel == target {
		return newTarget, true
	}
	if prefix := target + string(filepath.Separator); strings.HasPrefix(rel, prefix) {
		return filepath.Join(newTarget, strings.TrimPrefix(rel, prefix)), true
	}
	return "", false
}

func (m *Model) openFinder() tea.Cmd {
	notes, err := m.store.Notes()
	if err != nil {
		m.setError(err.Error())
		return nil
	}
	sort.Slice(notes, func(i, j int) bool { return strings.ToLower(notes[i]) < strings.ToLower(notes[j]) })
	m.finderAll = notes
	m.finderResults = notes
	m.listCursor = 0
	m.input = textinput.New()
	m.input.Prompt = "› "
	m.input.Placeholder = "Find a note by name"
	m.overlay = overlayFinder
	return m.input.Focus()
}

func (m Model) updateFinder(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.overlay = overlayNone
		return m, nil
	case "up", "ctrl+p", "ctrl+k":
		m.listCursor = clamp(m.listCursor-1, 0, len(m.finderResults)-1)
		return m, nil
	case "down", "ctrl+n", "ctrl+j":
		m.listCursor = clamp(m.listCursor+1, 0, len(m.finderResults)-1)
		return m, nil
	case "enter":
		if len(m.finderResults) > 0 {
			m.overlay = overlayNone
			m.jumpTo(m.finderResults[m.listCursor], 0)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.finderResults = filterNotes(m.input.Value(), m.finderAll)
	m.listCursor = 0
	return m, cmd
}

func filterNotes(query string, notes []string) []string {
	if strings.TrimSpace(query) == "" {
		return notes
	}
	names := make([]string, len(notes))
	for i, n := range notes {
		names[i] = strings.TrimSuffix(n, filepath.Ext(n))
	}
	matches := fuzzy.Find(query, names)
	out := make([]string, len(matches))
	for i, match := range matches {
		out[i] = notes[match.Index]
	}
	return out
}

func (m *Model) openSearch() tea.Cmd {
	m.searchResults = nil
	m.listCursor = 0
	m.input = textinput.New()
	m.input.Prompt = "› "
	m.input.Placeholder = "Search inside notes"
	m.overlay = overlaySearch
	return m.input.Focus()
}

func (m Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.overlay = overlayNone
		return m, nil
	case "up", "ctrl+p", "ctrl+k":
		m.listCursor = clamp(m.listCursor-1, 0, len(m.searchResults)-1)
		return m, nil
	case "down", "ctrl+n", "ctrl+j":
		m.listCursor = clamp(m.listCursor+1, 0, len(m.searchResults)-1)
		return m, nil
	case "enter":
		if len(m.searchResults) > 0 {
			match := m.searchResults[m.listCursor]
			m.overlay = overlayNone
			m.jumpTo(match.Path, match.Line)
		}
		return m, nil
	}
	before := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != before {
		results, err := m.store.Search(m.input.Value(), searchLimit)
		if err != nil {
			m.setError(err.Error())
		}
		m.searchResults = results
		m.listCursor = 0
	}
	return m, cmd
}

func (m *Model) jumpTo(rel string, line int) {
	m.tree.selectPath(rel)
	m.openNote(rel)
	m.focus = focusTree
	if line <= 0 {
		return
	}
	// Rendered markdown doesn't map 1:1 to source lines, so scroll proportionally.
	total := strings.Count(m.saved, "\n") + 1
	m.layout()
	offset := int(float64(line-1) / float64(total) * float64(m.viewport.TotalLineCount()))
	m.viewport.SetYOffset(offset)
	m.setStatus(fmt.Sprintf("Line %d", line))
}

func (m *Model) setStatus(s string) { m.status, m.statusErr = s, false }
func (m *Model) setError(s string)  { m.status, m.statusErr = s, true }
