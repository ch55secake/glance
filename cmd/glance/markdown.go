package main

import (
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// markdownLines renders GitHub-flavored PR descriptions with headings, lists,
// links, and highlighted fenced code. Rendering is bounded to the review column.
func markdownLines(body string, width int) []string {
	if strings.TrimSpace(body) == "" {
		return []string{"  " + mutedStyle.Render("(No description.)")}
	}
	style := "dark"
	if lipgloss.ColorProfile() == termenv.Ascii {
		style = "notty"
	}
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithColorProfile(lipgloss.ColorProfile()),
		glamour.WithWordWrap(max(1, width-2)),
		glamour.WithPreservedNewLines(),
	)
	if err == nil {
		var rendered string
		rendered, err = renderer.Render(safeMultiline(body))
		if err == nil {
			if lipgloss.ColorProfile() == termenv.Ascii {
				rendered = ansi.Strip(rendered)
			}
			result := make([]string, 0)
			for _, line := range strings.Split(strings.Trim(rendered, "\n"), "\n") {
				// Fenced code can be wider than Glamour's prose wrapping. Never
				// discard the tail of a code line to fit the review column.
				for _, part := range strings.Split(ansi.Hardwrap(line, max(1, width-2), true), "\n") {
					result = append(result, "  "+part)
				}
			}
			return result
		}
	}
	// Rendering errors should not hide a PR's description.
	result := make([]string, 0)
	for _, line := range strings.Split(safeMultiline(body), "\n") {
		for _, part := range wrapDiffLine(line, max(1, width-2)) {
			result = append(result, "  "+part)
		}
	}
	return result
}

func previewMarkdown(body string, width int) []string {
	lines := markdownLines(body, width)
	for i, line := range lines {
		lines[i] = strings.TrimPrefix(line, "  ")
	}
	return lines
}
