package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var sections = []string{"Overview", "Pull requests", "Issues", "Notifications"}

var (
	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	mutedStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	activeTabStyle = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color("229")).Background(lipgloss.Color("62")).Padding(0, 1)
	inactiveTabStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Padding(0, 1)
	panelStyle       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("62")).Padding(1, 2)
)

type model struct {
	width         int
	activeSection int
}

func initialModel() model {
	return model{width: 80}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "left", "h":
			m.activeSection = (m.activeSection + len(sections) - 1) % len(sections)
		case "right", "l", "tab":
			m.activeSection = (m.activeSection + 1) % len(sections)
		}
	}

	return m, nil
}

func (m model) View() string {
	width := m.width
	if width < 48 {
		width = 48
	}

	tabs := make([]string, 0, len(sections))
	for i, section := range sections {
		style := inactiveTabStyle
		if i == m.activeSection {
			style = activeTabStyle
		}
		tabs = append(tabs, style.Render(section))
	}

	contentWidth := width - 4
	if contentWidth < 40 {
		contentWidth = 40
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render("glance"),
		mutedStyle.Render("GitHub dashboard"),
		"",
		lipgloss.JoinHorizontal(lipgloss.Top, tabs...),
		"",
		panelStyle.Width(contentWidth).Render(m.sectionContent()),
		"",
		mutedStyle.Render("←/→ or h/l: navigate • q: quit"),
	)
}

func (m model) sectionContent() string {
	switch m.activeSection {
	case 0:
		return "Welcome to glance\nYour GitHub dashboard starts here.\n\nGitHub data integration is not implemented yet."
	case 1:
		return "No pull requests loaded yet."
	case 2:
		return "No issues loaded yet."
	case 3:
		return "No notifications loaded yet."
	default:
		return "Select a dashboard section."
	}
}
