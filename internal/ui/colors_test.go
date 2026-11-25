package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func renderColored(t *testing.T, md string) string {
	t.Helper()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	r, err := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"), glamour.WithWordWrap(80))
	if err != nil {
		t.Fatal(err)
	}
	var colors palette
	out, err := r.Render(colors.colorize(md))
	if err != nil {
		t.Fatal(err)
	}
	return colors.paint(out)
}

func TestColorMarkup(t *testing.T) {
	tests := []struct {
		name, md, seq, text string
	}{
		{"span", `<span style="color:red">red text</span>`, "\x1b[38;2;255;95;95m", "red"},
		{"span with other styles", `<span style="font-weight:bold; color: #00f">blue</span>`, "\x1b[38;2;0;0;255m", "blue"},
		{"font", `<font color="green">go</font>`, "\x1b[38;2;95;215;95m", "go"},
		{"latex text", `$\color{red}{\text{This is red}}$`, "\x1b[38;2;255;95;95m", "This"},
		{"latex hex", `$\color{#228B22}{\text{forest}}$`, "\x1b[38;2;34;139;34m", "forest"},
		{"latex bare", `$\color{blue}{plain}$`, "\x1b[38;2;95;135;255m", "plain"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := renderColored(t, tt.md)
			if !strings.Contains(out, tt.seq+tt.text) {
				t.Fatalf("want %q before %q in %q", tt.seq, tt.text, out)
			}
			if strings.ContainsAny(out, string([]rune{colorEnd, colorStart})) || strings.Contains(out, "color") {
				t.Fatalf("markup leaked into %q", out)
			}
		})
	}
}

func TestColorReappliedAcrossWords(t *testing.T) {
	out := renderColored(t, `<span style="color:red">one two three</span> after`)
	seq := "\x1b[38;2;255;95;95m"
	for _, w := range []string{"one", "two", "three"} {
		i := strings.Index(out, w)
		if i < 0 || strings.LastIndex(out[:i], seq) < strings.LastIndex(out[:i], "\x1b[38;5;252m") {
			t.Fatalf("%q is not red in %q", w, out)
		}
	}
	after := strings.Index(out, "after")
	if strings.LastIndex(out[:after], "\x1b[39m") < strings.LastIndex(out[:after], seq) {
		t.Fatalf("color leaks past the span in %q", out)
	}
}

func TestColorUntouchedInCodeAndUnknownColors(t *testing.T) {
	var colors palette
	md := "```html\n<span style=\"color:red\">x</span>\n```\n<span style=\"color:notacolor\">y</span>"
	got := colors.colorize(md)
	want := "```html\n<span style=\"color:red\">x</span>\n```\ny"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestColorPlainWithoutColorSupport(t *testing.T) {
	var colors palette
	out := colors.paint(colors.colorize(`<span style="color:red">red</span>`))
	if out != "red" {
		t.Fatalf("got %q", out)
	}
}

func TestHighlightLine(t *testing.T) {
	var hl lineHighlighter
	check := func(line, sub string, want attr) {
		t.Helper()
		a := hl.line(line)
		i := strings.Index(line, sub)
		if a[i] != want {
			t.Fatalf("%q in %q: got %+v, want %+v", sub, line, a[i], want)
		}
	}
	check("## Title", "#", mdMarker)
	check("## Title", "Title", mdHeading)
	check("say **loud** now", "**", mdMarker)
	check("say **loud** now", "loud", attr{bold: true})
	check("an *it* word", "it", attr{italic: true})
	check("snake_case_name", "case", attr{})
	check("run `go test`", "go", mdCode)
	check("- [x] shipped", "shipped", mdDone)
	check("- item", "-", mdBullet)
	check(`<span style="color:red">hot</span>`, "span", mdMarker)
	check(`<span style="color:red">hot</span>`, "hot", attr{fg: lipgloss.Color("#FF5F5F")})
	check("see [docs](https://x.dev)", "docs", mdLink)

	check("```go", "go", mdMarker)
	check("x := **1**", "1", mdCode)
	check("```", "`", mdMarker)
	check("**back**", "back", attr{bold: true})
}
