package ui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
)

const (
	undoCoalesceWindow = time.Second
	doubleClickWindow  = 400 * time.Millisecond
	tabWidth           = 4
	indentUnit         = "  "
	wheelStep          = 3
)

// Bubble Tea v1 has no shift+enter key; terminals that report it send one of
// these sequences, which arrive as an unexported unknown-CSI message.
var shiftEnterSeqs = map[string]bool{
	fmt.Sprintf("?CSI%+v?", []byte("13;2u")):    true,
	fmt.Sprintf("?CSI%+v?", []byte("27;2;13~")): true,
}

var (
	listPrefixRe  = regexp.MustCompile(`^(\s*)(?:([-*+])|(\d+)([.)]))(\s+)(\[[ xX]\]\s+)?`)
	quotePrefixRe = regexp.MustCompile(`^\s*>+\s?`)
)

type pos struct{ row, col int }

func (p pos) less(q pos) bool { return p.row < q.row || (p.row == q.row && p.col < q.col) }

type snapshot struct {
	value string
	cur   pos
}

type vrow struct{ line, start, end int }

type editor struct {
	lines     [][]rune
	cur       pos
	anchor    pos
	selecting bool
	goalX     int
	width     int
	height    int
	top       int
	focused   bool
	dragging  bool
	lastClick time.Time
	clickPos  pos
	undo      []snapshot
	redo      []snapshot
	lastPush  time.Time
	yank      string
	readClip  func() (string, error)
	writeClip func(string) error
}

func newEditor() editor {
	return editor{
		lines:     [][]rune{{}},
		goalX:     -1,
		width:     2,
		height:    1,
		readClip:  clipboard.ReadAll,
		writeClip: clipboard.WriteAll,
	}
}

func (e *editor) load(content string) {
	e.lines = splitLines(content)
	e.cur, e.anchor, e.selecting = pos{}, pos{}, false
	e.top, e.goalX = 0, -1
	e.undo, e.redo = nil, nil
	e.lastPush = time.Time{}
}

func splitLines(s string) [][]rune {
	parts := strings.Split(s, "\n")
	lines := make([][]rune, len(parts))
	for i, p := range parts {
		lines[i] = []rune(p)
	}
	return lines
}

func (e *editor) value() string {
	parts := make([]string, len(e.lines))
	for i, l := range e.lines {
		parts[i] = string(l)
	}
	return strings.Join(parts, "\n")
}

func (e *editor) setSize(w, h int) {
	e.width, e.height = max(w, 2), max(h, 1)
	e.ensureVisible()
}

func (e *editor) focus() tea.Cmd { e.focused = true; return nil }
func (e *editor) blur()          { e.focused, e.dragging = false, false }

func (e *editor) current() snapshot { return snapshot{value: e.value(), cur: e.cur} }

func (e *editor) hasSelection() bool { return e.selecting && e.anchor != e.cur }

func (e *editor) selection() (pos, pos) {
	if e.anchor.less(e.cur) {
		return e.anchor, e.cur
	}
	return e.cur, e.anchor
}

// update applies msg and reports whether the text changed.
func (e *editor) update(msg tea.Msg) (bool, tea.Cmd) {
	if !e.focused {
		return false, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		changed := e.handleKey(msg)
		e.ensureVisible()
		return changed, nil
	case fmt.Stringer:
		if shiftEnterSeqs[msg.String()] {
			changed := e.edit(true, e.newline)
			e.ensureVisible()
			return changed, nil
		}
	}
	return false, nil
}

func (e *editor) handleKey(k tea.KeyMsg) bool {
	if k.Paste {
		return e.edit(true, func() { e.insert(string(k.Runes)) })
	}
	key := k.String()
	switch key {
	case "up", "down", "shift+up", "shift+down", "pgup", "pgdown":
	default:
		e.goalX = -1
	}
	switch key {
	case "left":
		e.collapseOr(true, e.left)
	case "right":
		e.collapseOr(false, e.right)
	case "shift+left":
		e.move(true, e.left)
	case "shift+right":
		e.move(true, e.right)
	case "up":
		e.move(false, func() { e.vertical(-1) })
	case "down":
		e.move(false, func() { e.vertical(1) })
	case "shift+up":
		e.move(true, func() { e.vertical(-1) })
	case "shift+down":
		e.move(true, func() { e.vertical(1) })
	case "pgup":
		e.move(false, func() { e.vertical(-e.height) })
	case "pgdown":
		e.move(false, func() { e.vertical(e.height) })
	case "alt+left", "alt+b", "ctrl+left":
		e.move(false, e.wordLeft)
	case "alt+right", "alt+f", "ctrl+right":
		e.move(false, e.wordRight)
	case "alt+shift+left", "ctrl+shift+left":
		e.move(true, e.wordLeft)
	case "alt+shift+right", "ctrl+shift+right":
		e.move(true, e.wordRight)
	case "home":
		e.move(false, e.lineStart)
	case "end", "ctrl+e":
		e.move(false, e.lineEnd)
	case "shift+home":
		e.move(true, e.lineStart)
	case "shift+end":
		e.move(true, e.lineEnd)
	case "ctrl+home":
		e.move(false, e.docStart)
	case "ctrl+end":
		e.move(false, e.docEnd)
	case "ctrl+shift+home":
		e.move(true, e.docStart)
	case "ctrl+shift+end":
		e.move(true, e.docEnd)
	case "ctrl+a":
		e.selectAll()
	case "ctrl+c":
		e.copySelection()
	case "ctrl+x":
		return e.edit(true, e.cut)
	case "ctrl+v":
		return e.edit(true, e.paste)
	case "alt+up":
		return e.edit(true, func() { e.moveLines(-1) })
	case "alt+down":
		return e.edit(true, func() { e.moveLines(1) })
	case "enter", "alt+enter", "ctrl+j":
		return e.edit(true, e.newline)
	case "backspace", "ctrl+h":
		return e.edit(e.hasSelection(), e.backspace)
	case "delete", "ctrl+d":
		return e.edit(e.hasSelection(), e.deleteForward)
	case "alt+backspace", "ctrl+w":
		return e.edit(false, func() { e.deleteBy(e.wordLeft) })
	case "alt+delete", "alt+d":
		return e.edit(false, func() { e.deleteBy(e.wordRight) })
	case "ctrl+u":
		return e.edit(false, func() { e.deleteBy(e.lineStart) })
	case "ctrl+k":
		return e.edit(false, e.killLine)
	case "tab":
		return e.edit(true, e.indent)
	case "shift+tab":
		return e.edit(true, e.outdent)
	default:
		if (k.Type == tea.KeyRunes || k.Type == tea.KeySpace) && !k.Alt {
			return e.edit(e.hasSelection(), func() { e.insert(string(k.Runes)) })
		}
	}
	return false
}

func (e *editor) edit(boundary bool, f func()) bool {
	before := e.current()
	f()
	if e.value() == before.value {
		return false
	}
	now := time.Now()
	if boundary || now.Sub(e.lastPush) > undoCoalesceWindow || len(e.undo) == 0 {
		e.undo = append(e.undo, before)
	}
	e.lastPush = now
	e.redo = nil
	return true
}

func (e *editor) move(extend bool, f func()) {
	if !extend {
		e.selecting = false
	} else if !e.selecting {
		e.anchor, e.selecting = e.cur, true
	}
	f()
}

func (e *editor) collapseOr(toStart bool, f func()) {
	if !e.hasSelection() {
		e.move(false, f)
		return
	}
	a, b := e.selection()
	e.cur = b
	if toStart {
		e.cur = a
	}
	e.selecting = false
}

func (e *editor) left() {
	if e.cur.col > 0 {
		e.cur.col--
	} else if e.cur.row > 0 {
		e.cur.row--
		e.cur.col = len(e.lines[e.cur.row])
	}
}

func (e *editor) right() {
	if e.cur.col < len(e.lines[e.cur.row]) {
		e.cur.col++
	} else if e.cur.row < len(e.lines)-1 {
		e.cur.row++
		e.cur.col = 0
	}
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }

func (e *editor) wordLeft() {
	if e.cur.col == 0 {
		e.left()
		return
	}
	line, c := e.lines[e.cur.row], e.cur.col
	for c > 0 && !isWordRune(line[c-1]) {
		c--
	}
	for c > 0 && isWordRune(line[c-1]) {
		c--
	}
	e.cur.col = c
}

func (e *editor) wordRight() {
	line, c := e.lines[e.cur.row], e.cur.col
	if c == len(line) {
		e.right()
		return
	}
	for c < len(line) && !isWordRune(line[c]) {
		c++
	}
	for c < len(line) && isWordRune(line[c]) {
		c++
	}
	e.cur.col = c
}

func (e *editor) lineStart() { e.cur.col = 0 }
func (e *editor) lineEnd()   { e.cur.col = len(e.lines[e.cur.row]) }
func (e *editor) docStart()  { e.cur = pos{} }
func (e *editor) docEnd() {
	e.cur.row = len(e.lines) - 1
	e.lineEnd()
}

func (e *editor) vertical(delta int) {
	rows := e.vrows()
	i := vrowIndex(rows, e.cur)
	if e.goalX < 0 {
		e.goalX = e.xIn(rows[i], e.cur.col)
	}
	j := clamp(i+delta, 0, len(rows)-1)
	switch {
	case j == i && delta < 0:
		e.cur.col = 0
	case j == i && delta > 0:
		e.lineEnd()
	default:
		e.cur = pos{rows[j].line, e.colAt(rows, j, e.goalX)}
	}
}

func (e *editor) selectAll() {
	e.anchor, e.selecting = pos{}, true
	e.docEnd()
}

func (e *editor) cursorToEnd() {
	e.selecting = false
	e.docEnd()
	e.ensureVisible()
}

func (e *editor) gotoLine(row int) {
	e.selecting = false
	e.cur = pos{clamp(row, 0, len(e.lines)-1), 0}
	e.ensureVisible()
}

func (e *editor) text(a, b pos) string {
	if a.row == b.row {
		return string(e.lines[a.row][a.col:b.col])
	}
	var sb strings.Builder
	sb.WriteString(string(e.lines[a.row][a.col:]))
	for r := a.row + 1; r < b.row; r++ {
		sb.WriteByte('\n')
		sb.WriteString(string(e.lines[r]))
	}
	sb.WriteByte('\n')
	sb.WriteString(string(e.lines[b.row][:b.col]))
	return sb.String()
}

func splice(lines [][]rune, from, to int, repl [][]rune) [][]rune {
	out := make([][]rune, 0, len(lines)-(to-from)+len(repl))
	out = append(out, lines[:from]...)
	out = append(out, repl...)
	return append(out, lines[to:]...)
}

func (e *editor) deleteRange(a, b pos) {
	joined := append(append([]rune{}, e.lines[a.row][:a.col]...), e.lines[b.row][b.col:]...)
	e.lines = splice(e.lines, a.row, b.row+1, [][]rune{joined})
	e.cur = a
}

func (e *editor) deleteSelection() bool {
	if !e.hasSelection() {
		e.selecting = false
		return false
	}
	e.deleteRange(e.selection())
	e.selecting = false
	return true
}

func (e *editor) insert(s string) {
	e.deleteSelection()
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	parts := splitLines(s)
	line := e.lines[e.cur.row]
	tail := append([]rune{}, line[e.cur.col:]...)
	parts[0] = append(append([]rune{}, line[:e.cur.col]...), parts[0]...)
	last := len(parts) - 1
	col := len(parts[last])
	parts[last] = append(parts[last], tail...)
	e.lines = splice(e.lines, e.cur.row, e.cur.row+1, parts)
	e.cur = pos{e.cur.row + last, col}
}

func (e *editor) backspace() {
	if e.deleteSelection() {
		return
	}
	end := e.cur
	e.left()
	e.deleteRange(e.cur, end)
}

func (e *editor) deleteForward() {
	if e.deleteSelection() {
		return
	}
	start := e.cur
	e.right()
	e.deleteRange(start, e.cur)
}

func (e *editor) deleteBy(motion func()) {
	if e.deleteSelection() {
		return
	}
	start := e.cur
	motion()
	a, b := start, e.cur
	if b.less(a) {
		a, b = b, a
	}
	e.deleteRange(a, b)
}

func (e *editor) killLine() {
	if e.deleteSelection() {
		return
	}
	if e.cur.col == len(e.lines[e.cur.row]) {
		e.deleteForward()
		return
	}
	e.deleteRange(e.cur, pos{e.cur.row, len(e.lines[e.cur.row])})
}

// newline continues lists and quotes, and ends them on an empty item.
func (e *editor) newline() {
	e.deleteSelection()
	line := string(e.lines[e.cur.row])
	atEnd := e.cur.col == len(e.lines[e.cur.row])
	if g := listPrefixRe.FindStringSubmatch(line); g != nil && e.cur.col >= len([]rune(g[0])) {
		if atEnd && strings.TrimSpace(line[len(g[0]):]) == "" {
			e.lines[e.cur.row] = []rune{}
			e.cur.col = 0
			return
		}
		marker := g[2]
		if marker == "" {
			n, _ := strconv.Atoi(g[3])
			marker = strconv.Itoa(n+1) + g[4]
		}
		next := g[1] + marker + " "
		if g[6] != "" {
			next += "[ ] "
		}
		e.insert("\n" + next)
		return
	}
	if p := quotePrefixRe.FindString(line); p != "" && e.cur.col >= len([]rune(p)) {
		if atEnd && strings.TrimSpace(line[len(p):]) == "" {
			e.lines[e.cur.row] = []rune{}
			e.cur.col = 0
			return
		}
		e.insert("\n" + p)
		return
	}
	e.insert("\n")
}

func (e *editor) selectedRows() (int, int) {
	if !e.hasSelection() {
		return e.cur.row, e.cur.row
	}
	a, b := e.selection()
	if b.col == 0 && b.row > a.row {
		return a.row, b.row - 1
	}
	return a.row, b.row
}

func (e *editor) indent() {
	r0, r1 := e.selectedRows()
	if !e.hasSelection() && !listPrefixRe.MatchString(string(e.lines[r0])) {
		e.insert(indentUnit)
		return
	}
	unit := []rune(indentUnit)
	for r := r0; r <= r1; r++ {
		e.lines[r] = append(append([]rune{}, unit...), e.lines[r]...)
	}
	e.shiftCols(r0, r1, func(int) int { return len(unit) })
}

func (e *editor) outdent() {
	r0, r1 := e.selectedRows()
	removed := map[int]int{}
	for r := r0; r <= r1; r++ {
		line := e.lines[r]
		n := 0
		if len(line) > 0 && line[0] == '\t' {
			n = 1
		}
		for n < len(indentUnit) && n < len(line) && line[n] == ' ' {
			n++
		}
		e.lines[r] = line[n:]
		removed[r] = n
	}
	e.shiftCols(r0, r1, func(r int) int { return -removed[r] })
}

func (e *editor) shiftCols(r0, r1 int, delta func(row int) int) {
	for _, p := range []*pos{&e.cur, &e.anchor} {
		if p.row >= r0 && p.row <= r1 {
			p.col = clamp(p.col+delta(p.row), 0, len(e.lines[p.row]))
		}
	}
}

func (e *editor) moveLines(delta int) {
	r0, r1 := e.selectedRows()
	if (delta < 0 && r0 == 0) || (delta > 0 && r1 == len(e.lines)-1) {
		return
	}
	if delta < 0 {
		moved := e.lines[r0-1]
		copy(e.lines[r0-1:r1], e.lines[r0:r1+1])
		e.lines[r1] = moved
	} else {
		moved := e.lines[r1+1]
		copy(e.lines[r0+1:r1+2], e.lines[r0:r1+1])
		e.lines[r0] = moved
	}
	e.cur.row += delta
	e.anchor.row += delta
}

func (e *editor) copySelection() bool {
	if !e.hasSelection() {
		return false
	}
	e.toClipboard(e.text(e.selection()))
	return true
}

func (e *editor) toClipboard(s string) {
	e.yank = s
	if e.writeClip != nil {
		_ = e.writeClip(s)
	}
}

func (e *editor) cut() {
	if e.hasSelection() {
		e.toClipboard(e.text(e.selection()))
		e.deleteSelection()
		return
	}
	row := e.cur.row
	e.toClipboard(string(e.lines[row]) + "\n")
	if len(e.lines) == 1 {
		e.lines[0] = []rune{}
	} else {
		e.lines = splice(e.lines, row, row+1, nil)
	}
	e.cur = pos{min(row, len(e.lines)-1), 0}
}

func (e *editor) paste() {
	s := e.yank
	if e.readClip != nil {
		if clip, err := e.readClip(); err == nil && clip != "" {
			s = clip
		}
	}
	if s != "" {
		e.insert(s)
	}
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
	e.lines = splitLines(s.value)
	e.cur = pos{clamp(s.cur.row, 0, len(e.lines)-1), 0}
	e.cur.col = clamp(s.cur.col, 0, len(e.lines[e.cur.row]))
	e.selecting = false
	e.lastPush = time.Time{}
	e.ensureVisible()
}

func (e *editor) mouse(msg tea.MouseMsg, x, y int) {
	switch {
	case msg.Button == tea.MouseButtonWheelUp:
		e.top = max(0, e.top-wheelStep)
	case msg.Button == tea.MouseButtonWheelDown:
		e.top = clamp(e.top+wheelStep, 0, max(0, len(e.vrows())-e.height))
	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		p := e.posAt(x, y)
		e.goalX = -1
		now := time.Now()
		if p == e.clickPos && now.Sub(e.lastClick) < doubleClickWindow {
			e.selectWord(p)
			e.lastClick = time.Time{}
			return
		}
		e.lastClick, e.clickPos = now, p
		if !msg.Shift || !e.selecting {
			e.anchor = p
			if msg.Shift {
				e.anchor = e.cur
			}
		}
		e.cur, e.selecting, e.dragging = p, true, true
	case msg.Action == tea.MouseActionMotion && e.dragging:
		if y < 0 {
			e.top = max(0, e.top-1)
		} else if y >= e.height {
			e.top = clamp(e.top+1, 0, max(0, len(e.vrows())-e.height))
		}
		e.cur = e.posAt(x, clamp(y, 0, e.height-1))
	case msg.Action == tea.MouseActionRelease:
		e.dragging = false
	}
}

func (e *editor) selectWord(p pos) {
	line := e.lines[p.row]
	a, b := p.col, p.col
	for a > 0 && isWordRune(line[a-1]) {
		a--
	}
	for b < len(line) && isWordRune(line[b]) {
		b++
	}
	e.anchor, e.cur, e.selecting, e.dragging = pos{p.row, a}, pos{p.row, b}, true, false
}

func (e *editor) posAt(x, y int) pos {
	rows := e.vrows()
	j := clamp(e.top+y, 0, len(rows)-1)
	return pos{rows[j].line, e.colAt(rows, j, max(x, 0))}
}

func cellWidth(r rune) int {
	if r == '\t' {
		return tabWidth
	}
	if !unicode.IsPrint(r) {
		return 1
	}
	return runewidth.RuneWidth(r)
}

func (e *editor) wrapWidth() int { return max(e.width-1, 1) }

// wrapStarts returns where each visual row of line begins, breaking after the
// last space that fits and mid-word only when a word is wider than the pane.
func wrapStarts(line []rune, width int) []int {
	starts := []int{0}
	start, w, lastSpace := 0, 0, -1
	for i := 0; i < len(line); i++ {
		cw := cellWidth(line[i])
		for w+cw > width && i > start {
			brk := i
			if lastSpace >= start {
				brk = lastSpace + 1
			}
			starts = append(starts, brk)
			start, w, lastSpace = brk, 0, -1
			for k := start; k < i; k++ {
				w += cellWidth(line[k])
			}
		}
		w += cw
		if line[i] == ' ' {
			lastSpace = i
		}
	}
	return starts
}

func (e *editor) vrows() []vrow {
	rows := make([]vrow, 0, len(e.lines))
	width := e.wrapWidth()
	for i, l := range e.lines {
		starts := wrapStarts(l, width)
		for j, s := range starts {
			end := len(l)
			if j+1 < len(starts) {
				end = starts[j+1]
			}
			rows = append(rows, vrow{i, s, end})
		}
	}
	return rows
}

func vrowIndex(rows []vrow, p pos) int {
	idx := 0
	for i, r := range rows {
		if r.line > p.row {
			break
		}
		if r.line == p.row && r.start <= p.col {
			idx = i
		}
	}
	return idx
}

func (e *editor) xIn(r vrow, col int) int {
	w := 0
	for _, c := range e.lines[r.line][r.start:col] {
		w += cellWidth(c)
	}
	return w
}

func (e *editor) colAt(rows []vrow, j, x int) int {
	r := rows[j]
	line := e.lines[r.line]
	c, w := r.start, 0
	for c < r.end {
		cw := cellWidth(line[c])
		if w+cw > x {
			break
		}
		w += cw
		c++
	}
	if c == r.end && c > r.start && j+1 < len(rows) && rows[j+1].line == r.line {
		c--
	}
	return c
}

func (e *editor) ensureVisible() {
	rows := e.vrows()
	i := vrowIndex(rows, e.cur)
	if i < e.top {
		e.top = i
	}
	if i >= e.top+e.height {
		e.top = i - e.height + 1
	}
	e.top = clamp(e.top, 0, max(0, len(rows)-1))
}
