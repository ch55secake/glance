package main

import (
	"fmt"
	"strings"

	"glance/internal/githubapi"

	"github.com/charmbracelet/lipgloss"
)

type overviewKind int

const (
	overviewPR overviewKind = iota
	overviewNotification
	overviewIssue
)

type overviewItem struct {
	kind    overviewKind
	index   int
	key     string
	pr      githubapi.PullRequest
	label   string
	source  string
	title   string
	updated string
}

// overviewItems is a short, prioritized queue from the already-loaded inboxes.
// Keep the original indices so Enter can land on the corresponding inbox item.
func (m model) overviewItems() []overviewItem {
	items := make([]overviewItem, 0, 6)
	unread, prs := 0, 0
	addNotification := func(i int, notification githubapi.Notification) {
		label := "UNREAD"
		if notification.Reason == "review_requested" {
			label = "REVIEW"
		}
		items = append(items, overviewItem{
			kind: overviewNotification, index: i, key: "notification:" + notification.ID, label: label,
			source: notification.Repository.FullName, title: notification.Subject.Title,
			updated: relativeTime(notification.UpdatedAt),
		})
		unread++
	}
	addPR := func(pr githubapi.PullRequest, label string) {
		items = append(items, overviewItem{
			kind: overviewPR, pr: pr, key: fmt.Sprintf("pr:%s#%d", pr.Repository, pr.Number), label: label,
			source: fmt.Sprintf("%s#%d", pr.Repository, pr.Number), title: pr.Title,
			updated: relativeTime(pr.UpdatedAt),
		})
		prs++
	}
	for i, notification := range m.notifications {
		if notification.Unread && notification.Reason == "review_requested" && unread < 2 {
			addNotification(i, notification)
		}
	}
	for _, pr := range m.pullRequests {
		if prs >= 2 {
			break
		}
		if !pr.Draft && strings.EqualFold(pr.User.Login, m.viewer.Login) {
			addPR(pr, "YOURS")
		}
	}
	for i, notification := range m.notifications {
		if unread >= 2 {
			break
		}
		if notification.Unread && notification.Reason != "review_requested" {
			addNotification(i, notification)
		}
	}
	for _, pr := range m.pullRequests {
		if prs >= 3 {
			break
		}
		if !pr.Draft && !strings.EqualFold(pr.User.Login, m.viewer.Login) {
			addPR(pr, "OPEN")
		}
	}
	for _, pr := range m.pullRequests {
		if prs >= 3 {
			break
		}
		if pr.Draft {
			addPR(pr, "DRAFT")
		}
	}
	if len(m.issues) > 0 {
		issue := m.issues[0]
		items = append(items, overviewItem{
			kind: overviewIssue, index: 0, key: fmt.Sprintf("issue:%s#%d", issue.Repository, issue.Number), label: "ISSUE",
			source: fmt.Sprintf("%s#%d", issue.Repository, issue.Number), title: issue.Title,
			updated: relativeTime(issue.UpdatedAt),
		})
	}
	return items
}

func (m model) selectedOverviewKey() string {
	items := m.overviewItems()
	if m.overviewSelection < 0 || m.overviewSelection >= len(items) {
		return ""
	}
	return items[m.overviewSelection].key
}

func (m *model) restoreOverviewSelection(key string) {
	items := m.overviewItems()
	for i, item := range items {
		if key != "" && item.key == key {
			m.overviewSelection = i
			return
		}
	}
	m.overviewSelection = min(m.overviewSelection, max(0, len(items)-1))
}

func (m model) overviewView(width int) []string {
	lines := []string{"  " + accentStyle.Render("◈ OVERVIEW")}
	if m.loading && m.viewer.Login == "" {
		return append(lines, "  "+mutedStyle.Render(clipLine("Welcome to glance  ·  Connecting to GitHub and loading pull requests...", width)))
	}
	if m.viewer.Login == "" {
		detail, title := m.apiError, "Connection problem"
		if detail == "" {
			detail, title = "Authenticate with `gh auth login` to connect to GitHub.", "Not connected"
		}
		return append(lines, m.message(width, title, detail, "Press r to try again.")...)
	}
	mine, unread := 0, 0
	for _, pr := range m.pullRequests {
		if strings.EqualFold(pr.User.Login, m.viewer.Login) {
			mine++
		}
	}
	for _, notification := range m.notifications {
		if notification.Unread {
			unread++
		}
	}
	lines = append(lines, "  "+textStyle.Render(clipLine("Connected as @"+safeText(m.viewer.Login), width)))
	if width >= 68 {
		cards := []string{
			statCard("PRS", len(m.pullRequests), goodStyle, "#214D42"),
			statCard("YOURS", mine, accentStyle, "#443460"),
			statCard("UNREAD", unread, warnStyle, "#61492A"),
			statCard("ISSUES", len(m.issues), goodStyle, "#214D42"),
		}
		row := strings.Join(cards, "  ")
		if lipgloss.Width(row) <= width {
			lines = append(lines, "  "+row)
		} else {
			lines = append(lines,
				"  "+mutedStyle.Render(clipLine(fmt.Sprintf("%d PRs · %d yours", len(m.pullRequests), mine), width)),
				"  "+mutedStyle.Render(clipLine(fmt.Sprintf("%d unread · %d issues", unread, len(m.issues)), width)),
			)
		}
	} else {
		lines = append(lines,
			"  "+mutedStyle.Render(clipLine(fmt.Sprintf("%d PRs · %d yours", len(m.pullRequests), mine), width)),
			"  "+mutedStyle.Render(clipLine(fmt.Sprintf("%d unread · %d issues", unread, len(m.issues)), width)),
		)
	}
	if m.hasErrors() {
		var failures []string
		for _, item := range []struct{ name, err string }{{"PRs", m.apiError}, {"issues", m.issueError}, {"notifications", m.notificationError}} {
			if item.err != "" {
				failures = append(failures, item.name)
			}
		}
		lines = append(lines, "  "+errorStyle.Render(clipLine("⚠ Could not refresh "+strings.Join(failures, ", ")+" · r retry", width)))
	}
	lines = append(lines, "", "  "+accentStyle.Render("▸ NEXT UP"))
	items := m.overviewItems()
	if len(items) == 0 {
		message := "Nothing needs attention. Browse an inbox with ←/→."
		if m.hasErrors() {
			message = "Inbox data unavailable; press r to retry."
		}
		return append(lines, "  "+mutedStyle.Render(clipLine(message, width)))
	}
	// Four header rows and two footer rows belong to the dashboard shell.
	visible := min(len(items), max(1, (m.height-7-len(lines))/2))
	selected := min(m.overviewSelection, len(items)-1)
	start := min(max(0, selected-visible/2), len(items)-visible)
	for i := start; i < start+visible; i++ {
		lines = append(lines, overviewRow(items[i], width, i == selected)...)
	}
	lines = append(lines, "  "+mutedStyle.Render(clipLine(fmt.Sprintf("%d/%d selected  ·  ↑/↓ move  ·  enter open", selected+1, len(items)), width)))
	return lines
}

func overviewRow(item overviewItem, width int, selected bool) []string {
	marker, titleMarker := "  ", "  "
	if selected {
		marker, titleMarker = "▎ ", "▸ "
	}
	context := safeText(item.source)
	label := safeText(item.label) + "  ·  " + context
	if width >= 60 {
		age := safeText(item.updated)
		label = align(clipLine(label, max(1, width-3-lipgloss.Width(age))), age, width-2)
	} else {
		label = clipLine(label, max(1, width-2))
	}
	first := marker + label
	title := safeText(item.title)
	if title == "" {
		title = "(Untitled)"
	}
	second := titleMarker + clipLine(title, max(1, width-2))
	if selected {
		return []string{
			"  " + selectedStyle.Width(width).Render(first),
			"  " + selectedStyle.Bold(true).Width(width).Render(second),
		}
	}
	return []string{"  " + mutedStyle.Render(first), "  " + textStyle.Bold(true).Render(second)}
}
