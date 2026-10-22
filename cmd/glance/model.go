package main

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"glance/internal/githubapi"

	tea "github.com/charmbracelet/bubbletea"
)

var sections = []string{"Overview", "Pull requests", "Issues", "Notifications"}

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

func safeText(text string) string {
	return strings.Join(strings.FieldsFunc(text, func(r rune) bool {
		return unicode.IsControl(r)
	}), " ")
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
