package main

import (
	"fmt"
	"strings"
	"unicode"

	"glance/internal/githubapi"

	"github.com/charmbracelet/lipgloss"
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
	unreadBadge   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D6C5FF")).Background(lipgloss.Color("#443460"))
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
		content := m.pullRequestDetailsView(inner)
		visible := max(1, m.height-len(lines)-2)
		start := min(max(0, m.detailScroll), max(0, len(content)-visible))
		end := min(len(content), start+visible)
		lines = append(lines, content[start:end]...)
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

	lines = append(lines, "", m.footer(inner))
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
		"enter        Open PR details", "m            Merge from PR details",
		"/            Search PRs", "a / m / d    All / mine / drafts",
		"esc          Clear search & filter", "r            Refresh", "?            Close help", "q            Quit",
	}
	if m.height < 19 {
		shortcuts = []string{"←/→ sections   ↑/↓ select", "enter details   m merge", "/ search   a/m/d filters", "r refresh   ? close   q quit"}
	}
	lines := []string{"  " + accentStyle.Render("KEYBOARD SHORTCUTS"), ""}
	for _, shortcut := range shortcuts {
		lines = append(lines, "  "+textStyle.Render(clipLine(shortcut, width)))
	}
	return lines
}

func (m model) pullRequestView(width int) []string {
	items := m.filteredPRs()
	heading := "PULL REQUESTS"
	if m.loading && len(m.pullRequests) > 0 {
		heading += "  ↻ Refreshing"
	}
	lines := []string{
		"  " + align(accentStyle.Render(clipLine(heading, width-8)), mutedStyle.Render(fmt.Sprintf("%d open", len(m.pullRequests))), width),
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
	lines = append(lines, "  "+mutedStyle.Render(clipLine(fmt.Sprintf("%d–%d of %d  ·  j/k move  ·  / search", start+1, end, len(items)), width)))
	return lines
}

func (m model) pullRequestDetailsView(width int) []string {
	if m.prDetails.Number == 0 {
		if m.detailLoading {
			return m.message(width, "Loading pull request", "Fetching pull request details, reviews, and checks...", "This can take a moment.")
		}
		if m.detailError != "" {
			return m.message(width, "Could not load pull request", m.detailError, "Press r to try again, or esc to return.")
		}
		return m.message(width, "Pull request unavailable", "No pull request details are loaded.", "Press r to try again, or esc to return.")
	}
	details := m.prDetails
	state := strings.ToUpper(details.State)
	if details.Merged {
		state = "MERGED"
	} else if details.Draft {
		state += " · DRAFT"
	}
	lines := []string{
		"  " + accentStyle.Render(clipLine(fmt.Sprintf("%s#%d  ·  %s", safeText(details.Repository), details.Number, safeText(details.Title)), width)),
		"  " + goodStyle.Render(clipLine(state, width)),
	}
	if m.detailLoading {
		lines = append(lines, "  "+warnStyle.Render("Refreshing pull request details…"))
	}
	if m.detailError != "" {
		lines = append(lines, "  "+errorStyle.Render(clipLine("Refresh failed: "+m.detailError, width)))
	}
	if m.mergeNotice != "" {
		lines = append(lines, "  "+goodStyle.Render(clipLine(m.mergeNotice, width)))
	}
	if m.mergeError != "" {
		lines = append(lines, "  "+errorStyle.Render(clipLine(m.mergeError, width)))
	}
	if m.prDetails.MergeMethodsError != "" {
		lines = append(lines, "  "+warnStyle.Render(clipLine("Merge methods unavailable: "+safeText(m.prDetails.MergeMethodsError), width)))
	}
	if m.mergeDialog == chooseMergeMethod {
		lines = append(lines, "", "  "+accentStyle.Render("Choose merge method"))
		for i, method := range details.AllowedMergeMethods {
			label := mergeMethodLabel(method)
			if i == m.mergeMethodIndex {
				lines = append(lines, "  "+activeStyle.Render(" "+label+" "))
			} else {
				lines = append(lines, "  "+mutedStyle.Render(label))
			}
		}
		lines = append(lines, "  "+mutedStyle.Render(clipLine("←/→ choose · enter continue · esc cancel", width)))
	}
	if m.mergeDialog == confirmMerge {
		lines = append(lines, "", "  "+warnStyle.Render("Confirm merge"),
			"  "+clipLine(fmt.Sprintf("Merge %s#%d using %s?", details.Repository, details.Number, mergeMethodLabel(m.mergeMethod)), width),
			"  "+errorStyle.Render(clipLine("This changes the repository. Press y to confirm or esc to cancel.", width)))
	}
	if m.mergeInProgress {
		lines = append(lines, "  "+warnStyle.Render("Merging pull request…"))
	}
	lines = append(lines,
		"  "+mutedStyle.Render(clipLine("Author: @"+safeText(details.User.Login), width)),
		"  "+mutedStyle.Render(clipLine(fmt.Sprintf("Branches: %s  →  %s", safeText(details.Head.Ref), safeText(details.Base.Ref)), width)),
		"  "+mutedStyle.Render(clipLine("Updated "+relativeTime(details.UpdatedAt), width)),
	)
	if details.Mergeable == nil {
		lines = append(lines, "  "+warnStyle.Render(clipLine("Mergeability: checking ("+safeText(details.MergeableState)+")", width)))
	} else if *details.Mergeable {
		lines = append(lines, "  "+goodStyle.Render(clipLine("Mergeability: mergeable ("+safeText(details.MergeableState)+")", width)))
	} else {
		lines = append(lines, "  "+warnStyle.Render(clipLine("Mergeability: blocked ("+safeText(details.MergeableState)+")", width)))
	}
	lines = append(lines, "  "+accentStyle.Render("Requested reviewers"))
	if len(details.RequestedReviewers) == 0 {
		lines = append(lines, "  "+mutedStyle.Render("None"))
	} else {
		reviewers := make([]string, 0, len(details.RequestedReviewers))
		for _, reviewer := range details.RequestedReviewers {
			reviewers = append(reviewers, "@"+safeText(reviewer.Login))
		}
		lines = append(lines, "  "+mutedStyle.Render(clipLine(strings.Join(reviewers, ", "), width)))
	}
	lines = append(lines, "  "+accentStyle.Render("Reviews"))
	if details.ReviewsError != "" {
		lines = append(lines, "  "+errorStyle.Render(clipLine("Unavailable: "+safeText(details.ReviewsError), width)))
	} else if len(details.Reviews) == 0 {
		lines = append(lines, "  "+mutedStyle.Render("No reviews submitted"))
	} else {
		seen := make(map[string]bool)
		count := 0
		for _, review := range details.Reviews {
			login := safeText(review.User.Login)
			if seen[strings.ToLower(login)] {
				continue
			}
			seen[strings.ToLower(login)] = true
			lines = append(lines, "  "+mutedStyle.Render(clipLine("@"+login+": "+strings.ToUpper(safeText(review.State)), width)))
			count++
			if count == 5 {
				break
			}
		}
	}
	lines = append(lines, "  "+accentStyle.Render(fmt.Sprintf("Checks (%d)", len(details.CheckRuns))))
	if details.ChecksError != "" {
		lines = append(lines, "  "+errorStyle.Render(clipLine("Unavailable: "+safeText(details.ChecksError), width)))
	} else if len(details.CheckRuns) == 0 {
		lines = append(lines, "  "+mutedStyle.Render("No checks reported"))
	} else {
		for i, check := range details.CheckRuns {
			if i == 8 {
				lines = append(lines, "  "+mutedStyle.Render(fmt.Sprintf("… and %d more", len(details.CheckRuns)-i)))
				break
			}
			status := check.Conclusion
			if status == "" {
				status = check.Status
			}
			style := warnStyle
			switch status {
			case "success", "neutral", "skipped":
				style = goodStyle
			case "failure", "timed_out", "action_required", "cancelled":
				style = errorStyle
			}
			lines = append(lines, "  "+style.Render(clipLine(safeText(check.Name)+": "+strings.ToUpper(safeText(status)), width)))
		}
	}
	lines = append(lines, "  "+accentStyle.Render("Description"))
	description := strings.TrimSpace(details.Body)
	if description == "" {
		lines = append(lines, "  "+mutedStyle.Render("(No description.)"))
	} else {
		wrapped := lipgloss.NewStyle().Width(max(1, width-2)).Render(safeMultiline(description))
		for _, line := range strings.Split(wrapped, "\n") {
			lines = append(lines, "  "+line)
		}
	}
	return lines
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
	// Reserve room for the marker and keep the repository and PR number visible.
	label := fmt.Sprintf("%s#%d", safeText(pr.Repository), pr.Number)
	if width >= 60 {
		age := relativeTime(pr.UpdatedAt)
		if !selected {
			age = mutedStyle.Render(age)
		}
		label = align(clipLine(label, width-25), state+" "+age, width-2)
	} else {
		label = clipLine(label, max(1, width-lipgloss.Width(state)-3)) + " " + state
	}
	first := marker + label
	second := marker + clipLine(safeText(pr.Title), width-2)
	third := marker + clipLine("@"+safeText(pr.User.Login)+"  ·  updated "+relativeTime(pr.UpdatedAt), width-2)
	if selected {
		row := []string{first, second, third}
		for i := range row {
			row[i] = "  " + selectedStyle.Width(width).Render(row[i])
		}
		return row
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
			help = "←/→ choose method   enter continue   esc cancel"
		case m.mergeDialog == confirmMerge:
			help = "y confirm merge   esc cancel"
		default:
			help = "esc back   ↑/↓ scroll   m merge   r refresh   ? help"
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
