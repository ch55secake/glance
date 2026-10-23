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
	"github.com/muesli/termenv"
)

var (
	brandStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#82E9F0"))
	accentStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#C9A7FF"))
	textStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#F1F2F6"))
	mutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#A1AAC4"))
	goodStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#9AE8B8"))
	warnStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#FBD389"))
	errorStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF959A"))
	selectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("#343C57")).Foreground(lipgloss.Color("#F1F2F6"))
	activeStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#7252AB"))
	openBadge     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#B8F3CF")).Background(lipgloss.Color("#214D42"))
	draftBadge    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFE2A6")).Background(lipgloss.Color("#61492A"))
	mergedBadge   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D6C5FF")).Background(lipgloss.Color("#443460"))
	unreadBadge   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D6C5FF")).Background(lipgloss.Color("#443460"))
	addLineStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#B8F3CF")).Background(lipgloss.Color("#183A34"))
	delLineStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB6B9")).Background(lipgloss.Color("#452B36"))
	hunkLineStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#C9A7FF")).Background(lipgloss.Color("#302946"))
	fileLineStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F1F2F6")).Background(lipgloss.Color("#343C57"))
	hunkHeader    = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)
)

// View renders a fixed-height dashboard; the PR window moves with the selection.
func (m model) View() string {
	width := max(1, m.width)
	inner := max(1, width-4)
	lines := []string{
		m.header(inner),
		"  " + mutedStyle.Render(strings.Repeat("─", inner)),
		m.navigation(inner),
		"",
	}

	switch {
	case m.showHelp:
		lines = append(lines, m.helpView(inner)...)
	case m.detailOpen:
		leftWidth, rightWidth := m.reviewWidths(inner)
		content := m.pullRequestDetailsView(leftWidth)
		visible := m.detailViewportHeight()
		start := min(max(0, m.detailScroll), max(0, len(content)-visible))
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
		}
		lines = append(lines, m.mergeActionPanel(inner)...)
	case m.activeSection == 1:
		lines = append(lines, m.pullRequestView(inner)...)
	case m.activeSection == 0:
		lines = append(lines, m.overviewView(inner)...)
	case m.activeSection == 2:
		lines = append(lines, m.issueView(inner)...)
	case m.activeSection == 3:
		lines = append(lines, m.notificationView(inner)...)
	default:
		lines = append(lines, m.placeholderView(inner)...)
	}

	if !m.detailOpen || m.showHelp {
		lines = append(lines, "", m.footer(inner))
	}
	// Keep the view inside the terminal even on small screens or verbose API errors.
	viewLines := strings.Split(strings.Join(lines, "\n"), "\n")
	if m.height > 3 && len(viewLines) > m.height {
		viewLines = append(viewLines[:m.height-3], viewLines[len(viewLines)-3:]...)
	}
	return strings.Join(viewLines, "\n")
}

func (m model) header(width int) string {
	left := brandStyle.Render("◈ glance")
	if width >= 55 {
		left += mutedStyle.Render("  /  your GitHub workspace")
	}
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
	if width < 35 {
		return "  " + left
	}
	return "  " + align(left, style.Render(clipLine(status, width/2)), width)
}

func (m model) navigation(width int) string {
	if m.detailOpen {
		left := "  ← Pull requests  /  " + safeText(m.detailKey)
		style := mutedStyle
		if !m.sidebarFocus && m.mergeDialog == noMergeDialog && !m.mergeInProgress && !m.showHelp {
			left = "▎ ← Pull requests  /  " + safeText(m.detailKey)
			style = activeStyle
		}
		if width >= 55 {
			leftWidth, _ := m.reviewWidths(width)
			total := len(m.pullRequestDetailsView(leftWidth))
			progress := fmt.Sprintf("%d–%d / %d", min(m.detailScroll+1, total), min(m.detailScroll+m.detailViewportHeight(), total), total)
			return "  " + alignStyled(left, progress, width, style, mutedStyle)
		}
		return "  " + style.Render(clipLine(left, width))
	}
	if width < 65 {
		label := fmt.Sprintf("%d / %d  %s", m.activeSection+1, len(sections), sections[m.activeSection])
		return "  " + activeStyle.Render(clipLine(label, width))
	}
	tabs := make([]string, 0, len(sections))
	for i, section := range sections {
		label := fmt.Sprintf(" %d %s ", i+1, section)
		if i == m.activeSection {
			tabs = append(tabs, activeStyle.Render(label))
		} else {
			tabs = append(tabs, mutedStyle.Render(label))
		}
	}
	return "  " + strings.Join(tabs, " ")
}

func (m model) overviewView(width int) []string {
	lines := []string{"  " + accentStyle.Render("OVERVIEW"), ""}
	if m.loading && len(m.pullRequests) == 0 && len(m.issues) == 0 && len(m.notifications) == 0 {
		return append(lines, "  "+mutedStyle.Render(clipLine("Welcome to glance  ·  Connecting to GitHub and loading pull requests...", width)))
	}
	if m.viewer.Login == "" {
		detail := m.apiError
		title := "Not connected"
		if detail == "" {
			detail = "Authenticate with `gh auth login` to connect to GitHub."
		} else {
			title = "Connection problem"
		}
		return append(lines, m.message(width, title, detail, "Press r to try again.")...)
	}
	mine, drafts, unread := 0, 0, 0
	for _, pr := range m.pullRequests {
		if strings.EqualFold(pr.User.Login, m.viewer.Login) {
			mine++
		}
		if pr.Draft {
			drafts++
		}
	}
	for _, notification := range m.notifications {
		if notification.Unread {
			unread++
		}
	}
	lines = append(lines, "  "+textStyle.Render(clipLine("Connected as @"+safeText(m.viewer.Login)+".", width)), "")
	if width >= 100 {
		cards := []string{
			statCard("OPEN", len(m.pullRequests), goodStyle, "#214D42"),
			statCard("YOURS", mine, accentStyle, "#443460"),
			statCard("DRAFTS", drafts, warnStyle, "#61492A"),
			statCard("ISSUES", len(m.issues), goodStyle, "#214D42"),
			statCard("UNREAD", unread, accentStyle, "#443460"),
		}
		lines = append(lines, "  "+strings.Join(cards, "  "))
	} else if width >= 58 {
		cards := []string{
			statCard("OPEN", len(m.pullRequests), goodStyle, "#214D42"),
			statCard("YOURS", mine, accentStyle, "#443460"),
			statCard("DRAFTS", drafts, warnStyle, "#61492A"),
		}
		lines = append(lines, "  "+strings.Join(cards, "  "), "  "+goodStyle.Render(fmt.Sprintf("%d issues", len(m.issues)))+"   "+accentStyle.Render(fmt.Sprintf("%d unread notifications", unread)))
	} else {
		lines = append(lines,
			"  "+goodStyle.Render(fmt.Sprintf("%d open pull requests", len(m.pullRequests))),
			"  "+accentStyle.Render(fmt.Sprintf("%d authored by you", mine)),
			"  "+warnStyle.Render(fmt.Sprintf("%d drafts", drafts)),
			"  "+goodStyle.Render(fmt.Sprintf("%d issues", len(m.issues))),
			"  "+accentStyle.Render(fmt.Sprintf("%d unread notifications", unread)),
		)
	}
	if m.hasErrors() {
		lines = append(lines, "", "  "+errorStyle.Render("Some dashboard data could not be loaded:"))
		for _, item := range []struct{ name, err string }{{"PRs", m.apiError}, {"Issues", m.issueError}, {"Notifications", m.notificationError}} {
			if item.err != "" {
				lines = append(lines, "  "+mutedStyle.Render(clipLine(item.name+": "+item.err, width)))
			}
		}
	}
	return append(lines, "", "  "+mutedStyle.Render(clipLine("Use → to browse pull requests, issues, and notifications.", width)))
}

func statCard(label string, count int, style lipgloss.Style, background lipgloss.Color) string {
	return style.Bold(true).Background(background).Padding(0, 1).
		Render(fmt.Sprintf("%d %s", count, label))
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
		"  " + mutedStyle.Render(clipLine("Open issues across repositories with issues enabled", width)),
		"  " + mutedStyle.Render(strings.Repeat("─", width)),
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
	visible := min(m.visibleListCount(), len(m.issues))
	start := min(max(0, m.selectedIssue-visible/2), len(m.issues)-visible)
	end := start + visible
	for i := start; i < end; i++ {
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
	first := marker + clipLine(label, max(1, width-lipgloss.Width(badge)-3)) + " " + badge
	second := marker + clipLine(safeText(issue.Title), width-2)
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
		return []string{"  " + selectedStyle.Width(width).Render(first), "  " + selectedStyle.Width(width).Render(second), "  " + selectedStyle.Width(width).Render(third)}
	}
	return []string{"  " + first, "  " + textStyle.Bold(true).Render(second), "  " + mutedStyle.Render(third)}
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
		"  " + mutedStyle.Render(clipLine("Threads from repositories visible to your account", width)),
		"  " + mutedStyle.Render(strings.Repeat("─", width)),
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
	visible := min(m.visibleListCount(), len(m.notifications))
	start := min(max(0, m.selectedNotification-visible/2), len(m.notifications)-visible)
	end := start + visible
	for i := start; i < end; i++ {
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
	first := marker + clipLine(label, max(1, width-lipgloss.Width(state)-3)) + " " + state
	second := marker + clipLine(safeText(notification.Subject.Title), width-2)
	third := marker + clipLine(safeText(notification.Reason)+"  ·  updated "+relativeTime(notification.UpdatedAt), width-2)
	if selected {
		return []string{"  " + selectedStyle.Width(width).Render(first), "  " + selectedStyle.Width(width).Render(second), "  " + selectedStyle.Width(width).Render(third)}
	}
	return []string{"  " + first, "  " + textStyle.Bold(true).Render(second), "  " + mutedStyle.Render(third)}
}

func (m model) hasErrors() bool {
	return m.apiError != "" || m.issueError != "" || m.notificationError != ""
}

func (m model) helpView(width int) []string {
	shortcuts := []string{
		"←/→ or h/l  Switch sections", "↑/↓ or j/k  Select list item", "Home / End   First / last item",
		"enter        Open PR details", "m            Select/confirm merge",
		"/            Search PRs", "a / m / d    All / mine / drafts",
		"esc          Clear search & filter", "r            Refresh", "?            Close help", "q            Quit",
	}
	if m.detailOpen {
		shortcuts = []string{
			"↑/↓ or j/k  Scroll PR details and diff", "tab          Focus sidebar (wide terminals)", "PgUp/PgDn    Scroll a screen", "Home / End   First / last line",
			"m            Merge pull request", "r            Refresh details and diff",
			"esc          Return to PR inbox", "?            Close help", "q            Quit",
		}
	}
	if m.height < 19 {
		if m.detailOpen {
			shortcuts = []string{"↑/↓ scroll   PgUp/PgDn page", "m merge   r refresh", "esc back   ? close   q quit"}
		} else {
			shortcuts = []string{"←/→ sections   ↑/↓ select", "enter details   m merge", "/ search   a/m/d filters", "r refresh   ? close   q quit"}
		}
	}
	lines := []string{"  " + accentStyle.Render("KEYBOARD SHORTCUTS"), ""}
	for _, shortcut := range shortcuts {
		lines = append(lines, "  "+textStyle.Render(clipLine(shortcut, width)))
	}
	return lines
}

func (m model) pullRequestView(width int) []string {
	items := m.filteredPRs()
	heading := "◈ PULL REQUESTS"
	if m.loading && len(m.pullRequests) > 0 {
		heading += "  ↻ Refreshing"
	}
	count := fmt.Sprintf("%d open", len(m.pullRequests))
	if len(items) != len(m.pullRequests) {
		count = fmt.Sprintf("%d/%d shown", len(items), len(m.pullRequests))
	}
	lines := []string{
		"  " + align(accentStyle.Render(clipLine(heading, max(1, width-lipgloss.Width(count)-1))), mutedStyle.Render(count), width),
		"  " + m.filterBar(width),
	}
	if m.query != "" || m.searching {
		search := "/ " + safeText(m.query)
		if m.searching {
			search += "▌"
		}
		lines = append(lines, "  "+accentStyle.Render(clipLine(search, width)))
	}
	lines = append(lines, "  "+mutedStyle.Render(strings.Repeat("─", width)))
	if m.loading && len(m.pullRequests) == 0 {
		return append(lines, m.message(width, "Loading", "Loading open pull requests across your repositories...", "This can take a moment for large accounts.")...)
	}
	if m.apiError != "" {
		return append(lines, m.message(width, "Could not load open pull requests", m.apiError, "Press r to try again.")...)
	}
	if len(m.pullRequests) == 0 {
		return append(lines, m.message(width, "All clear", "No open pull requests found across repositories visible to your account.", "Press r to refresh.")...)
	}
	if len(items) == 0 {
		return append(lines, m.message(width, "No matches", "No matching pull requests.", "Press esc to clear filters.")...)
	}

	visible := min(m.visibleListCount(), len(items))
	start := max(0, m.selectedPR-visible/2)
	start = min(start, len(items)-visible)
	end := start + visible
	for i := start; i < end; i++ {
		lines = append(lines, m.prRow(items[i], width, i == m.selectedPR)...)
	}
	lines = append(lines, "  "+mutedStyle.Render(clipLine(fmt.Sprintf("%d/%d selected  ·  showing %d–%d  ·  ↵ review  ·  / search", m.selectedPR+1, len(items), start+1, end), width)))
	return lines
}

func (m model) pullRequestDetailsView(width int) []string {
	if m.detailLines != nil && m.detailLinesWidth == width {
		return m.detailLines
	}
	return m.renderPullRequestDetails(width)
}

func (m model) renderPullRequestDetails(width int) []string {
	if m.prDetails.Number == 0 {
		var lines []string
		if m.detailLoading {
			lines = m.message(width, "Loading pull request", "Fetching pull request details, reviews, and checks...", "This can take a moment.")
		} else if m.detailError != "" {
			lines = m.message(width, "Could not load pull request", m.detailError, "Press r to try again, or esc to return.")
		} else {
			lines = m.message(width, "Pull request unavailable", "No pull request details are loaded.", "Press r to try again, or esc to return.")
		}
		return append(lines, m.pullRequestDiffLines(width)...)
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
	lines = append(lines, m.pullRequestDiffLines(width)...)
	if !m.hasReviewSidebar() {
		lines = append(lines, "", "  "+accentStyle.Render(fmt.Sprintf("◉ CHECKS (%d)", len(details.CheckRuns))))
		lines = append(lines, m.checkStatusLines(width)...)
		lines = append(lines, "")
		lines = append(lines, m.reviewActivityLines(width)...)
	}
	return lines
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
	case "failure", "timed_out", "action_required":
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
	if m.sidebarFocus && m.mergeDialog == noMergeDialog && !m.mergeInProgress && !m.showHelp {
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
		guidance := "↑/↓ or j/k scroll  ·  PgUp/PgDn page  ·  ? help"
		if width < 45 {
			actions = "◆ ACTIONS  ·  m merge"
			guidance = "esc back  ·  r refresh  ·  ↑/↓"
		} else if width < 70 {
			actions = "◆ ACTIONS  ·  m merge  ·  esc back"
			guidance = "r refresh  ·  ↑/↓ scroll  ·  PgUp/PgDn"
		} else if m.hasReviewSidebar() {
			guidance = "tab switch pane  ·  ↑/↓ scroll  ·  PgUp/PgDn page  ·  ? help"
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
	lines := []string{"", "  " + accentStyle.Render("▥ CHANGED FILES")}
	if m.diffLoading && !m.diffLoaded {
		return append(lines, "  "+warnStyle.Render("Loading changed files and patches from GitHub…"))
	}
	if m.diffError != "" {
		lines = append(lines, "  "+errorStyle.Render(clipLine("Could not load diff: "+m.diffError, width)))
		if !m.diffLoaded || len(m.diffFiles) == 0 {
			return lines
		}
	}
	if !m.diffLoaded {
		return append(lines, "  "+mutedStyle.Render("Changed files have not loaded yet."))
	}
	if len(m.diffFiles) == 0 {
		return append(lines, "  "+mutedStyle.Render("No changed files."))
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
	for i, file := range m.diffFiles {
		if i > 0 {
			lines = append(lines, "")
		}
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
			continue
		}
		lines = append(lines, renderFilePatch(file.Patch, file.Filename, width)...)
	}
	return lines
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
			style = style.Foreground(lipgloss.Color("#D6C5FF")).Bold(true)
		case token.Type.InCategory(chroma.LiteralString):
			style = style.Foreground(lipgloss.Color("#FBD389"))
		case token.Type.InCategory(chroma.LiteralNumber):
			style = style.Foreground(lipgloss.Color("#82E9F0"))
		case token.Type.InCategory(chroma.Comment):
			style = style.Foreground(lipgloss.Color("#A1AAC4"))
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

func (m model) filterBar(width int) string {
	choices := []struct {
		name string
		key  string
		mode prFilter
	}{
		{"All", "a", allPRs}, {"Mine", "m", minePRs}, {"Drafts", "d", draftPRs},
	}
	var parts []string
	for _, choice := range choices {
		label := fmt.Sprintf(" %s %s ", choice.key, choice.name)
		if m.filter == choice.mode {
			parts = append(parts, activeStyle.Render(label))
		} else {
			parts = append(parts, mutedStyle.Render(label))
		}
	}
	bar := strings.Join(parts, " ")
	if lipgloss.Width(bar) > width {
		bar = accentStyle.Render(clipLine("[a] All  [m] Mine  [d] Drafts", width))
	}
	return bar
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
	first := marker + align(clipLine(label, labelWidth), state, width-2)
	titleMarker := marker
	if selected {
		titleMarker = "▸ "
	}
	second := titleMarker + clipLine(safeText(pr.Title), max(1, width-2))
	third := marker + clipLine("@"+safeText(pr.User.Login)+"  ·  updated "+relativeTime(pr.UpdatedAt), max(1, width-2))
	if selected {
		return []string{
			"  " + selectedStyle.Width(width).Render(first),
			"  " + selectedStyle.Bold(true).Width(width).Render(second),
			"  " + selectedStyle.Foreground(lipgloss.Color("#D0D7E7")).Width(width).Render(third),
		}
	}
	return []string{"  " + first, "  " + textStyle.Bold(true).Render(second), "  " + mutedStyle.Render(third)}
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
	help := "←/→ sections   r refresh   ? help   q quit"
	if m.detailOpen {
		switch {
		case m.mergeInProgress:
			help = "Merge in progress…   q quit"
		case m.mergeDialog == chooseMergeMethod:
			help = "↑/↓ choose method   enter continue   esc cancel"
		case m.mergeDialog == confirmMerge:
			help = "y confirm merge   esc cancel"
		default:
			help = "esc back   ↑/↓ scroll   PgUp/PgDn page   m merge"
			if m.hasReviewSidebar() {
				help = "tab sidebar/diff   j/k scroll   m merge   esc back"
			}
		}
	} else {
		switch m.activeSection {
		case 1:
			help = "enter details   ↑/↓ select   a/m/d filter   / search"
		case 2:
			help = "↑/↓ select   r refresh   ? help   q quit"
		case 3:
			help = "↑/↓ select   r refresh   ? help   q quit"
		}
	}
	if m.searching {
		help = "Type to search   enter/esc finish   backspace delete"
	}
	return "  " + mutedStyle.Render(clipLine(help, width))
}

func (m model) visibleListCount() int {
	reserved := 10 // header, navigation, toolbar, viewport footer, and help bar
	if m.query != "" || m.searching {
		reserved++
	}
	return max(1, (m.height-reserved)/3)
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
