package main

import (
	"context"
	"fmt"
	"strings"
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

const dashboardLoadTimeout = 2 * time.Minute

type model struct {
	width         int
	height        int
	activeSection int
	loading       bool
	viewer        githubapi.User
	apiError      string
	pullRequests  []githubapi.PullRequest
	selectedPR    int
	client        *githubapi.Client
}

type dashboardLoadedMsg struct {
	viewer       githubapi.User
	pullRequests []githubapi.PullRequest
	err          error
}

func initialModel() model {
	return model{width: 80, height: 24, loading: true, client: githubapi.NewClient()}
}

func (m model) Init() tea.Cmd {
	return m.loadDashboard()
}

func (m model) loadDashboard() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), dashboardLoadTimeout)
		defer cancel()

		viewer, err := m.client.Viewer(ctx)
		if err != nil {
			return dashboardLoadedMsg{err: err}
		}
		pullRequests, err := m.client.ListOpenPullRequests(ctx)
		return dashboardLoadedMsg{viewer: viewer, pullRequests: pullRequests, err: err}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case dashboardLoadedMsg:
		m.loading = false
		m.apiError = ""
		m.viewer = msg.viewer
		m.pullRequests = msg.pullRequests
		if msg.err != nil {
			m.apiError = msg.err.Error()
		}
		m.clampSelectedPR()
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "left", "h":
			m.activeSection = (m.activeSection + len(sections) - 1) % len(sections)
		case "right", "l", "tab":
			m.activeSection = (m.activeSection + 1) % len(sections)
		case "up", "k":
			if m.activeSection == 1 && m.selectedPR > 0 {
				m.selectedPR--
			}
		case "down", "j":
			if m.activeSection == 1 && m.selectedPR < len(m.pullRequests)-1 {
				m.selectedPR++
			}
		case "home":
			if m.activeSection == 1 {
				m.selectedPR = 0
			}
		case "end":
			if m.activeSection == 1 {
				m.selectedPR = len(m.pullRequests) - 1
				m.clampSelectedPR()
			}
		case "r", "R":
			m.loading = true
			m.apiError = ""
			m.pullRequests = nil
			m.selectedPR = 0
			return m, m.loadDashboard()
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

	help := "←/→ or h/l: sections • r: refresh • q: quit"
	if m.activeSection == 1 {
		help = "←/→: sections • ↑/↓ or j/k: select • r: refresh • q: quit"
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
		mutedStyle.Render(help),
	)
}

func (m model) sectionContent() string {
	switch m.activeSection {
	case 0:
		content := "Welcome to glance"
		switch {
		case m.loading:
			return content + "\n\nConnecting to GitHub and loading pull requests..."
		case m.apiError != "":
			if m.viewer.Login == "" {
				return content + "\n\n" + m.apiError
			}
			return content + "\n\nConnected as @" + m.viewer.Login + ".\nCould not load pull requests: " + m.apiError
		case m.viewer.Login != "":
			return content + "\n\nConnected as @" + m.viewer.Login + ".\n" + fmt.Sprintf("Loaded %d open pull requests across your repositories.", len(m.pullRequests))
		default:
			return content + "\n\nAuthenticate with `gh auth login` to connect to GitHub."
		}
	case 1:
		return m.pullRequestContent()
	case 2:
		return "No issues loaded yet."
	case 3:
		return "No notifications loaded yet."
	default:
		return "Select a dashboard section."
	}
}

func (m model) pullRequestContent() string {
	switch {
	case m.loading:
		return "Loading open pull requests across your repositories..."
	case m.apiError != "":
		return "Could not load open pull requests.\n\n" + m.apiError + "\n\nPress r to try again."
	case len(m.pullRequests) == 0:
		return "No open pull requests found across repositories visible to your account.\n\nPress r to refresh."
	}

	visibleCount := min(m.visiblePRCount(), len(m.pullRequests))
	start := m.selectedPR - visibleCount/2
	if start < 0 {
		start = 0
	}
	if maxStart := len(m.pullRequests) - visibleCount; start > maxStart {
		start = maxStart
	}
	end := min(start+visibleCount, len(m.pullRequests))
	lines := []string{fmt.Sprintf("Open pull requests (%d)", len(m.pullRequests))}
	for index := start; index < end; index++ {
		pullRequest := m.pullRequests[index]
		marker := "  "
		if index == m.selectedPR {
			marker = "› "
		}
		state := "OPEN"
		if pullRequest.Draft {
			state = "DRAFT"
		}
		title := strings.Join(strings.Fields(pullRequest.Title), " ")
		firstLine := fmt.Sprintf("%s[%s] %s#%d  %s", marker, state, pullRequest.Repository, pullRequest.Number, title)
		lines = append(lines, m.truncatePRLine(firstLine), fmt.Sprintf("    @%s • updated %s", pullRequest.User.Login, relativeTime(pullRequest.UpdatedAt)))
	}
	lines = append(lines, fmt.Sprintf("Showing %d-%d of %d", start+1, end, len(m.pullRequests)))
	return strings.Join(lines, "\n")
}

func (m model) visiblePRCount() int {
	if m.height <= 14 {
		return 1
	}
	return max(1, (m.height-14)/2)
}

func (m model) truncatePRLine(line string) string {
	availableWidth := m.width - 14
	if availableWidth < 20 {
		availableWidth = 20
	}
	if lipgloss.Width(line) <= availableWidth {
		return line
	}

	var truncated strings.Builder
	width := 0
	for _, char := range line {
		charWidth := lipgloss.Width(string(char))
		if width+charWidth > availableWidth-1 {
			break
		}
		truncated.WriteRune(char)
		width += charWidth
	}
	return truncated.String() + "…"
}

func (m *model) clampSelectedPR() {
	if m.selectedPR < 0 {
		m.selectedPR = 0
	}
	if m.selectedPR >= len(m.pullRequests) {
		m.selectedPR = max(0, len(m.pullRequests)-1)
	}
}

func relativeTime(updatedAt time.Time) string {
	if updatedAt.IsZero() {
		return "time unknown"
	}
	ago := time.Since(updatedAt)
	if ago < time.Minute {
		return "just now"
	}
	if ago < time.Hour {
		return fmt.Sprintf("%dm ago", int(ago.Minutes()))
	}
	if ago < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(ago.Hours()))
	}
	if ago < 30*24*time.Hour {
		return fmt.Sprintf("%dd ago", int(ago.Hours()/24))
	}
	return updatedAt.Format("Jan 2, 2006")
}
