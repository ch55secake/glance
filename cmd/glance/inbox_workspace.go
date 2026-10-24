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
}

func (m model) pullRequestWorkspace(items []githubapi.PullRequest, width, headingRows int) []string {
	selected := min(max(0, m.selectedPR), len(items)-1)
	railTextWidth := min(38, max(26, width/3)) - 2
	rail := make([]workspaceItem, 0, len(items))
	for _, pr := range items {
		state := ""
		if pr.Draft {
			state = "  ◆"
		}
		rail = append(rail, workspaceItem{safeText(pr.Title), workspaceReference(pr.Repository, pr.Number, railTextWidth-lipgloss.Width(state)) + state})
	}
	pr := items[selected]
	return m.workspace(width, headingRows, rail, selected, "↵ open review & diff", func(w int) []string {
		state := openBadge.Render(" ● OPEN ")
		if pr.Draft {
			state = draftBadge.Render(" ◆ DRAFT ")
		}
		lines := previewTitle(pr.Title, w)
		lines = append(lines, "", state,
			mutedStyle.Render(clipLine(workspaceReference(pr.Repository, pr.Number, w), w)),
			"", textStyle.Render(clipLine("By @"+safeText(pr.User.Login), w)),
			mutedStyle.Render(clipLine("Updated "+relativeTime(pr.UpdatedAt), w)))
		return append(lines, m.prPreviewContext(fmt.Sprintf("%s#%d", pr.Repository, pr.Number), w, max(0, m.height-3-headingRows-2-len(lines)-4))...)
	})
}

func (m model) prPreviewContext(key string, width, available int) []string {
	if available < 1 {
		return nil
	}
	details, ok := m.previewCache[key]
	if !ok {
		if problem := m.previewErrors[key]; problem != "" {
			return []string{"", warnStyle.Render("Context unavailable · r to retry"), mutedStyle.Render(clipLine(problem, width))}
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
	pass, fail, running := 0, 0, 0
	for _, check := range details.CheckRuns {
		switch check.Conclusion {
		case "success", "neutral", "skipped":
			pass++
		case "failure", "timed_out", "action_required":
			fail++
		default:
			running++
		}
	}
	checks := fmt.Sprintf("checks ✓%d ✕%d ◌%d", pass, fail, running)
	if details.ChecksError != "" {
		checks = "checks unavailable"
	}
	lines = append(lines, style.Render(clipLine(mergeability+"  ·  "+checks, width)))
	if len(lines) < available {
		status := fmt.Sprintf("Reviews  %d  ·  requested  %d", len(details.Reviews), len(details.RequestedReviewers))
		if details.ReviewsError != "" {
			status = "Reviews unavailable"
		}
		lines = append(lines, mutedStyle.Render(clipLine(status, width)))
	}
	if details.Head.Ref != "" && details.Base.Ref != "" && available >= 9 {
		lines = append(lines, mutedStyle.Render(clipLine(safeText(details.Head.Ref)+" → "+safeText(details.Base.Ref), width)))
	}
	if len(lines)+1 < available {
		if available-len(lines) >= 4 {
			lines = append(lines, "")
		}
		lines = append(lines, accentStyle.Render("DESCRIPTION"))
		body := previewExcerpt(details.Body)
		if strings.TrimSpace(body) == "" {
			body = "No description provided."
		}
		bodyLimit := available
		if available >= 12 && len(details.CheckRuns)+len(details.Reviews)+len(details.RequestedReviewers) > 0 {
			bodyLimit -= min(6, len(details.CheckRuns)+len(details.Reviews)+len(details.RequestedReviewers)+2)
		}
		for _, part := range previewDescriptionLines(body, width) {
			if len(lines) >= bodyLimit {
				break
			}
			lines = append(lines, mutedStyle.Render(part))
		}
	}
	if len(details.CheckRuns) > 0 && len(lines)+2 < available {
		lines = append(lines, "", accentStyle.Render("CHECKS"))
		for _, check := range details.CheckRuns {
			if len(lines) >= available {
				break
			}
			label, style := checkStyle(check)
			lines = append(lines, style.Render(clipLine(label+"  "+safeText(check.Name), width)))
		}
	}
	if len(details.RequestedReviewers)+len(details.Reviews) > 0 && len(lines)+2 < available {
		lines = append(lines, "", accentStyle.Render("REVIEW ACTIVITY"))
		for _, reviewer := range details.RequestedReviewers {
			if len(lines) >= available {
				break
			}
			lines = append(lines, mutedStyle.Render(clipLine("Requested  @"+safeText(reviewer.Login), width)))
		}
		for _, review := range details.Reviews {
			if len(lines) >= available {
				break
			}
			lines = append(lines, textStyle.Render(clipLine("@"+safeText(review.User.Login)+"  "+safeText(review.State), width)))
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

// Preserve authored line breaks while wrapping each individual line to the pane.
func previewDescriptionLines(body string, width int) []string {
	var lines []string
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == "" {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, wrapPreviewWords(line, width)...)
	}
	return lines
}

func (m model) issueWorkspace(width, headingRows int) []string {
	selected := min(max(0, m.selectedIssue), len(m.issues)-1)
	railTextWidth := min(38, max(26, width/3)) - 2
	rail := make([]workspaceItem, 0, len(m.issues))
	for _, issue := range m.issues {
		rail = append(rail, workspaceItem{safeText(issue.Title), workspaceReference(issue.Repository, issue.Number, railTextWidth)})
	}
	issue := m.issues[selected]
	return m.workspace(width, headingRows, rail, selected, "↑/↓ select an issue", func(w int) []string {
		lines := previewTitle(issue.Title, w)
		lines = append(lines, "", openBadge.Render(" ● OPEN "),
			mutedStyle.Render(clipLine(workspaceReference(issue.Repository, issue.Number, w), w)),
			"", textStyle.Render(clipLine("By @"+safeText(issue.User.Login), w)),
			mutedStyle.Render(clipLine(fmt.Sprintf("%d comments  ·  updated %s", issue.Comments, relativeTime(issue.UpdatedAt)), w)))
		if len(issue.Labels) > 0 {
			labels := make([]string, 0, len(issue.Labels))
			for _, label := range issue.Labels {
				labels = append(labels, safeText(label.Name))
			}
			lines = append(lines, "", mutedStyle.Render(clipLine("Labels  "+strings.Join(labels, ", "), w)))
		}
		if strings.TrimSpace(issue.Body) != "" {
			lines = append(lines, "", accentStyle.Render("DESCRIPTION"))
			for _, part := range previewDescriptionLines(previewExcerpt(issue.Body), w) {
				lines = append(lines, mutedStyle.Render(part))
			}
		}
		return lines
	})
}

func (m model) notificationWorkspace(width, headingRows int) []string {
	selected := min(max(0, m.selectedNotification), len(m.notifications)-1)
	rail := make([]workspaceItem, 0, len(m.notifications))
	for _, notification := range m.notifications {
		marker := ""
		if notification.Unread {
			marker = "● "
		}
		rail = append(rail, workspaceItem{marker + safeText(notification.Subject.Title), safeText(notification.Repository.FullName)})
	}
	notification := m.notifications[selected]
	return m.workspace(width, headingRows, rail, selected, "↑/↓ select a thread", func(w int) []string {
		state := mutedStyle.Render("○ READ")
		if notification.Unread {
			state = unreadBadge.Render(" ● UNREAD ")
		}
		lines := previewTitle(notification.Subject.Title, w)
		lines = append(lines, "", state,
			mutedStyle.Render(clipLine(safeText(notification.Repository.FullName)+"  ·  "+safeText(notification.Subject.Type), w)),
			"", mutedStyle.Render(clipLine("Updated "+relativeTime(notification.UpdatedAt), w)))
		if notification.LastReadAt != nil {
			lines = append(lines, mutedStyle.Render(clipLine("Last read "+relativeTime(*notification.LastReadAt), w)))
		}
		return append(lines, "", accentStyle.Render("WHY THIS IS IN YOUR INBOX"),
			textStyle.Render(clipLine(notificationReason(notification.Reason), w)),
			mutedStyle.Render(clipLine("Reason: "+safeText(notification.Reason), w)))
	})
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
	parts := wrapDiffLine(safeText(title), width)
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		lines = append(lines, textStyle.Bold(true).Render(part))
	}
	return lines
}

func previewExcerpt(body string) string {
	clean := []rune(safeMultiline(strings.ReplaceAll(body, "\r\n", "\n")))
	if len(clean) > 2000 {
		return string(clean[:2000]) + "…"
	}
	return string(clean)
}

func workspaceReference(repository string, number, width int) string {
	label := fmt.Sprintf("%s#%d", safeText(repository), number)
	if lipgloss.Width(label) > width {
		return fmt.Sprintf("#%d  %s", number, safeText(repository))
	}
	return label
}

func (m model) workspace(width, headingRows int, items []workspaceItem, selected int, action string, preview func(int) []string) []string {
	height := max(1, m.height-3-headingRows-2) // masthead, heading and pinned footer
	railWidth := min(38, max(26, width/3))
	previewWidth := width - railWidth - 4 // gap and vertical accent
	visible := min(len(items), max(1, (height-1)/2))
	if len(items) > visible {
		visible = max(1, (height-2)/2) // reserve the last row for the position cue
	}
	start := min(max(0, selected-visible/2), len(items)-visible)
	right := []string{mutedStyle.Render(fmt.Sprintf("SELECTED  %d / %d", selected+1, len(items))), ""}
	right = append(right, preview(previewWidth)...)
	if height > 10 && len(right) < height-2 {
		actionRow := height - 2
		for len(right) <= actionRow {
			right = append(right, "")
		}
		right[actionRow] = accentStyle.Render(clipLine(action, previewWidth))
	}
	lines := make([]string, 0, height)
	for row := 0; row < height; row++ {
		left := ""
		switch {
		case row == 0:
			left = mutedStyle.Render(fmt.Sprintf("INBOX  %d–%d", start+1, start+visible))
		case row <= 2*visible:
			index := start + (row-1)/2
			item := items[index]
			selectedRow := index == selected
			marker := "  "
			if selectedRow {
				marker = "▸ "
				if row%2 == 0 {
					marker = "▎ "
				}
			}
			value := item.title
			style := textStyle
			if row%2 == 0 {
				value = item.context
				style = mutedStyle
			}
			value = marker + clipLine(value, railWidth-2)
			if selectedRow {
				style = selectedStyle
				if row%2 == 1 {
					style = style.Bold(true)
				}
			}
			left = style.Width(railWidth).Render(value)
		}
		if row == height-1 && len(items) > visible {
			left = mutedStyle.Render(clipLine(fmt.Sprintf("%d of %d  ·  ↑/↓ browse", selected+1, len(items)), railWidth))
		}
		part := ""
		if row < len(right) {
			part = right[row]
		}
		lines = append(lines, "  "+lipgloss.NewStyle().Width(railWidth).Render(left)+"  "+accentStyle.Render("▏ ")+part)
	}
	return lines
}
