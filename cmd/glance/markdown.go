package main

import (
	"strings"

	"github.com/charmbracelet/glamour"
	glamouransi "github.com/charmbracelet/glamour/ansi"
	glamourstyles "github.com/charmbracelet/glamour/styles"
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
	style := glamour.WithStyles(markdownThemeStyles())
	if lipgloss.ColorProfile() == termenv.Ascii {
		style = glamour.WithStandardStyle("notty")
	}
	renderer, err := glamour.NewTermRenderer(
		style,
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

func markdownThemeStyles() glamouransi.StyleConfig {
	style := glamourstyles.DarkStyleConfig
	color := func(value string) *string { return &value }
	style.Document.Color = color(activeTheme.text)
	style.Heading.Color = color(activeTheme.accent)
	style.H1.Color, style.H1.BackgroundColor = color(activeTheme.accent), nil
	style.H2.Color, style.H3.Color = color(activeTheme.accent), color(activeTheme.accent)
	style.H4.Color, style.H5.Color, style.H6.Color = color(activeTheme.accent), color(activeTheme.accent), color(activeTheme.accent)
	style.Link.Color = color(activeTheme.brand)
	style.LinkText.Color = color(activeTheme.accent)
	style.Code.Color = color(activeTheme.codeString)
	style.Code.BackgroundColor = color(activeTheme.fileBackground)
	style.CodeBlock.Color = color(activeTheme.muted)
	if style.CodeBlock.Chroma != nil {
		chroma := *style.CodeBlock.Chroma
		chroma.Text.Color = color(activeTheme.text)
		chroma.Keyword.Color = color(activeTheme.keyword)
		chroma.Comment.Color = color(activeTheme.comment)
		chroma.LiteralNumber.Color = color(activeTheme.number)
		chroma.LiteralString.Color = color(activeTheme.codeString)
		chroma.GenericInserted.Color = color(activeTheme.addedText)
		chroma.GenericDeleted.Color = color(activeTheme.deletedText)
		chroma.Background.BackgroundColor = color(activeTheme.fileBackground)
		style.CodeBlock.Chroma = &chroma
	}
	return style
}

func previewMarkdown(body string, width int) []string {
	lines := markdownLines(body, width)
	for i, line := range lines {
		lines[i] = strings.TrimPrefix(line, "  ")
	}
	return lines
}
