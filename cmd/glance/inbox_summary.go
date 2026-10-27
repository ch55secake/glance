package main

import (
	"fmt"
	"strings"
	"unicode"

	"glance/internal/githubapi"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const reviewRequestedQuery = "is:pr is:open review-requested:@me"
const allOpenQuery = "is:pr is:open"

func ownedRepositoriesPRQuery(login string) (string, bool) {
	if len(login) == 0 || len(login) > 39 {
		return "", false
	}
	for i, char := range login {
		valid := unicode.IsLetter(char) && char <= unicode.MaxASCII || unicode.IsDigit(char) && char <= unicode.MaxASCII || char == '-'
		if !valid || (char == '-' && (i == 0 || i == len(login)-1)) {
			return "", false
		}
	}
	return "is:pr is:open user:" + login, true
}

func checkSummary(details githubapi.PullRequestDetails) string {
	text, _ := checkSummaryStatus(details)
	return text
}

func checkSummaryStatus(details githubapi.PullRequestDetails) (string, lipgloss.Style) {
	if details.ChecksError != "" {
		return "? Checks unavailable", errorStyle
	}
	if len(details.CheckRuns) == 0 {
		return "No checks reported", mutedStyle
	}
	counts := countCheckStatuses(details.CheckRuns)
	labels := []string{"✓ PASS", "✕ FAIL", "! CANCEL", "◌ RUNNING", "– SKIP", "? UNKNOWN"}
	parts := []string{"checks"}
	for _, label := range labels {
		if counts[label] == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s%d", strings.Fields(label)[0], counts[label]))
	}
	text := strings.Join(parts, " ")
	style := goodStyle
	if counts["✕ FAIL"] > 0 {
		style = errorStyle
	} else if counts["! CANCEL"]+counts["◌ RUNNING"] > 0 {
		style = warnStyle
	}
	return text, style
}

func (m model) pullRequestToolbar(width int) []string {
	box := m.githubFilterBox(width)
	lines := make([]string, len(box))
	for i, line := range box {
		lines[i] = "  " + line
	}
	return lines
}

func (m *model) applyPRQuery(query string) tea.Cmd {
	m.switchSection(0)
	m.searching = false
	m.searchDraft = ""
	m.searchCursor, m.searchOffset = 0, 0
	if query == m.query && m.apiError == "" {
		return m.schedulePRPreview()
	}
	m.query = query
	m.prRequestID++
	m.prLoading = true
	m.apiError = ""
	m.inboxPreviewFocus = false
	m.inboxPreviewScroll = 0
	m.clearPRPreviews()
	return m.searchPullRequests()
}
