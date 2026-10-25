package main

import (
	"fmt"

	"glance/internal/githubapi"

	tea "github.com/charmbracelet/bubbletea"
)

const reviewRequestedQuery = "is:pr is:open review-requested:@me"
const allOpenQuery = "is:pr is:open"

func checkSummary(details githubapi.PullRequestDetails) string {
	if details.ChecksError != "" {
		return "? Checks unavailable"
	}
	pass, fail, running := 0, 0, 0
	for _, check := range details.CheckRuns {
		switch check.Conclusion {
		case "success", "neutral", "skipped":
			pass++
		case "failure", "timed_out", "action_required", "cancelled", "startup_failure", "stale":
			fail++
		default:
			running++
		}
	}
	if len(details.CheckRuns) == 0 {
		return "No checks reported"
	}
	return fmt.Sprintf("checks ✓%d ✕%d ◌%d", pass, fail, running)
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
