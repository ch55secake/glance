package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"

	"glance/internal/githubapi"

	tea "github.com/charmbracelet/bubbletea"
)

var sections = []string{"Overview", "Pull requests", "Issues", "Notifications"}

const dashboardLoadTimeout = 2 * time.Minute

type model struct {
	width                int
	height               int
	activeSection        int
	loading              bool
	viewer               githubapi.User
	apiError             string
	pullRequests         []githubapi.PullRequest
	selectedPR           int
	issues               []githubapi.Issue
	issueError           string
	selectedIssue        int
	notifications        []githubapi.Notification
	notificationError    string
	selectedNotification int
	filter               prFilter
	query                string
	searching            bool
	showHelp             bool
	requestID            int
	client               *githubapi.Client
}

type dashboardLoadedMsg struct {
	requestID       int
	viewer          githubapi.User
	pullRequests    []githubapi.PullRequest
	issues          []githubapi.Issue
	notifications   []githubapi.Notification
	err             error
	pullRequestErr  error
	issueErr        error
	notificationErr error
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
		result := dashboardLoadedMsg{requestID: m.requestID, viewer: viewer}
		var requests sync.WaitGroup
		requests.Add(3)
		go func() {
			defer requests.Done()
			result.pullRequests, result.pullRequestErr = m.client.ListOpenPullRequests(ctx)
		}()
		go func() {
			defer requests.Done()
			result.issues, result.issueErr = m.client.ListOpenIssues(ctx)
		}()
		go func() {
			defer requests.Done()
			result.notifications, result.notificationErr = m.client.ListNotifications(ctx)
		}()
		requests.Wait()
		return result
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
		selectedPR := m.selectedKey()
		selectedIssue := m.selectedIssueKey()
		selectedNotification := m.selectedNotificationID()
		m.loading = false
		if msg.err != nil {
			m.apiError = safeText(msg.err.Error())
			m.issueError = m.apiError
			m.notificationError = m.apiError
		} else {
			m.viewer = msg.viewer
			m.apiError = errorString(msg.pullRequestErr)
			m.issueError = errorString(msg.issueErr)
			m.notificationError = errorString(msg.notificationErr)
			if msg.pullRequestErr == nil {
				m.pullRequests = msg.pullRequests
			}
			if msg.issueErr == nil {
				m.issues = msg.issues
			}
			if msg.notificationErr == nil {
				m.notifications = msg.notifications
			}
		}
		m.restoreSelection(selectedPR)
		m.restoreIssueSelection(selectedIssue)
		m.restoreNotificationSelection(selectedNotification)
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
			m.moveSelection(-1)
		case "down", "j":
			m.moveSelection(1)
		case "home":
			m.setSelection(0)
		case "end":
			m.setSelection(m.visibleItemTotal() - 1)
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
			m.issueError = ""
			m.notificationError = ""
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

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return safeText(err.Error())
}

func (m model) visibleItemTotal() int {
	switch m.activeSection {
	case 1:
		return len(m.filteredPRs())
	case 2:
		return len(m.issues)
	case 3:
		return len(m.notifications)
	default:
		return 0
	}
}

func (m *model) moveSelection(delta int) {
	switch m.activeSection {
	case 1:
		m.selectedPR += delta
		m.clampSelectedPR()
	case 2:
		m.selectedIssue = min(max(0, m.selectedIssue+delta), max(0, len(m.issues)-1))
	case 3:
		m.selectedNotification = min(max(0, m.selectedNotification+delta), max(0, len(m.notifications)-1))
	}
}

func (m *model) setSelection(index int) {
	switch m.activeSection {
	case 1:
		m.selectedPR = index
		m.clampSelectedPR()
	case 2:
		m.selectedIssue = min(max(0, index), max(0, len(m.issues)-1))
	case 3:
		m.selectedNotification = min(max(0, index), max(0, len(m.notifications)-1))
	}
}

func (m model) selectedKey() string {
	items := m.filteredPRs()
	if m.selectedPR >= 0 && m.selectedPR < len(items) {
		return fmt.Sprintf("%s#%d", items[m.selectedPR].Repository, items[m.selectedPR].Number)
	}
	return ""
}

func (m model) selectedIssueKey() string {
	if m.selectedIssue >= 0 && m.selectedIssue < len(m.issues) {
		issue := m.issues[m.selectedIssue]
		return fmt.Sprintf("%s#%d", issue.Repository, issue.Number)
	}
	return ""
}

func (m model) selectedNotificationID() string {
	if m.selectedNotification >= 0 && m.selectedNotification < len(m.notifications) {
		return m.notifications[m.selectedNotification].ID
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

func (m *model) restoreIssueSelection(key string) {
	for i, issue := range m.issues {
		if fmt.Sprintf("%s#%d", issue.Repository, issue.Number) == key {
			m.selectedIssue = i
			return
		}
	}
	m.selectedIssue = min(max(0, m.selectedIssue), max(0, len(m.issues)-1))
}

func (m *model) restoreNotificationSelection(id string) {
	for i, notification := range m.notifications {
		if notification.ID == id {
			m.selectedNotification = i
			return
		}
	}
	m.selectedNotification = min(max(0, m.selectedNotification), max(0, len(m.notifications)-1))
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
