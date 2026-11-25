package ui

import (
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

const undoCoalesceWindow = time.Second

type snapshot struct {
	value string
	row   int
	col   int
}

// editor wraps textarea with an undo/redo history, which textarea lacks.
type editor struct {
	ta       textarea.Model
	undo     []snapshot
	redo     []snapshot
	lastPush time.Time
}

func newEditor() editor {
	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.MaxHeight = 0
	ta.MaxWidth = 0
	ta.FocusedStyle.CursorLine = ta.FocusedStyle.CursorLine.UnsetBackground()
	ta.FocusedStyle.Base = ta.FocusedStyle.Base.UnsetBorderStyle()
	ta.BlurredStyle.Base = ta.BlurredStyle.Base.UnsetBorderStyle()
	// ctrl+p / ctrl+f / ctrl+n / ctrl+b belong to the app, not cursor movement.
	ta.KeyMap.LinePrevious = key.NewBinding(key.WithKeys("up"))
	ta.KeyMap.LineNext = key.NewBinding(key.WithKeys("down"))
	ta.KeyMap.CharacterForward = key.NewBinding(key.WithKeys("right"))
	ta.KeyMap.CharacterBackward = key.NewBinding(key.WithKeys("left"))
	return editor{ta: ta}
}

func (e *editor) load(content string) {
	e.ta.SetValue(content)
	e.restoreCursor(0, 0)
	e.undo = nil
	e.redo = nil
	e.lastPush = time.Time{}
}

func (e *editor) value() string { return e.ta.Value() }

func (e *editor) setSize(w, h int) {
	e.ta.SetWidth(w)
	e.ta.SetHeight(h)
}

func (e *editor) focus() tea.Cmd { return e.ta.Focus() }
func (e *editor) blur()          { e.ta.Blur() }

func (e *editor) current() snapshot {
	info := e.ta.LineInfo()
	return snapshot{value: e.ta.Value(), row: e.ta.Line(), col: info.StartColumn + info.ColumnOffset}
}

// update forwards msg to the textarea and reports whether the text changed.
func (e *editor) update(msg tea.Msg) (bool, tea.Cmd) {
	before := e.current()
	var cmd tea.Cmd
	e.ta, cmd = e.ta.Update(msg)
	if e.ta.Value() == before.value {
		return false, cmd
	}
	now := time.Now()
	isBoundary := false
	if k, ok := msg.(tea.KeyMsg); ok && (k.Type == tea.KeyEnter || k.Paste) {
		isBoundary = true
	}
	if isBoundary || now.Sub(e.lastPush) > undoCoalesceWindow || len(e.undo) == 0 {
		e.undo = append(e.undo, before)
	}
	e.lastPush = now
	e.redo = nil
	return true, cmd
}

func (e *editor) undoChange() bool {
	if len(e.undo) == 0 {
		return false
	}
	e.redo = append(e.redo, e.current())
	s := e.undo[len(e.undo)-1]
	e.undo = e.undo[:len(e.undo)-1]
	e.apply(s)
	return true
}

func (e *editor) redoChange() bool {
	if len(e.redo) == 0 {
		return false
	}
	e.undo = append(e.undo, e.current())
	s := e.redo[len(e.redo)-1]
	e.redo = e.redo[:len(e.redo)-1]
	e.apply(s)
	return true
}

func (e *editor) apply(s snapshot) {
	e.ta.SetValue(s.value)
	e.restoreCursor(s.row, s.col)
	e.lastPush = time.Time{}
}

// restoreCursor works around textarea having no exported way to set the row:
// SetValue leaves the cursor at the end, so walk up to the target line.
func (e *editor) restoreCursor(row, col int) {
	for e.ta.Line() > row {
		prev := e.ta.Line()
		e.ta.CursorUp()
		if e.ta.Line() == prev && e.ta.LineInfo().RowOffset == 0 {
			break
		}
	}
	e.ta.SetCursor(col)
}

func (e *editor) cursorToEnd() {
	e.ta.SetValue(e.ta.Value())
}

func (e *editor) gotoLine(row int) {
	e.restoreCursor(e.ta.LineCount()-1, 0)
	e.restoreCursor(row, 0)
}
