package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type fakeCSI string

func (f fakeCSI) String() string { return string(f) }

func testEditor(t *testing.T, content string) (*editor, *string) {
	t.Helper()
	clip := new(string)
	ed := newEditor()
	ed.readClip = func() (string, error) { return *clip, nil }
	ed.writeClip = func(s string) error { *clip = s; return nil }
	ed.setSize(40, 10)
	ed.load(content)
	ed.focus()
	return &ed, clip
}

func press(ed *editor, keys ...string) {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "alt+enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter, Alt: true}
		case "backspace":
			msg = tea.KeyMsg{Type: tea.KeyBackspace}
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		case "shift+tab":
			msg = tea.KeyMsg{Type: tea.KeyShiftTab}
		case "left":
			msg = tea.KeyMsg{Type: tea.KeyLeft}
		case "right":
			msg = tea.KeyMsg{Type: tea.KeyRight}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "up":
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case "end":
			msg = tea.KeyMsg{Type: tea.KeyEnd}
		case "shift+left":
			msg = tea.KeyMsg{Type: tea.KeyShiftLeft}
		case "shift+right":
			msg = tea.KeyMsg{Type: tea.KeyShiftRight}
		case "shift+down":
			msg = tea.KeyMsg{Type: tea.KeyShiftDown}
		case "shift+end":
			msg = tea.KeyMsg{Type: tea.KeyShiftEnd}
		case "alt+shift+right":
			msg = tea.KeyMsg{Type: tea.KeyShiftRight, Alt: true}
		case "alt+backspace":
			msg = tea.KeyMsg{Type: tea.KeyBackspace, Alt: true}
		case "alt+down":
			msg = tea.KeyMsg{Type: tea.KeyDown, Alt: true}
		case "ctrl+a":
			msg = tea.KeyMsg{Type: tea.KeyCtrlA}
		case "ctrl+c":
			msg = tea.KeyMsg{Type: tea.KeyCtrlC}
		case "ctrl+x":
			msg = tea.KeyMsg{Type: tea.KeyCtrlX}
		case "ctrl+v":
			msg = tea.KeyMsg{Type: tea.KeyCtrlV}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		ed.update(msg)
	}
}

func expectValue(t *testing.T, ed *editor, want string) {
	t.Helper()
	if got := ed.value(); got != want {
		t.Fatalf("value = %q, want %q", got, want)
	}
}

func TestSelectionTypingReplaces(t *testing.T) {
	ed, _ := testEditor(t, "hello world")
	press(ed, "shift+right", "shift+right", "shift+right", "shift+right", "shift+right", "J")
	expectValue(t, ed, "J world")
}

func TestSelectionBackspaceAcrossLines(t *testing.T) {
	ed, _ := testEditor(t, "one\ntwo\nthree")
	press(ed, "right", "shift+down", "shift+down", "backspace")
	expectValue(t, ed, "ohree")
}

func TestSelectAllDelete(t *testing.T) {
	ed, _ := testEditor(t, "a\nb\nc")
	press(ed, "ctrl+a", "backspace")
	expectValue(t, ed, "")
}

func TestArrowCollapsesSelection(t *testing.T) {
	ed, _ := testEditor(t, "abcdef")
	press(ed, "shift+right", "shift+right", "shift+right", "left", "X")
	expectValue(t, ed, "Xabcdef")
}

func TestWordSelectionAndDelete(t *testing.T) {
	ed, _ := testEditor(t, "alpha beta gamma")
	press(ed, "alt+shift+right", "alt+shift+right", "backspace")
	expectValue(t, ed, " gamma")
	press(ed, "end", "alt+backspace")
	expectValue(t, ed, " ")
}

func TestCopyCutPaste(t *testing.T) {
	ed, clip := testEditor(t, "copy me\nsecond")
	press(ed, "shift+end", "ctrl+c")
	if *clip != "copy me" {
		t.Fatalf("clipboard = %q", *clip)
	}
	press(ed, "down", "end", "ctrl+v")
	expectValue(t, ed, "copy me\nsecondcopy me")

	press(ed, "ctrl+x")
	expectValue(t, ed, "copy me")
	if *clip != "secondcopy me\n" {
		t.Fatalf("cut line clipboard = %q", *clip)
	}
}

func TestShiftEnterInsertsNewline(t *testing.T) {
	for name, msg := range map[string]tea.Msg{
		"alt+enter":    tea.KeyMsg{Type: tea.KeyEnter, Alt: true},
		"csi u":        fakeCSI("?CSI[49 51 59 50 117]?"),
		"modifyOthers": fakeCSI("?CSI[50 55 59 50 59 49 51 126]?"),
	} {
		t.Run(name, func(t *testing.T) {
			ed, _ := testEditor(t, "ab")
			press(ed, "right")
			if changed, _ := ed.update(msg); !changed {
				t.Fatal("no change")
			}
			expectValue(t, ed, "a\nb")
		})
	}
}

func TestListContinuation(t *testing.T) {
	ed, _ := testEditor(t, "- item")
	press(ed, "end", "enter", "x")
	expectValue(t, ed, "- item\n- x")

	ed, _ = testEditor(t, "1. first")
	press(ed, "end", "enter", "y")
	expectValue(t, ed, "1. first\n2. y")

	ed, _ = testEditor(t, "- [x] done")
	press(ed, "end", "enter", "z")
	expectValue(t, ed, "- [x] done\n- [ ] z")

	ed, _ = testEditor(t, "- a")
	press(ed, "end", "enter", "enter")
	expectValue(t, ed, "- a\n")
}

func TestTabIndentsListAndSelection(t *testing.T) {
	ed, _ := testEditor(t, "- a\nplain")
	press(ed, "end", "tab")
	expectValue(t, ed, "  - a\nplain")
	press(ed, "shift+tab")
	expectValue(t, ed, "- a\nplain")
	press(ed, "down", "tab")
	expectValue(t, ed, "- a\npla  in")
	press(ed, "ctrl+a", "tab")
	expectValue(t, ed, "  - a\n  pla  in")
}

func TestMoveLineDown(t *testing.T) {
	ed, _ := testEditor(t, "one\ntwo\nthree")
	press(ed, "alt+down")
	expectValue(t, ed, "two\none\nthree")
	if ed.cur.row != 1 {
		t.Fatalf("cursor row = %d", ed.cur.row)
	}
}

func TestWrappedVerticalMovement(t *testing.T) {
	ed, _ := testEditor(t, strings.Repeat("word ", 20))
	if rows := ed.vrows(); len(rows) < 3 {
		t.Fatalf("expected wrapping, got %d rows", len(rows))
	}
	press(ed, "down")
	if ed.cur.row != 0 || ed.cur.col == 0 {
		t.Fatalf("down should move within the wrapped line, cur = %+v", ed.cur)
	}
}

func TestMouseClickAndDrag(t *testing.T) {
	ed, _ := testEditor(t, "first line\nsecond line")
	ed.mouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}, 2, 1)
	if ed.cur != (pos{1, 2}) {
		t.Fatalf("click cur = %+v", ed.cur)
	}
	ed.mouse(tea.MouseMsg{Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft}, 8, 1)
	ed.mouse(tea.MouseMsg{Action: tea.MouseActionRelease}, 8, 1)
	if got := ed.text(ed.selection()); got != "cond l" {
		t.Fatalf("drag selection = %q", got)
	}
}

func TestUndoRestoresSelectionDelete(t *testing.T) {
	ed, _ := testEditor(t, "keep this")
	press(ed, "ctrl+a", "backspace")
	ed.undoChange()
	expectValue(t, ed, "keep this")
}
