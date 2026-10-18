package main

import (
	"context"
	"time"

	"glance/internal/githubapi"

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
	loading       bool
	viewer        githubapi.User
	apiError      string
}

type viewerLoadedMsg struct {
	viewer githubapi.User
	err    error
}

func initialModel() model {
	return model{width: 80, loading: true}
}

func (m model) Init() tea.Cmd {
	return loadViewer
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case viewerLoadedMsg:
		m.loading = false
		m.apiError = ""
		if msg.err != nil {
			m.apiError = msg.err.Error()
		} else {
			m.viewer = msg.viewer
		}
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

func loadViewer() tea.Msg {
	client, err := githubapi.NewClientFromEnv()
	if err != nil {
		return viewerLoadedMsg{err: err}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	viewer, err := client.Viewer(ctx)
	return viewerLoadedMsg{viewer: viewer, err: err}
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
		content := "Welcome to glance"
		switch {
		case m.loading:
			return content + "\n\nConnecting to GitHub..."
		case m.apiError != "":
			return content + "\n\n" + m.apiError
		case m.viewer.Login != "":
			return content + "\n\nConnected as @" + m.viewer.Login + ".\nPull request views are coming next."
		default:
			return content + "\n\nSet GLANCE_GITHUB_TOKEN to connect to GitHub."
		}
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
