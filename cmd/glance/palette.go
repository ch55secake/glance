package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type paletteAction struct {
	name string
	hint string
	run  func(*model) tea.Cmd
}

func (m model) paletteActions() []paletteAction {
	actions := []paletteAction{
		{"Pull requests", "Switch inbox", func(m *model) tea.Cmd { m.detailOpen = false; m.activeSection = 0; return m.schedulePRPreview() }},
		{"Issues", "Switch inbox", func(m *model) tea.Cmd { m.detailOpen = false; m.activeSection = 1; return nil }},
		{"Notifications", "Switch inbox", func(m *model) tea.Cmd { m.detailOpen = false; m.activeSection = 2; return nil }},
		{"Refresh", "Reload current view", func(m *model) tea.Cmd {
			updated, cmd := m.Update(keyForPalette("r"))
			*m = updated.(model)
			return cmd
		}},
		{"Edit GitHub filter", "Change PR search", func(m *model) tea.Cmd {
			m.detailOpen = false
			m.activeSection = 0
			m.searching = true
			m.searchDraft = m.query
			return nil
		}},
		{"Help", "Show shortcuts", func(m *model) tea.Cmd { m.showHelp = true; return nil }},
		{"Quit", "Exit glance", func(*model) tea.Cmd { return tea.Quit }},
	}
	if !m.detailOpen && m.activeSection == 0 && len(m.pullRequests) > 0 {
		actions = append(actions[:5], append([]paletteAction{{"Review selected PR", "Open details & diff", func(m *model) tea.Cmd { return m.openSelectedPullRequest() }}}, actions[5:]...)...)
	}
	return actions
}

func keyForPalette(text string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)}
}

func (m model) matchingPaletteActions() []paletteAction {
	query := strings.ToLower(strings.TrimSpace(m.paletteQuery))
	actions := make([]paletteAction, 0)
	for _, action := range m.paletteActions() {
		if query == "" || strings.Contains(strings.ToLower(action.name+" "+action.hint), query) {
			actions = append(actions, action)
		}
	}
	return actions
}

func (m model) updatePaletteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	actions := m.matchingPaletteActions()
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.paletteOpen = false
		m.paletteQuery = ""
		return m, m.schedulePRPreview()
	case "up":
		m.paletteSelection = max(0, m.paletteSelection-1)
	case "down":
		m.paletteSelection = min(max(0, len(actions)-1), m.paletteSelection+1)
	case "enter":
		if m.paletteSelection < len(actions) {
			m.paletteOpen = false
			m.paletteQuery = ""
			return m, actions[m.paletteSelection].run(&m)
		}
	case "backspace", "ctrl+h":
		m.paletteQuery = dropLastRune(m.paletteQuery)
		m.paletteSelection = 0
	case "ctrl+u":
		m.paletteQuery = ""
		m.paletteSelection = 0
	default:
		if msg.Type == tea.KeyRunes {
			m.paletteQuery += string(msg.Runes)
			m.paletteSelection = 0
		}
	}
	return m, nil
}

func (m model) paletteView(width int) []string {
	lines := []string{"  " + accentStyle.Render("COMMAND PALETTE"), "  " + textStyle.Render(clipLine(": "+safeText(m.paletteQuery)+"▌", width)), ""}
	actions := m.matchingPaletteActions()
	if len(actions) == 0 {
		return append(lines, "  "+mutedStyle.Render("No matching commands"))
	}
	visible := min(len(actions), max(1, m.height-9))
	start := min(max(0, m.paletteSelection-visible+1), len(actions)-visible)
	for i := start; i < start+visible; i++ {
		label := "  " + actions[i].name
		if i == m.paletteSelection {
			label = "▸ " + actions[i].name
		}
		row := align(clipLine(label, max(1, width-lipgloss.Width(actions[i].hint)-2)), mutedStyle.Render(clipLine(actions[i].hint, max(1, width/2))), width)
		if i == m.paletteSelection {
			row = selectedStyle.Width(width).Render(row)
		}
		lines = append(lines, "  "+row)
	}
	return lines
}
