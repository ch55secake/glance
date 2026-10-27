package main

import (
	"fmt"
	"strings"

	"glance/internal/githubapi"

	"github.com/charmbracelet/lipgloss"
)

// Wide inboxes are a compact selection rail beside a full-width preview.
type workspaceItem struct {
	title   string
	context string
	meta    string
}

func (m model) pullRequestWorkspace(items []githubapi.PullRequest, width int) []string {
	selected := min(max(0, m.selectedPR), len(items)-1)
	railWidth, _ := workspaceWidths(width)
	railTextWidth := railWidth - 6
	rail := make([]workspaceItem, 0, len(items))
	for _, pr := range items {
		state := ""
		if pr.Draft {
			state = "  ◆"
		}
		meta := "@" + safeText(pr.User.Login) + " · " + relativeTime(pr.UpdatedAt)
		if details, ok := m.previewCache[fmt.Sprintf("%s#%d", pr.Repository, pr.Number)]; ok {
			meta = checkSummary(details) + " · " + relativeTime(pr.UpdatedAt)
		}
		rail = append(rail, workspaceItem{safeText(pr.Title), workspaceReference(pr.Repository, pr.Number, railTextWidth-lipgloss.Width(state)) + state, meta})
	}
	return m.workspace(width, rail, selected)
}

func (m model) pullRequestPreview(pr githubapi.PullRequest, width int) []string {
	state := openBadge.Render(" ● OPEN ")
	if pr.Draft {
		state = draftBadge.Render(" ◆ DRAFT ")
	}
	lines := previewTitle(pr.Title, width)
	lines = append(lines, state+" "+mutedStyle.Render(clipLine(workspaceReference(pr.Repository, pr.Number, width-10), width-10)),
		textStyle.Render(clipLine("By @"+safeText(pr.User.Login), width)),
		mutedStyle.Render(clipLine("Updated "+relativeTime(pr.UpdatedAt), width)))
	return append(lines, m.prPreviewContext(fmt.Sprintf("%s#%d", pr.Repository, pr.Number), width)...)
}

func (m model) prPreviewContext(key string, width int) []string {
	details, ok := m.previewCache[key]
	if !ok {
		if problem := m.previewErrors[key]; problem != "" {
			return []string{"", warnStyle.Render(clipLine("Context unavailable · r to retry", width)), mutedStyle.Render(clipLine(problem, width))}
		}
		status := "Loading review context…"
		if !m.previewPending[key] {
			status = "Review context will load shortly…"
		}
		return []string{"", mutedStyle.Render(clipLine(status, width))}
	}
	lines := []string{""}
	mergeability := "Mergeability pending"
	style := warnStyle
	if details.Mergeable != nil {
		if *details.Mergeable {
			mergeability = "✓ Mergeable"
			style = goodStyle
		} else {
			mergeability = "✕ Not mergeable"
			style = errorStyle
		}
	}
	lines = append(lines, style.Render(clipLine(mergeability, width)), textStyle.Render(clipLine(checkSummary(details), width)))
	status := fmt.Sprintf("Reviews  %d  ·  requested  %d", len(details.Reviews), len(details.RequestedReviewers))
	if details.ReviewsError != "" {
		status = "Reviews unavailable"
	}
	lines = append(lines, mutedStyle.Render(clipLine(status, width)))
	if details.Head.Ref != "" && details.Base.Ref != "" {
		lines = append(lines, mutedStyle.Render(clipLine(safeText(details.Head.Ref)+" → "+safeText(details.Base.Ref), width)))
	}
	lines = append(lines, "", accentStyle.Render("DESCRIPTION"))
	body := previewBody(details.Body)
	if strings.TrimSpace(body) == "" {
		body = "No description provided."
	}
	lines = append(lines, previewMarkdown(body, width)...)
	if len(details.CheckRuns) > 0 {
		lines = append(lines, "", accentStyle.Render("CHECKS"))
		for _, check := range details.CheckRuns {
			label, style := checkStyle(check)
			for _, part := range wrapPreviewWords(label+"  "+safeText(check.Name), width) {
				lines = append(lines, style.Render(part))
			}
		}
	}
	if len(details.RequestedReviewers)+len(details.Reviews) > 0 {
		lines = append(lines, "", accentStyle.Render("REVIEW ACTIVITY"))
		for _, reviewer := range details.RequestedReviewers {
			for _, part := range wrapPreviewWords("Requested @"+safeText(reviewer.Login), width) {
				lines = append(lines, mutedStyle.Render(part))
			}
		}
		for _, review := range details.Reviews {
			for _, part := range wrapPreviewWords("@"+safeText(review.User.Login)+"  "+safeText(review.State), width) {
				lines = append(lines, textStyle.Render(part))
			}
		}
	}
	return lines
}

func wrapPreviewWords(text string, width int) []string {
	var lines []string
	current := ""
	for _, word := range strings.Fields(text) {
		if lipgloss.Width(word) > width {
			if current != "" {
				lines = append(lines, current)
			}
			parts := wrapDiffLine(word, width)
			lines = append(lines, parts[:len(parts)-1]...)
			current = parts[len(parts)-1]
			continue
		}
		if current != "" && lipgloss.Width(current+" "+word) > width {
			lines = append(lines, current)
			current = ""
		}
		if current != "" {
			current += " "
		}
		current += word
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func (m model) issueWorkspace(width int) []string {
	selected := min(max(0, m.selectedIssue), len(m.issues)-1)
	railWidth, _ := workspaceWidths(width)
	railTextWidth := railWidth - 6
	rail := make([]workspaceItem, 0, len(m.issues))
	for _, issue := range m.issues {
		rail = append(rail, workspaceItem{safeText(issue.Title), workspaceReference(issue.Repository, issue.Number, railTextWidth), fmt.Sprintf("%d comments · %s", issue.Comments, relativeTime(issue.UpdatedAt))})
	}
	return m.workspace(width, rail, selected)
}

func issuePreview(issue githubapi.Issue, width int) []string {
	lines := previewTitle(issue.Title, width)
	lines = append(lines, openBadge.Render(" ● OPEN ")+" "+mutedStyle.Render(clipLine(workspaceReference(issue.Repository, issue.Number, width-10), width-10)),
		textStyle.Render(clipLine("By @"+safeText(issue.User.Login), width)),
		mutedStyle.Render(clipLine(fmt.Sprintf("%d comments  ·  updated %s", issue.Comments, relativeTime(issue.UpdatedAt)), width)))
	if len(issue.Labels) > 0 {
		labels := make([]string, 0, len(issue.Labels))
		for _, label := range issue.Labels {
			labels = append(labels, safeText(label.Name))
		}
		for _, part := range wrapPreviewWords("Labels  "+strings.Join(labels, ", "), width) {
			lines = append(lines, mutedStyle.Render(part))
		}
	}
	lines = append(lines, "", accentStyle.Render("DESCRIPTION"))
	body := previewBody(issue.Body)
	if strings.TrimSpace(body) == "" {
		body = "No description provided."
	}
	lines = append(lines, previewMarkdown(body, width)...)
	return lines
}

func (m model) notificationWorkspace(width int) []string {
	selected := min(max(0, m.selectedNotification), len(m.notifications)-1)
	rail := make([]workspaceItem, 0, len(m.notifications))
	for _, notification := range m.notifications {
		marker := ""
		if notification.Unread {
			marker = "● "
		}
		rail = append(rail, workspaceItem{marker + safeText(notification.Subject.Title), safeText(notification.Repository.FullName), notificationReason(notification.Reason)})
	}
	return m.workspace(width, rail, selected)
}

func notificationPreview(notification githubapi.Notification, width int) []string {
	state := mutedStyle.Render("○ READ")
	if notification.Unread {
		state = unreadBadge.Render(" ● UNREAD ")
	}
	lines := previewTitle(notification.Subject.Title, width)
	lines = append(lines, state,
		mutedStyle.Render(clipLine(safeText(notification.Repository.FullName)+"  ·  "+safeText(notification.Subject.Type), width)),
		mutedStyle.Render(clipLine("Updated "+relativeTime(notification.UpdatedAt), width)))
	if notification.LastReadAt != nil {
		lines = append(lines, mutedStyle.Render(clipLine("Last read "+relativeTime(*notification.LastReadAt), width)))
	}
	return append(lines, "", accentStyle.Render("WHY THIS IS IN YOUR INBOX"),
		textStyle.Render(clipLine(notificationReason(notification.Reason), width)),
		mutedStyle.Render(clipLine("Reason: "+safeText(notification.Reason), width)))
}

func notificationReason(reason string) string {
	switch reason {
	case "review_requested":
		return "Your review was requested"
	case "mention", "team_mention":
		return "You or your team were mentioned"
	case "assign":
		return "You were assigned"
	case "author":
		return "You created this thread"
	case "comment":
		return "A new comment was posted"
	case "ci_activity":
		return "A check or workflow was updated"
	default:
		return safeText(reason)
	}
}

func previewTitle(title string, width int) []string {
	parts := wrapPreviewWords(safeText(title), width)
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		lines = append(lines, textStyle.Bold(true).Render(part))
	}
	return lines
}

func previewBody(body string) string {
	return safeMultiline(strings.ReplaceAll(body, "\r\n", "\n"))
}

func workspaceReference(repository string, number, width int) string {
	label := fmt.Sprintf("%s#%d", safeText(repository), number)
	if lipgloss.Width(label) > width {
		return fmt.Sprintf("#%d  %s", number, safeText(repository))
	}
	return label
}

func (m model) workspace(width int, items []workspaceItem, selected int) []string {
	height := m.workspaceHeight()
	railWidth, previewWidth := workspaceWidths(width)
	visible := min(len(items), m.inboxPageSize())
	start := min(max(0, selected-visible/2), len(items)-visible)
	preview := m.inboxPreviewLines(previewWidth)
	previewStart := min(max(0, m.inboxPreviewScroll), max(0, len(preview)-m.inboxPreviewHeight()))
	var rail []string
	for i := start; i < start+visible; i++ {
		item := items[i]
		values := []string{item.title, item.context, item.meta}
		for row, value := range values[:m.inboxItemHeight()] {
			marker, style := "  ", mutedStyle
			if row == 0 {
				style = textStyle.Bold(true)
			}
			if i == selected {
				marker = "▎ "
				if row == 0 {
					marker = "▸ "
				}
				style = selectedStyle.Bold(row == 0)
			}
			rail = append(rail, style.Width(railWidth-4).Render(marker+clipLine(value, railWidth-6)))
		}
	}
	left := panel(railWidth, height, fmt.Sprintf("INBOX  %d–%d", start+1, start+visible), fmt.Sprintf("%d of %d · ↑/↓ browse", selected+1, len(items)), rail, !m.inboxPreviewFocus && !m.searching)
	hint := "tab preview"
	if m.inboxPreviewFocus {
		hint = "esc list"
	}
	status := hint + " · " + scrollPosition(previewStart, m.inboxPreviewHeight(), len(preview))
	right := panel(previewWidth+4, height, "PREVIEW", status,
		preview[previewStart:min(len(preview), previewStart+m.inboxPreviewHeight())], m.inboxPreviewFocus && !m.searching)
	lines := make([]string, height)
	for row := range lines {
		lines[row] = "  " + left[row] + "  " + right[row]
	}
	return lines
}
