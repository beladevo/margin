package ui

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

var (
	spanColorRe  = regexp.MustCompile(`(?is)<span\s+style\s*=\s*["'][^"']*?color\s*:\s*([^;"']+)[^"']*["']\s*>(.*?)</span>`)
	fontColorRe  = regexp.MustCompile(`(?is)<font\s+color\s*=\s*["']?([^"'\s>]+)["']?\s*>(.*?)</font>`)
	latexColorRe = regexp.MustCompile(`\$\\color\{([^}]+)\}\{(?:\\text(?:bf|sf|tt|it|rm)?\{([^}]*)\}|([^}]*))\}\$`)
	hexColorRe   = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
)

var namedColors = map[string]string{
	"red":     "#FF5F5F",
	"green":   "#5FD75F",
	"blue":    "#5F87FF",
	"yellow":  "#FFD75F",
	"orange":  "#FFAF5F",
	"purple":  "#AF87FF",
	"violet":  "#AF87FF",
	"magenta": "#FF5FD7",
	"pink":    "#FF87D7",
	"cyan":    "#5FD7FF",
	"teal":    "#5FAFAF",
	"gray":    "#9E9E9E",
	"grey":    "#9E9E9E",
	"white":   "#FFFFFF",
	"black":   "#000000",
	"brown":   "#AF875F",
}

const (
	colorEnd   = '\uE000'
	colorStart = '\uE001'
)

type palette []string

func (p *palette) colorize(md string) string {
	lines := strings.Split(md, "\n")
	fence := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if marker := fenceMarker(trimmed); marker != "" {
			switch {
			case fence == "":
				fence = marker
			case strings.HasPrefix(trimmed, fence):
				fence = ""
			}
			continue
		}
		if fence != "" {
			continue
		}
		lines[i] = p.colorizeLine(line)
	}
	return strings.Join(lines, "\n")
}

func fenceMarker(line string) string {
	for _, m := range []string{"```", "~~~"} {
		if strings.HasPrefix(line, m) {
			return m
		}
	}
	return ""
}

func (p *palette) colorizeLine(line string) string {
	line = spanColorRe.ReplaceAllStringFunc(line, func(s string) string {
		g := spanColorRe.FindStringSubmatch(s)
		return p.mark(g[1], g[2])
	})
	line = fontColorRe.ReplaceAllStringFunc(line, func(s string) string {
		g := fontColorRe.FindStringSubmatch(s)
		return p.mark(g[1], g[2])
	})
	return latexColorRe.ReplaceAllStringFunc(line, func(s string) string {
		g := latexColorRe.FindStringSubmatch(s)
		return p.mark(g[1], g[2]+g[3])
	})
}

func (p *palette) mark(color, text string) string {
	hex, ok := resolveColor(color)
	if !ok {
		return text
	}
	*p = append(*p, hex)
	return string(colorStart+rune(len(*p)-1)) + text + string(colorEnd)
}

// Glamour mangles raw escape codes and resets styles between words, so
// colored spans travel through it as private-use markers and their color is
// re-applied after every escape sequence Glamour emits inside the span.
func (p palette) paint(rendered string) string {
	var b strings.Builder
	active := ""
	for i := 0; i < len(rendered); {
		r, size := utf8.DecodeRuneInString(rendered[i:])
		switch {
		case r == colorEnd:
			if active != "" {
				b.WriteString("\x1b[39m")
			}
			active = ""
		case r >= colorStart && int(r-colorStart) < len(p):
			active = sgr(p[r-colorStart])
			b.WriteString(active)
		case r == '\x1b':
			end := strings.IndexByte(rendered[i:], 'm')
			if end < 0 {
				b.WriteString(rendered[i:])
				return b.String()
			}
			b.WriteString(rendered[i : i+end+1])
			b.WriteString(active)
			i += end + 1
			continue
		default:
			b.WriteString(rendered[i : i+size])
		}
		i += size
	}
	return b.String()
}

func sgr(hex string) string {
	seq := termenv.Style{}.Foreground(lipgloss.ColorProfile().Color(hex)).Styled("")
	return strings.TrimSuffix(seq, "\x1b[0m")
}

func resolveColor(c string) (string, bool) {
	c = strings.ToLower(strings.TrimSpace(c))
	if hex, ok := namedColors[c]; ok {
		return hex, true
	}
	if !hexColorRe.MatchString(c) {
		return "", false
	}
	if len(c) == 4 {
		c = "#" + strings.Repeat(c[1:2], 2) + strings.Repeat(c[2:3], 2) + strings.Repeat(c[3:4], 2)
	}
	return c, true
}
