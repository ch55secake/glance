package main

import (
	"fmt"
	"strings"

	"glance/internal/githubapi"

	"github.com/charmbracelet/lipgloss"
)

// Wide inboxes are a compact selection rail beside a full-width preview. No
// extra API requests are needed when moving through the rail.
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
		return append(lines, "", state,
			mutedStyle.Render(clipLine(workspaceReference(pr.Repository, pr.Number, w), w)),
			"", textStyle.Render(clipLine("By @"+safeText(pr.User.Login), w)),
			mutedStyle.Render(clipLine("Updated "+relativeTime(pr.UpdatedAt), w)))
	})
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
		return append(lines, "", state,
			mutedStyle.Render(clipLine(safeText(notification.Repository.FullName)+"  ·  "+safeText(notification.Subject.Type), w)),
			"", textStyle.Render(clipLine(safeText(notification.Reason), w)),
			mutedStyle.Render(clipLine("Updated "+relativeTime(notification.UpdatedAt), w)))
	})
}

func previewTitle(title string, width int) []string {
	parts := wrapDiffLine(safeText(title), width)
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		lines = append(lines, textStyle.Bold(true).Render(part))
	}
	return lines
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
