package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *model) openThemePicker() {
	m.paletteOpen = false
	m.themePickerOpen = true
	m.themePickerSelection = themeIndex(m.theme)
}

func (m model) updateThemePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.themePickerSelection = min(max(0, m.themePickerSelection), len(themes)-1)
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.themePickerOpen = false
	case "up", "k", "shift+tab":
		m.themePickerSelection = max(0, m.themePickerSelection-1)
	case "down", "j", "tab":
		m.themePickerSelection = min(len(themes)-1, m.themePickerSelection+1)
	case "home":
		m.themePickerSelection = 0
	case "end":
		m.themePickerSelection = len(themes) - 1
	case "enter":
		m.theme = applyTheme(themes[m.themePickerSelection].id)
		m.themePickerOpen = false
		m.themeConfigError = ""
		m.detailLines = nil
		m.inboxLines = nil
		if m.detailOpen {
			m.cacheDetailLines()
		}
		if m.hasInboxPreview() && !m.detailOpen {
			m.cacheInboxPreview()
		}
		if err := saveThemePreference(m.theme); err != nil {
			m.themeConfigError = safeText(err.Error())
		}
	}
	return m, nil
}

func themeSwatches(candidate theme) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(candidate.brand)).Render("◆") + " " +
		lipgloss.NewStyle().Foreground(lipgloss.Color(candidate.accent)).Render("◆") + " " +
		lipgloss.NewStyle().Foreground(lipgloss.Color(candidate.good)).Render("◆") + " " +
		lipgloss.NewStyle().Foreground(lipgloss.Color(candidate.warning)).Render("◆") + " " +
		lipgloss.NewStyle().Foreground(lipgloss.Color(candidate.error)).Render("◆")
}

func (m model) themePickerView(width int) []string {
	cardWidth := min(width, 78)
	inner := cardWidth - 4
	visible := min(len(themes), max(1, m.height-11))
	selected := min(max(0, m.themePickerSelection), len(themes)-1)
	start := min(max(0, selected-visible+1), len(themes)-visible)
	content := []string{mutedStyle.Render("Preview a palette · selection applies and saves immediately"), ""}
	for i := start; i < start+visible; i++ {
		candidate := themes[i]
		marker := "  "
		style := textStyle
		if i == selected {
			marker = "▸ "
			style = selectedStyle.Bold(true)
		}
		current := ""
		if candidate.id == m.theme {
			current = "  CURRENT"
		}
		labelWidth := inner - lipgloss.Width(themeSwatches(candidate)) - 3
		row := marker + style.Render(clipLine(candidate.name+current, labelWidth)) + "  " + themeSwatches(candidate)
		content = append(content, row)
		content = append(content, mutedStyle.Render(clipLine(candidate.description, inner)))
	}
	position := fmt.Sprintf("%d / %d", selected+1, len(themes))
	height := min(m.height-5, len(content)+2)
	lines := panel(cardWidth, height, "COLOR THEMES", position+" · Enter applies", content, true)
	margin := strings.Repeat(" ", 2+(width-cardWidth)/2)
	for i := range lines {
		lines[i] = margin + lines[i]
	}
	return lines
}
