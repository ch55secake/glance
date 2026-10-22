package main

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

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
	selectedPRStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("229"))
	statusStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	draftStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("221"))
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
	filter        prFilter
	query         string
	searching     bool
	showHelp      bool
	requestID     int
	client        *githubapi.Client
}

type dashboardLoadedMsg struct {
	requestID    int
	viewer       githubapi.User
	pullRequests []githubapi.PullRequest
	err          error
}

type prFilter int

const (
	allPRs prFilter = iota
	minePRs
	draftPRs
)

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
			return dashboardLoadedMsg{requestID: m.requestID, err: err}
		}
		pullRequests, err := m.client.ListOpenPullRequests(ctx)
		return dashboardLoadedMsg{requestID: m.requestID, viewer: viewer, pullRequests: pullRequests, err: err}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case dashboardLoadedMsg:
		if msg.requestID != m.requestID {
			break
		}
		selected := m.selectedKey()
		m.loading = false
		m.apiError = ""
		m.viewer = msg.viewer
		m.pullRequests = msg.pullRequests
		if msg.err != nil {
			m.apiError = safeText(msg.err.Error())
		}
		m.restoreSelection(selected)
	case tea.KeyMsg:
		if m.searching {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc", "enter":
				m.searching = false
			case "backspace", "ctrl+h":
				m.query = dropLastRune(m.query)
				m.selectedPR = 0
			default:
				if msg.Type == tea.KeyRunes {
					m.query += string(msg.Runes)
					m.selectedPR = 0
				}
			}
			return m, nil
		}
		if m.showHelp {
			if msg.String() == "?" || msg.String() == "esc" || msg.String() == "enter" {
				m.showHelp = false
			}
			if msg.String() == "q" || msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}
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
			if m.activeSection == 1 && m.selectedPR < len(m.filteredPRs())-1 {
				m.selectedPR++
			}
		case "home":
			if m.activeSection == 1 {
				m.selectedPR = 0
			}
		case "end":
			if m.activeSection == 1 {
				m.selectedPR = len(m.filteredPRs()) - 1
				m.clampSelectedPR()
			}
		case "?":
			m.showHelp = true
		case "/":
			if m.activeSection == 1 {
				m.searching = true
			}
		case "esc":
			if m.activeSection == 1 {
				m.query = ""
				m.filter = allPRs
				m.selectedPR = 0
			}
		case "a", "m", "d":
			if m.activeSection == 1 {
				switch msg.String() {
				case "a":
					m.filter = allPRs
				case "m":
					m.filter = minePRs
				case "d":
					m.filter = draftPRs
				}
				m.selectedPR = 0
			}
		case "r", "R":
			if m.loading {
				break
			}
			m.loading = true
			m.apiError = ""
			m.requestID++
			return m, m.loadDashboard()
		}
	}

	return m, nil
}

func (m model) filteredPRs() []githubapi.PullRequest {
	query := strings.ToLower(strings.TrimSpace(m.query))
	filtered := make([]githubapi.PullRequest, 0, len(m.pullRequests))
	for _, pr := range m.pullRequests {
		if m.filter == minePRs && (m.viewer.Login == "" || !strings.EqualFold(pr.User.Login, m.viewer.Login)) {
			continue
		}
		if m.filter == draftPRs && !pr.Draft {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(pr.Repository+" "+pr.Title+" "+pr.User.Login+" "+fmt.Sprint(pr.Number)), query) {
			continue
		}
		filtered = append(filtered, pr)
	}
	return filtered
}

func (m model) selectedKey() string {
	items := m.filteredPRs()
	if m.selectedPR >= 0 && m.selectedPR < len(items) {
		return fmt.Sprintf("%s#%d", items[m.selectedPR].Repository, items[m.selectedPR].Number)
	}
	return ""
}

func (m *model) restoreSelection(key string) {
	for i, pr := range m.filteredPRs() {
		if fmt.Sprintf("%s#%d", pr.Repository, pr.Number) == key {
			m.selectedPR = i
			return
		}
	}
	m.clampSelectedPR()
}

func dropLastRune(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return ""
	}
	return string(runes[:len(runes)-1])
}

func (m model) View() string {
	width := max(1, m.width)

	tabs := make([]string, 0, len(sections))
	if width < 70 {
		tabs = append(tabs, activeTabStyle.Render(sections[m.activeSection]))
	} else {
		for i, section := range sections {
			style := inactiveTabStyle
			if i == m.activeSection {
				style = activeTabStyle
			}
			tabs = append(tabs, style.Render(section))
		}
	}

	help := "←/→ sections • r refresh • ? help • q quit"
	if m.activeSection == 1 {
		help = "↑/↓ select • / search • a/m/d filter • ? help • q quit"
	}
	if m.searching {
		help = "Type to search • enter/esc finish • backspace delete"
	}
	header := titleStyle.Render("glance") + "  " + mutedStyle.Render("/ pull request dashboard")
	if width < 38 {
		header = titleStyle.Render("glance")
	}
	content := m.sectionContent()
	if m.showHelp {
		content = "Keyboard shortcuts\n\n←/→, h/l, tab  Sections\n↑/↓, j/k        Select PR\nHome/End        First/last PR\n/               Search\na/m/d           All/mine/drafts\nesc             Clear filters\nr               Refresh\n?               Close help\nq               Quit"
		if m.height < 18 {
			content = "Shortcuts\n←/→ sections  ↑/↓ select\n/ search  a/m/d filters\nr refresh  ? close  q quit"
		}
	}
	if m.showHelp || m.activeSection != 1 {
		lines := strings.Split(content, "\n")
		for i := range lines {
			lines[i] = lipgloss.NewStyle().Width(max(1, width-10)).Render(lines[i])
		}
		content = strings.Join(lines, "\n")
	}
	// Width includes the rounded border and the panel's horizontal padding.
	panel := panelStyle.Width(max(1, width-8)).Render(content)
	lines := []string{header, lipgloss.JoinHorizontal(lipgloss.Top, tabs...), panel, mutedStyle.Render(clipLine(help, width))}
	return strings.Join(lines, "\n")
}

func (m model) sectionContent() string {
	switch m.activeSection {
	case 0:
		content := "Welcome to glance\n\n"
		switch {
		case m.loading:
			return content + "Connecting to GitHub and loading pull requests..."
		case m.apiError != "":
			if m.viewer.Login == "" {
				return content + m.apiError
			}
			return content + "Connected as @" + m.viewer.Login + ".\nCould not load pull requests: " + m.apiError
		case m.viewer.Login != "":
			mine, drafts := 0, 0
			for _, pr := range m.pullRequests {
				if strings.EqualFold(pr.User.Login, m.viewer.Login) {
					mine++
				}
				if pr.Draft {
					drafts++
				}
			}
			return fmt.Sprintf("%sConnected as @%s.\n\nOpen pull requests: %d\nAuthored by you: %d\nDrafts: %d\n\nUse → to browse pull requests.", content, m.viewer.Login, len(m.pullRequests), mine, drafts)
		default:
			return content + "Authenticate with `gh auth login` to connect to GitHub."
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
	case m.loading && len(m.pullRequests) == 0:
		return "Loading open pull requests across your repositories..."
	case m.apiError != "":
		return "Could not load open pull requests.\n\n" + m.apiError + "\n\nPress r to try again."
	case len(m.pullRequests) == 0:
		return "No open pull requests found across repositories visible to your account.\n\nPress r to refresh."
	}

	items := m.filteredPRs()
	label := "All"
	switch m.filter {
	case minePRs:
		label = "Mine"
	case draftPRs:
		label = "Drafts"
	}
	header := fmt.Sprintf("Open pull requests (%d)  •  %s (%d)", len(m.pullRequests), label, len(items))
	if m.loading {
		header += "  •  Refreshing..."
	}
	header = clipLine(header, max(1, m.width-10))
	if m.query != "" || m.searching {
		searchLine := "Search: " + safeText(m.query)
		if m.searching {
			searchLine += "▌"
		}
		header += "\n" + clipLine(searchLine, max(1, m.width-10))
	}
	if len(items) == 0 {
		return header + "\n\nNo matching pull requests. Press esc to clear filters."
	}
	visibleCount := min(m.visiblePRCount(), len(items))
	start := m.selectedPR - visibleCount/2
	if start < 0 {
		start = 0
	}
	if maxStart := len(items) - visibleCount; start > maxStart {
		start = maxStart
	}
	end := min(start+visibleCount, len(items))
	lines := []string{header}
	for index := start; index < end; index++ {
		pullRequest := items[index]
		marker := "  "
		if index == m.selectedPR {
			marker = "› "
		}
		state := "OPEN"
		if pullRequest.Draft {
			state = "DRAFT"
		}
		title := safeText(pullRequest.Title)
		firstLine := fmt.Sprintf("%s[%s] %s#%d  %s", marker, state, safeText(pullRequest.Repository), pullRequest.Number, title)
		line := clipLine(firstLine, max(1, m.width-10))
		meta := clipLine(fmt.Sprintf("    @%s • updated %s", safeText(pullRequest.User.Login), relativeTime(pullRequest.UpdatedAt)), max(1, m.width-10))
		if index == m.selectedPR {
			line = selectedPRStyle.Render(line)
		} else if pullRequest.Draft {
			line = draftStyle.Render(line)
		} else {
			line = statusStyle.Render(line)
		}
		lines = append(lines, line, mutedStyle.Render(meta))
	}
	lines = append(lines, fmt.Sprintf("Showing %d-%d of %d", start+1, end, len(items)))
	return strings.Join(lines, "\n")
}

func (m model) visiblePRCount() int {
	reserved := 10 // header, tabs, panel borders/padding, list header and footer
	if m.query != "" || m.searching {
		reserved++
	}
	return max(1, (m.height-reserved)/2)
}

func safeText(text string) string {
	return strings.Join(strings.FieldsFunc(text, func(r rune) bool {
		return unicode.IsControl(r)
	}), " ")
}

func clipLine(line string, width int) string {
	if lipgloss.Width(line) <= width {
		return line
	}

	var truncated strings.Builder
	used := 0
	for _, char := range line {
		charWidth := lipgloss.Width(string(char))
		if used+charWidth > width-1 {
			break
		}
		truncated.WriteRune(char)
		used += charWidth
	}
	if width < 2 {
		return "…"
	}
	return truncated.String() + "…"
}

func (m *model) clampSelectedPR() {
	if m.selectedPR < 0 {
		m.selectedPR = 0
	}
	if m.selectedPR >= len(m.filteredPRs()) {
		m.selectedPR = max(0, len(m.filteredPRs())-1)
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
