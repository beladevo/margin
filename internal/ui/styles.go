package ui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	accent  = lipgloss.AdaptiveColor{Light: "#6B4FD8", Dark: "#A48CFF"}
	subtle  = lipgloss.AdaptiveColor{Light: "#9A9A9A", Dark: "#5C5C66"}
	text    = lipgloss.AdaptiveColor{Light: "#1F1F24", Dark: "#E4E4EA"}
	warning = lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#FBBF24"}
	danger  = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"}
	success = lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#4ADE80"}
)

var styles = struct {
	pane, paneFocused                     lipgloss.Style
	treeDir, treeNote                     lipgloss.Style
	treeCursor, treeCursorBlur            lipgloss.Style
	muted, title, key, status             lipgloss.Style
	modified, saved, errorText, mode      lipgloss.Style
	banner, match, selected, overlayTitle lipgloss.Style
}{
	pane:           lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(subtle).Padding(0, 1),
	paneFocused:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(0, 1),
	treeDir:        lipgloss.NewStyle().Foreground(accent).Bold(true),
	treeNote:       lipgloss.NewStyle().Foreground(text),
	treeCursor:     lipgloss.NewStyle().Background(accent).Foreground(lipgloss.Color("#FFFFFF")).Bold(true),
	treeCursorBlur: lipgloss.NewStyle().Background(subtle).Foreground(lipgloss.Color("#FFFFFF")),
	muted:          lipgloss.NewStyle().Foreground(subtle),
	title:          lipgloss.NewStyle().Foreground(accent).Bold(true),
	key:            lipgloss.NewStyle().Foreground(accent).Bold(true),
	status:         lipgloss.NewStyle().Foreground(text),
	modified:       lipgloss.NewStyle().Foreground(warning),
	saved:          lipgloss.NewStyle().Foreground(success),
	errorText:      lipgloss.NewStyle().Foreground(danger).Bold(true),
	mode:           lipgloss.NewStyle().Background(accent).Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Padding(0, 1),
	banner:         lipgloss.NewStyle().Background(warning).Foreground(lipgloss.Color("#000000")).Bold(true).Padding(0, 1),
	match:          lipgloss.NewStyle().Foreground(accent).Bold(true),
	selected:       lipgloss.NewStyle().Background(accent).Foreground(lipgloss.Color("#FFFFFF")),
	overlayTitle:   lipgloss.NewStyle().Foreground(accent).Bold(true).MarginBottom(1),
}

func lipglossWidth(s string) int { return lipgloss.Width(s) }

func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(s, width, "…")
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	return max(lo, min(v, hi))
}
