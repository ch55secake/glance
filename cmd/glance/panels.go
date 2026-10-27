package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// panel renders a fixed-size surface without painting the terminal background.
// Headings and position cues stay outside the scrollable content.
func panel(width, height int, title, status string, content []string, focused bool) []string {
	width, height = max(6, width), max(3, height)
	border := lipgloss.NewStyle().Foreground(lipgloss.Color(themeBorderColor))
	if focused {
		border = accentStyle
		if title != "" {
			title = "▎ " + title
		}
	}
	cap := func(left, right, label string) string {
		label = clipLine(label, width-6)
		if label != "" {
			label = " " + label + " "
		}
		return border.Render(left + "─" + label + strings.Repeat("─", max(0, width-3-lipgloss.Width(label))) + right)
	}
	lines := []string{cap("╭", "╮", title)}
	for i := 0; i < height-2; i++ {
		value := ""
		if i < len(content) {
			value = ansi.Truncate(content[i], width-4, "…")
		}
		lines = append(lines, border.Render("│")+" "+lipgloss.NewStyle().Width(width-4).Render(value)+" "+border.Render("│"))
	}
	return append(lines, cap("╰", "╯", status))
}

// Keep the working context visible beneath command, file, and comment dialogs.
// ANSI-aware cuts preserve cell positions even with wide glyphs at the card edge.
func (m model) modalView() string {
	background := m
	background.paletteOpen, background.filePickerOpen, background.themePickerOpen, background.commentComposerOpen = false, false, false, false
	lines := strings.Split(background.View(), "\n")
	for len(lines) < m.height {
		lines = append(lines, "")
	}
	for i, line := range lines {
		lines[i] = mutedStyle.Faint(true).Width(m.width).Render(ansi.Strip(line))
	}
	width := m.width - 4
	cardWidth := min(width, 72)
	var card []string
	if m.themePickerOpen {
		cardWidth = min(width, 78)
		card = m.themePickerView(width)
	} else if m.filePickerOpen {
		cardWidth = min(width, 100)
		card = m.filePickerView(width)
	} else if m.commentComposerOpen {
		card = m.commentComposerView(width)
	} else {
		card = m.paletteView(width)
	}
	left := 2 + (width-cardWidth)/2
	top := 3 + max(0, (m.height-5-len(card))/2)
	for _, row := range []int{top - 1, top + len(card)} {
		if row >= 3 && row < m.height-2 {
			lines[row] = ansi.Cut(lines[row], 0, left-1) + strings.Repeat(" ", cardWidth+2) + ansi.Cut(lines[row], left+cardWidth+1, m.width)
		}
	}
	for i, line := range card {
		row := top + i
		if row >= m.height-2 {
			break
		}
		line = strings.TrimPrefix(line, strings.Repeat(" ", left))
		lines[row] = strings.Repeat(" ", m.width)
		lines[row] = ansi.Cut(lines[row], 0, left-1) + " " + line + " " + ansi.Cut(lines[row], left+cardWidth+1, m.width)
	}
	lines[1] = m.navigation(width)
	lines[m.height-2], lines[m.height-1] = "", m.footer(width)
	return strings.Join(lines, "\n")
}
