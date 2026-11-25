package ui

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

var (
	codeColor   = lipgloss.AdaptiveColor{Light: "#B4235A", Dark: "#F5A97F"}
	quoteColor  = lipgloss.AdaptiveColor{Light: "#6B6B76", Dark: "#A0A0AA"}
	selectionBg = lipgloss.AdaptiveColor{Light: "#DDD5FF", Dark: "#3D3566"}
)

type attr struct {
	fg                              lipgloss.TerminalColor
	bold, italic, strike, underline bool
}

var (
	mdMarker  = attr{fg: subtle}
	mdHeading = attr{fg: accent, bold: true}
	mdCode    = attr{fg: codeColor}
	mdQuote   = attr{fg: quoteColor, italic: true}
	mdBullet  = attr{fg: accent, bold: true}
	mdLink    = attr{fg: accent, underline: true}
	mdDone    = attr{fg: subtle, strike: true}
)

var (
	headingRe     = regexp.MustCompile(`^(#{1,6})(?:\s+|$)`)
	hrRe          = regexp.MustCompile(`^\s*(?:(?:-\s*){3,}|(?:\*\s*){3,}|(?:_\s*){3,})$`)
	tableSepRe    = regexp.MustCompile(`^\s*\|?(?:\s*:?-+:?\s*\|)+\s*(?::?-+:?\s*)?$`)
	codeSpanRe    = regexp.MustCompile("`[^`]+`")
	boldStarRe    = regexp.MustCompile(`\*\*[^*\s](?:[^*]*?[^*\s])?\*\*`)
	boldUnderRe   = regexp.MustCompile(`__[^_\s](?:[^_]*?[^_\s])?__`)
	italicStarRe  = regexp.MustCompile(`\*[^*\s](?:[^*]*?[^*\s])?\*`)
	italicUnderRe = regexp.MustCompile(`_[^_\s](?:[^_]*?[^_\s])?_`)
	strikeRe      = regexp.MustCompile(`~~[^~]+~~`)
	linkRe        = regexp.MustCompile(`!?\[([^\]]*)\]\(([^)]*)\)`)
	urlRe         = regexp.MustCompile(`https?://[^\s)>\]]+`)
	htmlTagRe     = regexp.MustCompile(`</?[a-zA-Z][^>]*>`)
)

// lineHighlighter styles markdown source one line at a time; it carries the
// fenced-code state from line to line.
type lineHighlighter struct{ fence string }

func (h *lineHighlighter) line(s string) []attr {
	a := make([]attr, len(s))
	trimmed := strings.TrimSpace(s)
	if m := fenceMarker(trimmed); m != "" && (h.fence == "" || strings.HasPrefix(trimmed, h.fence)) {
		if h.fence == "" {
			h.fence = m
		} else {
			h.fence = ""
		}
		fill(a, 0, len(s), mdMarker)
		return a
	}
	if h.fence != "" {
		fill(a, 0, len(s), mdCode)
		return a
	}
	if hrRe.MatchString(s) {
		fill(a, 0, len(s), mdMarker)
		return a
	}
	isTable := strings.HasPrefix(trimmed, "|")
	if isTable && tableSepRe.MatchString(s) {
		fill(a, 0, len(s), mdMarker)
		return a
	}
	body := 0
	if m := headingRe.FindStringSubmatchIndex(s); m != nil {
		fill(a, m[2], m[3], mdMarker)
		fill(a, m[3], len(s), mdHeading)
		body = m[1]
	} else if m := quotePrefixRe.FindStringIndex(s); m != nil {
		fill(a, m[0], m[1], mdMarker)
		fill(a, m[1], len(s), mdQuote)
		body = m[1]
	} else if m := listPrefixRe.FindStringSubmatchIndex(s); m != nil {
		if m[4] >= 0 {
			fill(a, m[4], m[5], mdBullet)
		} else {
			fill(a, m[6], m[9], mdBullet)
		}
		if m[12] >= 0 {
			if strings.ContainsAny(s[m[12]:m[13]], "xX") {
				fill(a, m[12], m[13], attr{fg: success, bold: true})
				fill(a, m[1], len(s), mdDone)
			} else {
				fill(a, m[12], m[13], mdBullet)
			}
		}
		body = m[1]
	}
	inline(s, a, body)
	if isTable {
		for i := range s {
			if s[i] == '|' {
				a[i] = mdMarker
			}
		}
	}
	return a
}

func inline(s string, a []attr, from int) {
	each := func(re *regexp.Regexp, f func(m []int)) {
		for _, m := range re.FindAllStringSubmatchIndex(s[from:], -1) {
			for i := range m {
				if m[i] >= 0 {
					m[i] += from
				}
			}
			f(m)
		}
	}
	emphasis := func(re *regexp.Regexp, n int, set func(*attr)) {
		each(re, func(m []int) {
			if n == 1 && !emphasisBoundary(s, m[0], m[1]) {
				return
			}
			modify(a, m[0]+n, m[1]-n, set)
			fill(a, m[0], m[0]+n, mdMarker)
			fill(a, m[1]-n, m[1], mdMarker)
		})
	}

	each(htmlTagRe, func(m []int) { fill(a, m[0], m[1], mdMarker) })
	each(urlRe, func(m []int) { fill(a, m[0], m[1], mdLink) })
	each(linkRe, func(m []int) {
		fill(a, m[0], m[1], mdMarker)
		fill(a, m[2], m[3], mdLink)
	})
	emphasis(boldStarRe, 2, func(t *attr) { t.bold = true })
	emphasis(boldUnderRe, 2, func(t *attr) { t.bold = true })
	emphasis(italicStarRe, 1, func(t *attr) { t.italic = true })
	emphasis(italicUnderRe, 1, func(t *attr) { t.italic = true })
	emphasis(strikeRe, 2, func(t *attr) { t.strike = true })
	colored := func(m []int, color [2]int, content [2]int) {
		hex, ok := resolveColor(s[color[0]:color[1]])
		if !ok {
			return
		}
		fill(a, m[0], m[1], mdMarker)
		modify(a, content[0], content[1], func(t *attr) { t.fg = lipgloss.Color(hex) })
	}
	each(spanColorRe, func(m []int) { colored(m, [2]int{m[2], m[3]}, [2]int{m[4], m[5]}) })
	each(fontColorRe, func(m []int) { colored(m, [2]int{m[2], m[3]}, [2]int{m[4], m[5]}) })
	each(latexColorRe, func(m []int) {
		content := [2]int{m[4], m[5]}
		if m[4] < 0 {
			content = [2]int{m[6], m[7]}
		}
		colored(m, [2]int{m[2], m[3]}, content)
	})
	each(codeSpanRe, func(m []int) {
		fill(a, m[0], m[1], mdCode)
		fill(a, m[0], m[0]+1, mdMarker)
		fill(a, m[1]-1, m[1], mdMarker)
	})
}

// emphasisBoundary rejects single-marker matches that are really part of
// ** / __ runs or of snake_case words.
func emphasisBoundary(s string, start, end int) bool {
	marker := s[start]
	if start > 0 && (s[start-1] == marker || (marker == '_' && isWordByte(s[start-1]))) {
		return false
	}
	if end < len(s) && (s[end] == marker || (marker == '_' && isWordByte(s[end]))) {
		return false
	}
	return true
}

func isWordByte(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

func fill(a []attr, from, to int, v attr) {
	for i := max(from, 0); i < min(to, len(a)); i++ {
		a[i] = v
	}
}

func modify(a []attr, from, to int, f func(*attr)) {
	for i := max(from, 0); i < min(to, len(a)); i++ {
		f(&a[i])
	}
}

func runeAttrs(line []rune, byteAttrs []attr) []attr {
	out := make([]attr, len(line))
	b := 0
	for i, r := range line {
		if b < len(byteAttrs) {
			out[i] = byteAttrs[b]
		}
		b += utf8.RuneLen(r)
	}
	return out
}

type cell struct {
	attr
	selected, cursor bool
}

func (c cell) render(s string) string {
	st := lipgloss.NewStyle().
		Bold(c.bold).
		Italic(c.italic).
		Strikethrough(c.strike).
		Underline(c.underline)
	if c.fg != nil {
		st = st.Foreground(c.fg)
	}
	if c.selected {
		st = st.Background(selectionBg)
	}
	if c.cursor {
		st = st.Reverse(true)
	}
	return st.Render(s)
}

func (e editor) view() string {
	rows := e.vrows()
	top := clamp(e.top, 0, len(rows)-1)
	end := min(top+e.height, len(rows))

	var hl lineHighlighter
	attrs := make(map[int][]attr)
	for i := 0; i <= rows[end-1].line; i++ {
		a := hl.line(string(e.lines[i]))
		if i >= rows[top].line {
			attrs[i] = runeAttrs(e.lines[i], a)
		}
	}

	out := make([]string, 0, e.height)
	for i := top; i < end; i++ {
		out = append(out, e.renderRow(rows, i, attrs[rows[i].line]))
	}
	for len(out) < e.height {
		out = append(out, "")
	}
	return strings.Join(out, "\n")
}

func (e editor) renderRow(rows []vrow, i int, attrs []attr) string {
	r := rows[i]
	line := e.lines[r.line]
	hasSel := e.hasSelection()
	selA, selB := e.selection()
	inSel := func(p pos) bool { return hasSel && !p.less(selA) && p.less(selB) }

	var sb strings.Builder
	var run []rune
	var style cell
	flush := func() {
		if len(run) > 0 {
			sb.WriteString(style.render(string(run)))
			run = run[:0]
		}
	}
	for c := r.start; c < r.end; c++ {
		p := pos{r.line, c}
		cs := cell{attr: attrs[c], selected: inSel(p), cursor: e.focused && p == e.cur}
		if cs != style {
			flush()
			style = cs
		}
		switch ch := line[c]; {
		case ch == '\t':
			run = append(run, []rune(strings.Repeat(" ", tabWidth))...)
		case !unicode.IsPrint(ch):
			run = append(run, '·')
		default:
			run = append(run, ch)
		}
	}
	flush()

	lastSegment := i+1 >= len(rows) || rows[i+1].line != r.line
	if lastSegment {
		eol := pos{r.line, len(line)}
		cursor := e.focused && e.cur == eol
		if cursor || inSel(eol) {
			sb.WriteString(cell{selected: inSel(eol), cursor: cursor}.render(" "))
		}
	}
	return sb.String()
}
