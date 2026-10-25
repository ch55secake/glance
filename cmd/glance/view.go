package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"glance/internal/githubapi"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

var (
	hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)
)

// View renders a fixed-height dashboard; the PR window moves with the selection.
func (m model) View() string {
	if m.width < 30 || m.height < 10 {
		return m.smallTerminalView()
	}
	if m.paletteOpen || m.filePickerOpen || m.themePickerOpen {
		return m.modalView()
	}
	width := max(1, m.width)
	inner := max(1, width-4)
	if m.detailOpen {
		m.cacheDetailLines()
	}
	location := ""
	if m.detailOpen && !m.showHelp && !m.paletteOpen && !m.filePickerOpen {
		location = m.reviewLocation(inner)
	}
	lines := []string{
		m.header(inner),
		m.navigation(inner),
		location,
	}

	switch {
	case m.showHelp:
		lines = append(lines, m.helpView(inner)...)
	case m.detailOpen:
		leftWidth, rightWidth := m.reviewWidths(inner)
		content := m.pullRequestDetailsView(leftWidth)
		visible := m.detailViewportHeight()
		start := min(max(0, m.detailScroll), m.maxDetailScroll())
		end := min(len(content), start+visible)
		if rightWidth > 0 {
			sidebar := m.reviewSidebar(rightWidth, visible)
			for i := 0; i < visible; i++ {
				left := ""
				if start+i < end {
					left = content[start+i]
				}
				right := ""
				if i < len(sidebar) {
					right = sidebar[i]
				}
				lines = append(lines, lipgloss.NewStyle().Width(leftWidth+2).Render(left)+"  "+right)
			}
		} else {
			lines = append(lines, content[start:end]...)
			for i := end - start; i < visible; i++ {
				lines = append(lines, "")
			}
		}
		lines = append(lines, m.mergeActionPanel(inner)...)
	case m.activeSection == 0:
		lines = append(lines, m.pullRequestView(inner)...)
	case m.activeSection == 1:
		lines = append(lines, m.issueView(inner)...)
	case m.activeSection == 2:
		lines = append(lines, m.notificationView(inner)...)
	default:
		lines = append(lines, m.placeholderView(inner)...)
	}

	if !m.detailOpen || m.showHelp || m.paletteOpen || m.filePickerOpen {
		// Keep controls at the bottom as lists, empty states, and errors change size.
		for len(lines) < m.height-2 {
			lines = append(lines, "")
		}
		lines = append(lines, "", m.footer(inner))
	}
	// Keep the view inside the terminal even on small screens or verbose API errors.
	viewLines := strings.Split(strings.Join(lines, "\n"), "\n")
	if m.height > 3 && len(viewLines) > m.height {
		viewLines = append(viewLines[:m.height-3], viewLines[len(viewLines)-3:]...)
	}
	for i, line := range viewLines {
		viewLines[i] = ansi.Truncate(line, width, "…")
	}
	return strings.Join(viewLines, "\n")
}

func (m model) smallTerminalView() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	lines := []string{brandStyle.Render("glance"), "Resize to at least 30 × 10", "ctrl+c quit"}
	lines = lines[:min(len(lines), m.height)]
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, m.width, "")
	}
	return strings.Join(lines, "\n")
}

func (m model) header(width int) string {
	left := brandStyle.Render("◈ glance")
	status := "CONNECTING"
	style := warnStyle
	if m.hasErrors() {
		status = "API ERROR"
		style = errorStyle
	} else if m.viewer.Login != "" {
		status = "● @" + safeText(m.viewer.Login)
		style = goodStyle
		if m.loading {
			status = "↻ " + status
		}
	}
	if m.themeConfigError != "" {
		status, style = "CONFIG ERROR", warnStyle
	}
	if width < 35 {
		return "  " + left
	}
	return "  " + align(left, style.Render(clipLine(status, width/2)), width)
}

func (m model) navigation(width int) string {
	if m.detailOpen {
		left := "  ← Pull requests  /  " + safeText(m.detailKey)
		style := mutedStyle
		if !m.sidebarFocus && m.mergeDialog == noMergeDialog && !m.mergeInProgress && !m.showHelp && !m.paletteOpen && !m.filePickerOpen {
			left = "▎ ← Pull requests  /  " + safeText(m.detailKey)
			style = activeStyle
		}
		if width >= 55 && !m.paletteOpen && !m.showHelp && !m.filePickerOpen {
			leftWidth, _ := m.reviewWidths(width)
			total := len(m.pullRequestDetailsView(leftWidth))
			progress := fmt.Sprintf("%d–%d / %d", min(m.detailScroll+1, total), min(m.detailScroll+m.detailViewportHeight(), total), total)
			return "  " + alignStyled(left, progress, width, style, mutedStyle)
		}
		return "  " + style.Render(clipLine(left, width))
	}
	if width < 40 {
		label := fmt.Sprintf("%d / %d  %s", m.activeSection+1, len(sections), sections[m.activeSection])
		return "  " + activeStyle.Render(clipLine(label, width))
	}
	tabs := make([]string, 0, len(sections))
	names := sections
	if width < 65 {
		names = []string{"PRs", "Issues", "Inbox"}
	}
	for i, section := range names {
		label := fmt.Sprintf(" %d %s ", i+1, section)
		if width >= 90 {
			count := []int{len(m.pullRequests), len(m.issues), len(m.notifications)}[i]
			label = fmt.Sprintf(" %d %s (%d) ", i+1, section, count)
		}
		if i == m.activeSection {
			tabs = append(tabs, activeStyle.Render(label))
		} else {
			tabs = append(tabs, mutedStyle.Render(label))
		}
	}
	return "  " + strings.Join(tabs, " ")
}

func (m model) placeholderView(width int) []string {
	name := sections[m.activeSection]
	return []string{
		"  " + accentStyle.Render(strings.ToUpper(name)),
		"",
		"  " + mutedStyle.Render(clipLine(name+" are not available yet.", width)),
		"  " + mutedStyle.Render(clipLine("Use ←/→ to switch back to your pull requests.", width)),
	}
}

func (m model) issueView(width int) []string {
	heading := "ISSUES"
	if m.loading && len(m.issues) > 0 {
		heading += "  ↻ Refreshing"
	}
	lines := []string{
		"  " + alignStyled(heading, fmt.Sprintf("%d open", len(m.issues)), width, accentStyle, mutedStyle),
		"",
	}
	if m.loading && len(m.issues) == 0 {
		return append(lines, m.message(width, "Loading", "Loading issues across your repositories...", "This can take a moment for large accounts.")...)
	}
	if m.issueError != "" {
		return append(lines, m.message(width, "Could not load issues", m.issueError, "Press r to try again.")...)
	}
	if len(m.issues) == 0 {
		return append(lines, m.message(width, "All clear", "No open issues found across repositories visible to your account.", "Press r to refresh.")...)
	}
	if m.hasInboxPreview() {
		return append(lines, m.issueWorkspace(width)...)
	}
	visible := min(m.visibleListCount(), len(m.issues))
	start := min(max(0, m.selectedIssue-visible/2), len(m.issues)-visible)
	end := start + visible
	for i := start; i < end; i++ {
		if i > start && m.listRowSpacing() > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, m.issueRow(m.issues[i], width, i == m.selectedIssue)...)
	}
	return append(lines, "  "+mutedStyle.Render(clipLine(fmt.Sprintf("%d–%d of %d  ·  j/k move", start+1, end, len(m.issues)), width)))
}

func (m model) issueRow(issue githubapi.Issue, width int, selected bool) []string {
	marker := "  "
	if selected {
		marker = "▎ "
	}
	badge := openBadge.Render(" ● OPEN ")
	if selected {
		badge = " ● OPEN "
	}
	label := fmt.Sprintf("#%d  %s", issue.Number, safeText(issue.Repository))
	first := marker + clipLine(safeText(issue.Title), width-2)
	second := marker + clipLine(label, max(1, width-lipgloss.Width(badge)-3)) + " " + badge
	metadata := "@" + safeText(issue.User.Login) + "  ·  " + fmt.Sprintf("%d comments", issue.Comments) + "  ·  updated " + relativeTime(issue.UpdatedAt)
	if len(issue.Labels) > 0 {
		labels := make([]string, 0, len(issue.Labels))
		for _, label := range issue.Labels {
			labels = append(labels, safeText(label.Name))
		}
		metadata += "  ·  " + strings.Join(labels, ", ")
	}
	third := marker + clipLine(metadata, width-2)
	if selected {
		return []string{"  " + selectedStyle.Bold(true).Width(width).Render(first), "  " + selectedStyle.Width(width).Render(second), "  " + selectedStyle.Width(width).Render(third)}
	}
	return []string{"  " + textStyle.Bold(true).Render(first), "  " + second, "  " + mutedStyle.Render(third)}
}

func (m model) notificationView(width int) []string {
	unread := 0
	for _, notification := range m.notifications {
		if notification.Unread {
			unread++
		}
	}
	heading := "NOTIFICATIONS"
	if m.loading && len(m.notifications) > 0 {
		heading += "  ↻ Refreshing"
	}
	lines := []string{
		"  " + alignStyled(heading, fmt.Sprintf("%d unread  ·  %d total", unread, len(m.notifications)), width, accentStyle, mutedStyle),
		"",
	}
	if m.loading && len(m.notifications) == 0 {
		return append(lines, m.message(width, "Loading", "Loading GitHub notifications...", "This can take a moment.")...)
	}
	if m.notificationError != "" {
		return append(lines, m.message(width, "Could not load notifications", m.notificationError, "Press r to try again.")...)
	}
	if len(m.notifications) == 0 {
		return append(lines, m.message(width, "You're all caught up", "No notifications were found.", "Press r to refresh.")...)
	}
	if m.hasInboxPreview() {
		return append(lines, m.notificationWorkspace(width)...)
	}
	visible := min(m.visibleListCount(), len(m.notifications))
	start := min(max(0, m.selectedNotification-visible/2), len(m.notifications)-visible)
	end := start + visible
	for i := start; i < end; i++ {
		if i > start && m.listRowSpacing() > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, m.notificationRow(m.notifications[i], width, i == m.selectedNotification)...)
	}
	return append(lines, "  "+mutedStyle.Render(clipLine(fmt.Sprintf("%d–%d of %d  ·  j/k move", start+1, end, len(m.notifications)), width)))
}

func (m model) notificationRow(notification githubapi.Notification, width int, selected bool) []string {
	marker := "  "
	if selected {
		marker = "▎ "
	}
	state := mutedStyle.Render("○ READ")
	if notification.Unread {
		state = unreadBadge.Render(" ● UNREAD ")
	}
	if selected {
		state = "● UNREAD"
		if !notification.Unread {
			state = "○ READ"
		}
	}
	label := fmt.Sprintf("%s  ·  %s", safeText(notification.Repository.FullName), safeText(notification.Subject.Type))
	first := marker + clipLine(safeText(notification.Subject.Title), width-2)
	second := marker + clipLine(label, max(1, width-lipgloss.Width(state)-3)) + " " + state
	third := marker + clipLine(safeText(notification.Reason)+"  ·  updated "+relativeTime(notification.UpdatedAt), width-2)
	if selected {
		return []string{"  " + selectedStyle.Bold(true).Width(width).Render(first), "  " + selectedStyle.Width(width).Render(second), "  " + selectedStyle.Width(width).Render(third)}
	}
	return []string{"  " + textStyle.Bold(true).Render(first), "  " + second, "  " + mutedStyle.Render(third)}
}

func (m model) hasErrors() bool {
	return m.apiError != "" || m.issueError != "" || m.notificationError != ""
}

func (m model) helpView(width int) []string {
	content := m.helpContent(width)
	visible := m.helpViewportHeight()
	start := min(m.helpScroll, max(0, len(content)-visible))
	heading := accentStyle.Render("KEYBOARD SHORTCUTS")
	if width >= 50 {
		heading = align(heading, mutedStyle.Render(scrollPosition(start, visible, len(content))), width)
	}
	return append([]string{"  " + heading, ""}, content[start:min(len(content), start+visible)]...)
}

func (m model) pullRequestView(width int) []string {
	items := m.pullRequests
	lines := m.pullRequestToolbar(width)
	if (m.prLoading || m.loading && m.prRequestID == 0) && len(m.pullRequests) == 0 {
		return append(lines, m.message(width, "Loading", "Searching GitHub for pull requests...", "This can take a moment.")...)
	}
	if m.apiError != "" {
		return append(lines, m.message(width, "Could not search pull requests", m.apiError, "Edit the filter with / or press r to retry.")...)
	}
	if len(m.pullRequests) == 0 {
		return append(lines, m.message(width, "No matches", "No pull requests match this GitHub filter.", "Press / to change the filter.")...)
	}
	if m.hasInboxPreview() {
		return append(lines, m.pullRequestWorkspace(items, width)...)
	}

	visible := min(m.visibleListCount(), len(items))
	start := max(0, m.selectedPR-visible/2)
	start = min(start, len(items)-visible)
	end := start + visible
	for i := start; i < end; i++ {
		if i > start && m.listRowSpacing() > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, m.prRow(items[i], width, i == m.selectedPR)...)
	}
	lines = append(lines, "  "+mutedStyle.Render(clipLine(fmt.Sprintf("%d/%d selected  ·  showing %d–%d  ·  ↵ review  ·  / filter", m.selectedPR+1, len(items), start+1, end), width)))
	return lines
}

func (m model) pullRequestDetailsView(width int) []string {
	if m.detailLines != nil && m.detailLinesWidth == width {
		return m.detailLines
	}
	return m.renderPullRequestDetails(width)
}

func (m model) renderPullRequestDetails(width int) []string {
	return m.renderReview(width).lines
}

func (m model) renderReview(width int) reviewDocument {
	if m.prDetails.Number == 0 {
		var lines []string
		if m.detailLoading {
			lines = m.message(width, "Loading pull request", "Fetching pull request details, reviews, and checks...", "This can take a moment.")
		} else if m.detailError != "" {
			lines = m.message(width, "Could not load pull request", m.detailError, "Press r to try again, or esc to return.")
		} else {
			lines = m.message(width, "Pull request unavailable", "No pull request details are loaded.", "Press r to try again, or esc to return.")
		}
		return appendReviewDiff(lines, m.renderReviewDiff(width))
	}
	details := m.prDetails
	state := openBadge.Render(" ● OPEN ")
	if details.Merged {
		state = mergedBadge.Render(" ● MERGED ")
	} else if details.Draft {
		state = draftBadge.Render(" ◆ DRAFT ")
	} else if details.State != "open" {
		state = mutedStyle.Render(" " + strings.ToUpper(safeText(details.State)) + " ")
	}
	lines := []string{
		"  " + accentStyle.Render(clipLine(fmt.Sprintf("%s#%d  ·  %s", safeText(details.Repository), details.Number, safeText(details.Title)), width)),
		"  " + state,
	}
	if m.detailLoading {
		lines = append(lines, "  "+warnStyle.Render("Refreshing pull request details…"))
	}
	if m.detailError != "" {
		lines = append(lines, "  "+errorStyle.Render(clipLine("Refresh failed: "+m.detailError, width)))
	}
	if m.prDetails.MergeMethodsError != "" {
		lines = append(lines, "  "+warnStyle.Render(clipLine("Merge methods unavailable: "+safeText(m.prDetails.MergeMethodsError), width)))
	}
	lines = append(lines,
		"  "+mutedStyle.Render(clipLine("Author: @"+safeText(details.User.Login), width)),
		"  "+mutedStyle.Render(clipLine(fmt.Sprintf("Branches: %s  →  %s", safeText(details.Head.Ref), safeText(details.Base.Ref)), width)),
		"  "+mutedStyle.Render(clipLine("Updated "+relativeTime(details.UpdatedAt), width)),
	)
	if !m.hasReviewSidebar() {
		lines = append(lines, "", m.mergeabilityLine(width))
	}
	lines = append(lines, "", "  "+accentStyle.Render("▤ DESCRIPTION"))
	lines = append(lines, markdownLines(details.Body, width)...)
	document := appendReviewDiff(lines, m.renderReviewDiff(width))
	lines = document.lines
	if !m.hasReviewSidebar() {
		lines = append(lines, "", "  "+accentStyle.Render(fmt.Sprintf("◉ CHECKS (%d)", len(details.CheckRuns))))
		lines = append(lines, m.checkStatusLines(width)...)
		lines = append(lines, "")
		lines = append(lines, m.reviewActivityLines(width)...)
	}
	document.lines = lines
	return document
}

func (m model) reviewActivityLines(width int) []string {
	details := m.prDetails
	lines := []string{"  " + accentStyle.Render("◇ REVIEW ACTIVITY"), "  " + accentStyle.Render("Requested reviewers")}
	if details.Number == 0 {
		if m.detailLoading {
			return append(lines, "  "+warnStyle.Render("Loading reviews…"))
		}
		if m.detailError != "" {
			return append(lines, "  "+errorStyle.Render("Reviews unavailable"))
		}
	}
	if len(details.RequestedReviewers) == 0 {
		lines = append(lines, "  "+mutedStyle.Render("None"))
	} else {
		for _, reviewer := range details.RequestedReviewers {
			for _, part := range wrapDiffLine("@"+safeText(reviewer.Login), max(1, width)) {
				lines = append(lines, "  "+mutedStyle.Render(part))
			}
		}
	}
	lines = append(lines, "  "+accentStyle.Render("Reviews"))
	if details.ReviewsError != "" {
		lines = append(lines, panelMessage("Unavailable: "+details.ReviewsError, width, errorStyle)...)
	} else if len(details.Reviews) == 0 {
		lines = append(lines, "  "+mutedStyle.Render("No reviews submitted"))
	} else {
		seen := make(map[string]bool)
		for _, review := range details.Reviews {
			login := safeText(review.User.Login)
			if seen[strings.ToLower(login)] {
				continue
			}
			seen[strings.ToLower(login)] = true
			style := mutedStyle
			switch strings.ToUpper(review.State) {
			case "APPROVED":
				style = goodStyle
			case "CHANGES_REQUESTED":
				style = errorStyle
			}
			for _, part := range wrapDiffLine("@"+login+": "+strings.ToUpper(safeText(review.State)), max(1, width)) {
				lines = append(lines, "  "+style.Render(part))
			}
		}
	}
	return lines
}

func (m model) reviewWidths(width int) (left, right int) {
	if width < 96 {
		return width, 0
	}
	return width - 34, 32 // two-column layout with a two-cell gutter
}

func (m model) hasReviewSidebar() bool {
	_, right := m.reviewWidths(max(1, m.width-4))
	return right > 0
}

func (m model) mergeabilityLine(width int) string {
	return m.mergeabilityStatus(width, false)
}

func (m model) mergeabilityStatus(width int, compact bool) string {
	details := m.prDetails
	var symbol, label string
	style := warnStyle
	switch {
	case m.detailLoading && details.Number == 0:
		symbol, label = "◌", "loading"
	case details.Merged:
		symbol, label, style = "✓", "merged", mergedBadge
	case details.Draft:
		symbol, label = "◇", "draft"
	case details.Mergeable == nil:
		symbol, label = "?", "unknown"
	case *details.Mergeable:
		symbol, label, style = "✓", "mergeable", goodStyle
	default:
		symbol, label, style = "✕", "blocked", errorStyle
	}
	if details.MergeableState != "" {
		label += " (" + safeText(details.MergeableState) + ")"
	}
	if !compact {
		label = "Mergeability: " + label
	}
	return "  " + style.Render(clipLine(symbol+" "+label, width))
}

func checkStyle(check githubapi.CheckRun) (label string, style lipgloss.Style) {
	state := check.Conclusion
	if state == "" {
		state = check.Status
	}
	switch state {
	case "success":
		return "✓ PASS", goodStyle
	case "neutral", "skipped":
		return "– SKIP", mutedStyle
	case "failure", "timed_out", "action_required", "startup_failure", "stale":
		return "✕ FAIL", errorStyle
	case "cancelled":
		return "! CANCEL", warnStyle
	case "queued", "in_progress", "pending", "waiting", "requested":
		return "◌ RUNNING", warnStyle
	default:
		return "? UNKNOWN", mutedStyle
	}
}

func (m model) checkStatusLines(width int) []string {
	if m.prDetails.ChecksError != "" {
		return panelMessage("Checks unavailable: "+m.prDetails.ChecksError, width, errorStyle)
	}
	if m.detailLoading && m.prDetails.Number == 0 {
		return []string{"  " + warnStyle.Render("Loading checks…")}
	}
	if len(m.prDetails.CheckRuns) == 0 {
		return []string{"  " + mutedStyle.Render("No checks reported")}
	}
	lines := make([]string, 0, len(m.prDetails.CheckRuns))
	for _, check := range m.prDetails.CheckRuns {
		label, style := checkStyle(check)
		parts := wrapDiffLine(safeText(check.Name), max(1, width-lipgloss.Width(label)-2))
		for i, part := range parts {
			if i == 0 {
				lines = append(lines, "  "+style.Render(label+"  "+part))
			} else {
				lines = append(lines, "  "+style.Render("↳ "+part))
			}
		}
	}
	return lines
}

func (m model) maxSidebarScroll() int {
	_, width := m.reviewWidths(max(1, m.width-4))
	if width == 0 {
		return 0
	}
	if m.detailViewportHeight() <= 5 {
		return 0
	}
	available := max(1, m.detailViewportHeight()-5)
	return max(0, len(m.sidebarContent(width-2))-available)
}

func (m model) sidebarContent(width int) []string {
	lines := m.checkStatusLines(width)
	lines = append(lines, "")
	return append(lines, m.reviewActivityLines(width)...)
}

func (m model) reviewSidebar(width, height int) []string {
	inner := max(1, width-2)
	heading := "  ◈ READINESS"
	headingStyle := accentStyle
	if m.sidebarFocus && m.mergeDialog == noMergeDialog && !m.mergeInProgress && !m.showHelp && !m.paletteOpen && !m.filePickerOpen {
		heading = "▎ ◈ READINESS"
		headingStyle = activeStyle
	}
	lines := []string{
		"  " + headingStyle.Render(clipLine(heading, inner)),
		m.mergeabilityStatus(inner, true),
		"  " + mutedStyle.Render(strings.Repeat("─", inner)),
		"  " + accentStyle.Render(clipLine(fmt.Sprintf("◉ CHECKS  (%d)", len(m.prDetails.CheckRuns)), inner)),
	}
	if len(lines) >= height {
		return lines[:height]
	}
	content := m.sidebarContent(inner)
	visible := max(1, height-len(lines)-1)
	start := min(max(0, m.sidebarScroll), max(0, len(content)-visible))
	end := min(len(content), start+visible)
	lines = append(lines, content[start:end]...)
	if len(content) > visible {
		lines = append(lines, "  "+mutedStyle.Render(clipLine(fmt.Sprintf("%d–%d / %d", start+1, end, len(content)), inner)))
	} else {
		lines = append(lines, "")
	}
	return lines
}

// mergeActionPanel stays below the scrollable PR content so the next action and
// any result remain visible even at the bottom of a long diff.
func (m model) mergeActionPanel(width int) []string {
	lines := []string{"  " + mutedStyle.Render(strings.Repeat("─", width))}
	if m.mergeError != "" {
		lines = append(lines, panelMessage("Merge: "+m.mergeError, width, errorStyle)...)
	}
	if m.mergeNotice != "" {
		lines = append(lines, panelMessage(m.mergeNotice, width, goodStyle)...)
	}
	switch {
	case m.mergeInProgress:
		return append(lines, "  "+warnStyle.Render(clipLine("MERGING  ·  Waiting for GitHub…", width)))
	case m.mergeDialog == chooseMergeMethod:
		lines = append(lines, "  "+accentStyle.Render(clipLine("Choose merge method", width)))
		for i, method := range m.prDetails.AllowedMergeMethods {
			choice := fmt.Sprintf(" %d  %s ", i+1, mergeMethodLabel(method))
			if i == m.mergeMethodIndex {
				lines = append(lines, "  "+activeStyle.Render(clipLine("▸"+choice, width)))
			} else {
				lines = append(lines, "  "+mutedStyle.Render(clipLine(" "+choice, width)))
			}
		}
		return append(lines, "  "+mutedStyle.Render(clipLine("↑/↓ or j/k choose  ·  enter continue  ·  esc cancel", width)))
	case m.mergeDialog == confirmMerge:
		lines = append(lines,
			"  "+warnStyle.Render(clipLine("Confirm merge  ·  "+mergeMethodLabel(m.mergeMethod), width)),
			"  "+textStyle.Render(clipLine(safeText(m.detailKey)+"  →  "+safeText(m.prDetails.Base.Ref), width)),
			"  "+mutedStyle.Render(clipLine("y confirm  ·  esc cancel", width)),
		)
		return lines
	default:
		actions := "◆ ACTIONS  ·  m merge  ·  r refresh  ·  esc back"
		guidance := "f files  ·  [/] jump  ·  v viewed  ·  ↑/↓ scroll  ·  ? help"
		if width < 45 {
			actions = "◆ ACTIONS  ·  m merge"
			guidance = "esc back  ·  f files"
		} else if width < 70 {
			actions = "◆ ACTIONS  ·  m merge  ·  esc back"
			guidance = "f files  ·  [/] jump  ·  v viewed"
		} else if m.hasReviewSidebar() {
			guidance = "f files  ·  [/] jump  ·  v viewed  ·  tab switch pane  ·  PgUp/PgDn page  ·  ? help"
		}
		if width >= 70 {
			actions += "  ·  : commands"
		}
		return append(lines,
			"  "+accentStyle.Render(clipLine(actions, width)),
			"  "+mutedStyle.Render(clipLine(guidance, width)),
		)
	}
}

func panelMessage(message string, width int, style lipgloss.Style) []string {
	parts := wrapDiffLine(safeText(message), width)
	lines := make([]string, 0, min(4, len(parts)))
	for i, part := range parts {
		if i == 4 {
			break
		}
		if i == 3 && len(parts) > 4 {
			part = clipLine(part+"…", width)
		}
		lines = append(lines, "  "+style.Render(part))
	}
	return lines
}

func (m model) pullRequestDiffLines(width int) []string {
	return m.renderReviewDiff(width).lines
}

func (m model) renderReviewDiff(width int) reviewDocument {
	lines := []string{"", "  " + accentStyle.Render("▥ CHANGED FILES")}
	if m.diffLoading && !m.diffLoaded {
		return reviewDocument{lines: append(lines, "  "+warnStyle.Render("Loading changed files and patches from GitHub…"))}
	}
	if m.diffError != "" {
		lines = append(lines, "  "+errorStyle.Render(clipLine("Could not load diff: "+m.diffError, width)))
		if !m.diffLoaded || len(m.diffFiles) == 0 {
			return reviewDocument{lines: lines}
		}
	}
	if !m.diffLoaded {
		return reviewDocument{lines: append(lines, "  "+mutedStyle.Render("Changed files have not loaded yet."))}
	}
	if len(m.diffFiles) == 0 {
		return reviewDocument{lines: append(lines, "  "+mutedStyle.Render("No changed files."))}
	}
	additions, deletions := 0, 0
	for _, file := range m.diffFiles {
		additions += file.Additions
		deletions += file.Deletions
	}
	fileLabel := "files"
	if len(m.diffFiles) == 1 {
		fileLabel = "file"
	}
	lines = append(lines, "  "+mutedStyle.Render(clipLine(fmt.Sprintf("%d %s changed  ·  +%d −%d", len(m.diffFiles), fileLabel, additions, deletions), width)))
	var files []reviewFileAnchor
	for i, file := range m.diffFiles {
		if i > 0 {
			lines = append(lines, "")
		}
		start := len(lines)
		path := safeText(file.Filename)
		if file.PreviousFilename != "" && file.PreviousFilename != file.Filename {
			path = safeText(file.PreviousFilename) + " → " + path
		}
		for j, part := range wrapDiffLine(path, max(1, width-2)) {
			prefix := "  "
			if j > 0 {
				prefix = "↳ "
			}
			lines = append(lines, "  "+fileLineStyle.Width(width).Render(prefix+part))
		}
		lines = append(lines, "  "+mutedStyle.Render(clipLine(fmt.Sprintf("File %d/%d  ·  %s  ·  +%d −%d", i+1, len(m.diffFiles), safeText(file.Status), file.Additions, file.Deletions), width)))
		if file.Patch == "" {
			lines = append(lines, "  "+mutedStyle.Render(clipLine("GitHub did not provide a patch (binary or too large).", width)))
		} else {
			lines = append(lines, renderFilePatch(file.Patch, file.Filename, width)...)
		}
		files = append(files, reviewFileAnchor{index: i, row: start, end: len(lines)})
	}
	return reviewDocument{lines: lines, files: files}
}

// renderPatch keeps unified-diff line numbers intact while wrapping long code
// lines instead of silently hiding the remainder offscreen.
func renderPatch(patch string, width int) []string {
	return renderDiffPatch(patch, width, nil)
}

func renderFilePatch(patch, filename string, width int) []string {
	return renderDiffPatch(patch, width, lexers.Match(filename))
}

func renderDiffPatch(patch string, width int, lexer chroma.Lexer) []string {
	var lines []string
	oldLine, newLine := 0, 0
	for _, raw := range strings.Split(strings.TrimSuffix(patch, "\n"), "\n") {
		line := safeMultiline(raw)
		if match := hunkHeader.FindStringSubmatch(line); match != nil {
			oldLine, _ = strconv.Atoi(match[1])
			newLine, _ = strconv.Atoi(match[2])
			for _, part := range wrapDiffLine(line, width) {
				lines = append(lines, "  "+hunkLineStyle.Width(width).Render(part))
			}
			continue
		}
		style := textStyle
		oldNumber, newNumber := "", ""
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "\\"):
			style = mutedStyle
		case strings.HasPrefix(line, "+"):
			newNumber = strconv.Itoa(newLine)
			newLine++
			style = addLineStyle
		case strings.HasPrefix(line, "-"):
			oldNumber = strconv.Itoa(oldLine)
			oldLine++
			style = delLineStyle
		default:
			oldNumber, newNumber = strconv.Itoa(oldLine), strconv.Itoa(newLine)
			oldLine++
			newLine++
		}
		gutter := fmt.Sprintf("%4s %4s │ ", oldNumber, newNumber)
		available := max(1, width-lipgloss.Width(gutter))
		for i, part := range wrapDiffLine(line, available) {
			prefix := gutter
			if i > 0 {
				prefix = strings.Repeat(" ", lipgloss.Width(gutter)-2) + "↳ "
			}
			if lexer != nil && lipgloss.ColorProfile() != termenv.Ascii &&
				!strings.HasPrefix(line, "\\") && !strings.HasPrefix(line, "+++") && !strings.HasPrefix(line, "---") {
				marker, code := "", part
				if i == 0 && len(part) > 0 {
					marker, code = part[:1], part[1:]
				}
				part = marker + highlightCode(code, lexer, style)
			}
			lines = append(lines, "  "+style.Width(width).Render(prefix+part))
		}
	}
	return lines
}

func highlightCode(code string, lexer chroma.Lexer, background lipgloss.Style) string {
	tokens, err := chroma.Tokenise(lexer, nil, code)
	if err != nil {
		return code
	}
	var result strings.Builder
	for _, token := range tokens {
		style := background
		switch {
		case token.Type.InCategory(chroma.Keyword):
			style = style.Foreground(lipgloss.Color(syntaxKeyword)).Bold(true)
		case token.Type.InCategory(chroma.LiteralString):
			style = style.Foreground(lipgloss.Color(syntaxString))
		case token.Type.InCategory(chroma.LiteralNumber):
			style = style.Foreground(lipgloss.Color(syntaxNumber))
		case token.Type.InCategory(chroma.Comment):
			style = style.Foreground(lipgloss.Color(syntaxComment))
		}
		result.WriteString(style.Render(token.Value))
	}
	return result.String()
}

func wrapDiffLine(line string, width int) []string {
	width = max(1, width)
	var parts []string
	var current strings.Builder
	used := 0
	for _, char := range line {
		charWidth := lipgloss.Width(string(char))
		if used > 0 && used+charWidth > width {
			parts = append(parts, current.String())
			current.Reset()
			used = 0
		}
		current.WriteRune(char)
		used += charWidth
	}
	return append(parts, current.String())
}

func mergeMethodLabel(method githubapi.MergeMethod) string {
	switch method {
	case githubapi.MergeCommit:
		return "Merge commit"
	case githubapi.MergeSquash:
		return "Squash and merge"
	case githubapi.MergeRebase:
		return "Rebase and merge"
	default:
		return string(method)
	}
}

func safeMultiline(text string) string {
	var safe strings.Builder
	for _, char := range text {
		if char == '\n' {
			safe.WriteRune(char)
		} else if char == '\r' || char == '\t' {
			safe.WriteRune(' ')
		} else if !unicode.IsControl(char) {
			safe.WriteRune(char)
		}
	}
	return safe.String()
}

func (m model) githubFilterBox(width int) []string {
	value := m.query
	if m.searching {
		value = m.searchDraft
	}
	if width < 12 {
		return []string{accentStyle.Render(clipLine("⌕ "+safeText(value), width))}
	}
	boxWidth := min(width, 68)
	inputWidth := boxWidth - 2
	value = safeText(value)
	available := inputWidth - lipgloss.Width(" ⌕ ") - 1
	if m.searching {
		value = searchViewport(value, m.searchCursor, m.searchOffset, available)
	} else if lipgloss.Width(value) > available {
		for lipgloss.Width(value) > available-1 && len(value) > 0 {
			value = string([]rune(value)[1:])
		}
		value = "…" + value
	}
	border := lipgloss.NewStyle().Foreground(lipgloss.Color(themeBorderColor))
	if m.searching {
		border = accentStyle
	} else if m.apiError != "" {
		border = errorStyle
	}
	count := fmt.Sprintf("%d found", len(m.pullRequests))
	if m.prLoading || m.loading {
		count = "↻ " + count
	}
	top := "╭─ " + count + " " + strings.Repeat("─", max(0, boxWidth-lipgloss.Width(count)-5)) + "╮"
	hint := "/ edit"
	if m.searching {
		hint = "editing"
	}
	bottom := "╰─ " + hint + " " + strings.Repeat("─", max(0, boxWidth-lipgloss.Width(hint)-5)) + "╯"
	return []string{
		border.Render(top),
		border.Render("│") + textStyle.Width(inputWidth).Render(" ⌕ "+value) + border.Render("│"),
		border.Render(bottom),
	}
}

func (m model) prRow(pr githubapi.PullRequest, width int, selected bool) []string {
	marker := "  "
	if selected {
		marker = "▎ "
	}
	state := " ● OPEN "
	if pr.Draft {
		state = " ◆ DRAFT "
	}
	if !selected {
		if pr.Draft {
			state = draftBadge.Render(state)
		} else {
			state = openBadge.Render(state)
		}
	}
	// Keep the number visible if a long repository name needs clipping.
	labelWidth := max(1, width-lipgloss.Width(state)-3)
	label := fmt.Sprintf("%s#%d", safeText(pr.Repository), pr.Number)
	if lipgloss.Width(label) > labelWidth {
		label = fmt.Sprintf("#%d  %s", pr.Number, safeText(pr.Repository))
	}
	second := marker + align(clipLine(label, labelWidth), state, width-2)
	titleMarker := marker
	if selected {
		titleMarker = "▸ "
	}
	first := titleMarker + clipLine(safeText(pr.Title), max(1, width-2))
	third := marker + clipLine("@"+safeText(pr.User.Login)+"  ·  updated "+relativeTime(pr.UpdatedAt), max(1, width-2))
	if selected {
		return []string{
			"  " + selectedStyle.Bold(true).Width(width).Render(first),
			"  " + selectedStyle.Width(width).Render(second),
			"  " + selectedStyle.Foreground(lipgloss.Color(themeTextColor)).Width(width).Render(third),
		}
	}
	return []string{"  " + textStyle.Bold(true).Render(first), "  " + second, "  " + mutedStyle.Render(third)}
}

func (m model) message(width int, title, detail, hint string) []string {
	wrap := lipgloss.NewStyle().Width(width)
	wrapped := strings.ReplaceAll(wrap.Render(safeText(detail)), "\n", "\n  ")
	style := accentStyle
	if strings.HasPrefix(title, "Could not load") || title == "Connection problem" {
		style = errorStyle
	} else if title == "All clear" {
		style = goodStyle
	} else if title == "No matches" {
		style = warnStyle
	}
	return []string{
		"", "  " + style.Render(clipLine(title, width)),
		"  " + wrapped,
		"", "  " + mutedStyle.Render(clipLine(hint, width)),
	}
}

func (m model) footer(width int) string {
	var hints []shortcut
	switch {
	case m.filePickerOpen:
		hints = []shortcut{{"enter", "jump"}, {"esc", "close"}, {"↑/↓", "choose"}, {"ctrl+u", "clear"}}
	case m.themePickerOpen:
		hints = []shortcut{{"enter", "apply"}, {"esc", "close"}, {"↑/↓", "preview"}}
	case m.paletteOpen:
		hints = []shortcut{{"enter", "run"}, {"esc", "close"}, {"↑/↓", "choose"}, {"ctrl+u", "clear"}}
	case m.searching:
		hints = []shortcut{{"enter", "apply"}, {"esc", "cancel"}, {"←/→", "move"}, {"Home/End", "jump"}, {"ctrl+u", "clear"}}
	case m.showHelp:
		hints = []shortcut{{"esc", "close"}, {"↑/↓", "scroll"}, {"?", "close"}, {"q", "quit"}}
	case m.inboxPreviewFocus:
		hints = []shortcut{{"esc", "list"}, {"?", "help"}, {"↑/↓", "scroll"}, {"PgUp/PgDn", "page"}, {":", "commands"}}
	case m.activeSection == 0:
		hints = []shortcut{{"enter", "review"}, {"?", "help"}, {"/", "filter"}, {":", "commands"}}
	default:
		hints = []shortcut{{"↑/↓", "select"}, {"?", "help"}, {":", "commands"}}
	}
	if !m.paletteOpen && !m.filePickerOpen && !m.themePickerOpen && !m.searching && !m.showHelp {
		if m.hasInboxPreview() && !m.inboxPreviewFocus {
			hints = append(hints, shortcut{"tab", "preview"})
		}
		hints = append(hints, shortcut{"1–3", "sections"}, shortcut{"r", "refresh"}, shortcut{"q", "quit"})
	}
	return "  " + shortcutHints(width, hints...)
}

type shortcut struct{ key, label string }

// Only show complete hints; important controls never disappear into an ellipsis.
func shortcutHints(width int, hints ...shortcut) string {
	var parts []string
	used := 0
	for _, hint := range hints {
		size := lipgloss.Width(hint.key + " " + hint.label)
		gap := 0
		if len(parts) > 0 {
			gap = 2
		}
		if used+gap+size > width {
			continue
		}
		parts = append(parts, accentStyle.Render(hint.key)+" "+mutedStyle.Render(hint.label))
		used += gap + size
	}
	return strings.Join(parts, "  ")
}

func (m model) visibleListCount() int {
	reserved := 9 // header, navigation, section heading, toolbar, list position and help bar
	return max(1, (m.height-reserved+m.listRowSpacing())/(3+m.listRowSpacing()))
}

func (m model) listRowSpacing() int {
	if m.height >= 24 {
		return 1
	}
	return 0
}

// align places a left and a right label on a single line.
func align(left, right string, width int) string {
	space := max(1, width-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", space) + right
}

func alignStyled(left, right string, width int, leftStyle, rightStyle lipgloss.Style) string {
	right = clipLine(right, max(1, width/2))
	rightStyled := rightStyle.Render(right)
	left = clipLine(left, max(1, width-lipgloss.Width(rightStyled)-1))
	return align(leftStyle.Render(left), rightStyled, width)
}

func clipLine(line string, width int) string {
	if lipgloss.Width(line) <= width {
		return line
	}
	if width < 2 {
		return "…"
	}
	var clipped strings.Builder
	used := 0
	for _, char := range line {
		charWidth := lipgloss.Width(string(char))
		if used+charWidth > width-1 {
			break
		}
		clipped.WriteRune(char)
		used += charWidth
	}
	return clipped.String() + "…"
}
