package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"glance/internal/githubapi"

	tea "github.com/charmbracelet/bubbletea"
)

var sections = []string{"Pull requests", "Issues", "Notifications"}

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
	detailOpen           bool
	detailLoading        bool
	detailRequestID      int
	detailKey            string
	detailError          string
	detailScroll         int
	detailLines          []string
	detailLinesWidth     int
	sidebarScroll        int
	sidebarFocus         bool
	prDetails            githubapi.PullRequestDetails
	previewCache         map[string]githubapi.PullRequestDetails
	previewErrors        map[string]string
	previewPending       map[string]bool
	previewOrder         []string
	previewEpoch         int
	diffLoading          bool
	diffLoaded           bool
	diffRequestID        int
	diffError            string
	diffFiles            []githubapi.PullRequestFile
	mergeDialog          mergeDialogState
	mergeMethodIndex     int
	mergeMethod          githubapi.MergeMethod
	mergeInProgress      bool
	mergeRequestID       int
	mergeError           string
	mergeNotice          string
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

type pullRequestDetailsLoadedMsg struct {
	requestID int
	key       string
	details   githubapi.PullRequestDetails
	err       error
}

type pullRequestFilesLoadedMsg struct {
	requestID int
	key       string
	files     []githubapi.PullRequestFile
	err       error
}

type pullRequestMergedMsg struct {
	requestID int
	key       string
	result    githubapi.MergeResult
	err       error
}

type mergeDialogState int

const (
	noMergeDialog mergeDialogState = iota
	chooseMergeMethod
	confirmMerge
)

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

func (m *model) openSelectedPullRequest() tea.Cmd {
	items := m.filteredPRs()
	if m.selectedPR < 0 || m.selectedPR >= len(items) {
		return nil
	}
	pr := items[m.selectedPR]
	m.detailKey = fmt.Sprintf("%s#%d", pr.Repository, pr.Number)
	m.detailOpen = true
	m.detailLoading = true
	m.detailError = ""
	m.prDetails = githubapi.PullRequestDetails{}
	m.mergeError = ""
	m.mergeNotice = ""
	m.mergeDialog = noMergeDialog
	m.mergeInProgress = false
	m.detailScroll = 0
	m.detailLines = nil
	m.sidebarScroll = 0
	m.sidebarFocus = false
	m.diffLoading = true
	m.diffLoaded = false
	m.diffError = ""
	m.diffFiles = nil
	m.detailRequestID++
	return tea.Batch(m.loadPullRequestDetails(), m.loadPullRequestFiles())
}

func (m model) loadPullRequestDetails() tea.Cmd {
	requestID, key, client := m.detailRequestID, m.detailKey, m.client
	owner, repository, number, err := parsePullRequestKey(key)
	return func() tea.Msg {
		if err != nil {
			return pullRequestDetailsLoadedMsg{requestID: requestID, key: key, err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), dashboardLoadTimeout)
		defer cancel()
		details, err := client.GetPullRequestDetails(ctx, owner, repository, number)
		return pullRequestDetailsLoadedMsg{requestID: requestID, key: key, details: details, err: err}
	}
}

func (m *model) loadPullRequestFiles() tea.Cmd {
	m.diffRequestID++
	requestID, key, client := m.diffRequestID, m.detailKey, m.client
	owner, repository, number, err := parsePullRequestKey(key)
	return func() tea.Msg {
		if err != nil {
			return pullRequestFilesLoadedMsg{requestID: requestID, key: key, err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), dashboardLoadTimeout)
		defer cancel()
		files, err := client.ListPullRequestFiles(ctx, owner, repository, number)
		return pullRequestFilesLoadedMsg{requestID: requestID, key: key, files: files, err: err}
	}
}

func (m *model) mergePullRequest(method githubapi.MergeMethod) tea.Cmd {
	m.mergeRequestID++
	requestID, key, sha, client := m.mergeRequestID, m.detailKey, m.prDetails.Head.SHA, m.client
	owner, repository, number, err := parsePullRequestKey(key)
	m.mergeInProgress = true
	m.mergeDialog = noMergeDialog
	m.mergeError = ""
	m.mergeNotice = ""
	return func() tea.Msg {
		if err != nil {
			return pullRequestMergedMsg{requestID: requestID, key: key, err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), dashboardLoadTimeout)
		defer cancel()
		result, err := client.MergePullRequest(ctx, owner, repository, number, method, sha)
		return pullRequestMergedMsg{requestID: requestID, key: key, result: result, err: err}
	}
}

func parsePullRequestKey(key string) (owner, repository string, number int, err error) {
	repo, numberText, ok := strings.Cut(key, "#")
	if !ok {
		return "", "", 0, fmt.Errorf("invalid pull request reference %q", key)
	}
	owner, repository, ok = strings.Cut(repo, "/")
	if !ok || owner == "" || repository == "" {
		return "", "", 0, fmt.Errorf("invalid pull request repository %q", repo)
	}
	number, err = strconv.Atoi(numberText)
	if err != nil || number < 1 {
		return "", "", 0, fmt.Errorf("invalid pull request number %q", numberText)
	}
	return owner, repository, number, nil
}

func (m model) updatePullRequestDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" || key == "q" {
		return m, tea.Quit
	}
	if m.mergeInProgress {
		return m, nil
	}
	if m.mergeDialog == chooseMergeMethod {
		methods := m.prDetails.AllowedMergeMethods
		if len(methods) == 0 {
			m.mergeDialog = noMergeDialog
			m.mergeError = "No merge methods are enabled for this repository."
			return m, nil
		}
		switch key {
		case "esc":
			m.mergeDialog = noMergeDialog
		case "up", "k":
			m.mergeMethodIndex = (m.mergeMethodIndex + len(methods) - 1) % len(methods)
		case "down", "j":
			m.mergeMethodIndex = (m.mergeMethodIndex + 1) % len(methods)
		case "enter":
			if len(methods) > 0 {
				m.mergeMethod = methods[m.mergeMethodIndex]
				m.mergeDialog = confirmMerge
			}
		case "1", "2", "3":
			index, _ := strconv.Atoi(key)
			index--
			if index >= 0 && index < len(methods) {
				m.mergeMethodIndex = index
			}
		}
		return m, nil
	}
	if m.mergeDialog == confirmMerge {
		switch key {
		case "esc", "n", "N":
			m.mergeDialog = noMergeDialog
		case "y", "Y":
			return m, m.mergePullRequest(m.mergeMethod)
		}
		return m, nil
	}
	switch key {
	case "esc":
		m.detailOpen = false
		m.detailScroll = 0
	case "up", "k":
		if m.sidebarFocus {
			m.sidebarScroll = max(0, m.sidebarScroll-1)
		} else {
			m.detailScroll = max(0, m.detailScroll-1)
		}
	case "down", "j":
		if m.sidebarFocus {
			m.sidebarScroll = min(m.maxSidebarScroll(), m.sidebarScroll+1)
		} else {
			m.detailScroll = min(m.maxDetailScroll(), m.detailScroll+1)
		}
	case "pgup", "ctrl+u":
		if m.sidebarFocus {
			m.sidebarScroll = max(0, m.sidebarScroll-m.detailViewportHeight())
		} else {
			m.detailScroll = max(0, m.detailScroll-m.detailViewportHeight())
		}
	case "pgdown", "ctrl+d":
		if m.sidebarFocus {
			m.sidebarScroll = min(m.maxSidebarScroll(), m.sidebarScroll+m.detailViewportHeight())
		} else {
			m.detailScroll = min(m.maxDetailScroll(), m.detailScroll+m.detailViewportHeight())
		}
	case "home":
		if m.sidebarFocus {
			m.sidebarScroll = 0
		} else {
			m.detailScroll = 0
		}
	case "end":
		if m.sidebarFocus {
			m.sidebarScroll = m.maxSidebarScroll()
		} else {
			m.detailScroll = m.maxDetailScroll()
		}
	case "tab":
		if m.hasReviewSidebar() {
			m.sidebarFocus = !m.sidebarFocus
		}
	case "r", "R":
		if !m.detailLoading && !m.diffLoading {
			m.detailLines = nil
			m.detailLoading = true
			m.detailError = ""
			m.diffLoading = true
			m.diffLoaded = false
			m.diffError = ""
			m.diffFiles = nil
			m.detailScroll = 0
			m.detailRequestID++
			m.cacheDetailLines()
			return m, tea.Batch(m.loadPullRequestDetails(), m.loadPullRequestFiles())
		}
	case "m", "M":
		switch {
		case m.detailLoading:
			m.mergeError = "Wait for the pull request details to finish loading before merging."
		case m.detailError != "":
			m.mergeError = "Refresh the pull request details before merging."
		case m.prDetails.State != "open" || m.prDetails.Merged:
			m.mergeError = "Only open, unmerged pull requests can be merged."
		case m.prDetails.Draft:
			m.mergeError = "Mark this pull request ready for review before merging."
		case m.prDetails.Head.SHA == "":
			m.mergeError = "GitHub did not provide a head SHA; refresh before merging."
		case m.prDetails.Mergeable != nil && !*m.prDetails.Mergeable:
			m.mergeError = "GitHub reports this pull request is not mergeable; check for conflicts."
		case len(m.prDetails.AllowedMergeMethods) == 0:
			if m.prDetails.MergeMethodsError != "" {
				m.mergeError = "Could not load repository merge settings: " + safeText(m.prDetails.MergeMethodsError)
			} else {
				m.mergeError = "This repository has no enabled merge methods."
			}
		default:
			m.mergeError = ""
			m.mergeMethodIndex = 0
			m.mergeDialog = chooseMergeMethod
		}
	}
	return m, nil
}

func (m model) maxDetailScroll() int {
	visible := m.detailViewportHeight()
	left, _ := m.reviewWidths(max(1, m.width-4))
	return max(0, len(m.pullRequestDetailsView(left))-visible)
}

// cacheDetailLines does the expensive Markdown and syntax highlighting only
// when the PR data or review-column width changes, never for a scroll key.
func (m *model) cacheDetailLines() {
	if !m.detailOpen {
		return
	}
	width, _ := m.reviewWidths(max(1, m.width-4))
	if m.detailLines != nil && m.detailLinesWidth == width {
		return
	}
	m.detailLines = m.renderPullRequestDetails(width)
	m.detailLinesWidth = width
}

func (m model) detailViewportHeight() int {
	return max(1, m.height-3-len(m.mergeActionPanel(max(1, m.width-4))))
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.detailOpen {
			m.cacheDetailLines()
		}
		if !m.hasReviewSidebar() {
			m.sidebarFocus = false
			m.sidebarScroll = 0
		} else {
			m.sidebarScroll = min(m.sidebarScroll, m.maxSidebarScroll())
		}
		if m.detailOpen {
			m.detailScroll = min(m.detailScroll, m.maxDetailScroll())
		}
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
	case previewReadyMsg:
		if msg.epoch != m.previewEpoch || m.selectedKey() != msg.key || m.width < 80 || m.height < 20 || m.activeSection != 0 || m.detailOpen || m.showHelp || m.searching {
			break
		}
		if m.previewPending[msg.key] || m.previewErrors[msg.key] != "" {
			break
		}
		if _, cached := m.previewCache[msg.key]; cached {
			break
		}
		if m.previewPending == nil {
			m.previewPending = make(map[string]bool)
		}
		m.previewPending[msg.key] = true
		return m, m.loadPRPreview(msg.key, msg.epoch)
	case previewLoadedMsg:
		if msg.epoch != m.previewEpoch {
			break
		}
		delete(m.previewPending, msg.key)
		if msg.err != nil {
			if m.previewErrors == nil {
				m.previewErrors = make(map[string]string)
			}
			m.previewErrors[msg.key] = safeText(msg.err.Error())
			break
		}
		m.storePRPreview(msg.key, msg.details)
	case pullRequestDetailsLoadedMsg:
		if msg.requestID != m.detailRequestID || msg.key != m.detailKey {
			break
		}
		m.detailLoading = false
		if msg.err != nil {
			m.detailError = safeText(msg.err.Error())
			m.detailLines = nil
			m.cacheDetailLines()
			break
		}
		m.prDetails = msg.details
		m.storePRPreview(msg.key, msg.details)
		m.detailError = ""
		m.detailLines = nil
		m.cacheDetailLines()
		m.detailScroll = min(m.detailScroll, m.maxDetailScroll())
		m.sidebarScroll = min(m.sidebarScroll, m.maxSidebarScroll())
	case pullRequestFilesLoadedMsg:
		if msg.requestID != m.diffRequestID || msg.key != m.detailKey {
			break
		}
		m.diffLoading = false
		if msg.err != nil {
			m.diffError = safeText(msg.err.Error())
			m.diffLoaded = false
			m.detailLines = nil
			m.cacheDetailLines()
			break
		}
		m.diffFiles = msg.files
		m.diffLoaded = true
		m.diffError = ""
		m.detailLines = nil
		m.cacheDetailLines()
		m.detailScroll = min(m.detailScroll, m.maxDetailScroll())
	case pullRequestMergedMsg:
		if msg.requestID != m.mergeRequestID || msg.key != m.detailKey {
			break
		}
		m.mergeInProgress = false
		if msg.err != nil {
			m.mergeError = safeText(msg.err.Error())
			break
		}
		m.mergeNotice = "Merged successfully using " + strings.ToLower(string(m.mergeMethod)) + "."
		m.clearPRPreviews()
		m.loading = true
		m.requestID++
		m.detailLoading = true
		m.detailRequestID++
		m.detailLines = nil
		m.cacheDetailLines()
		return m, tea.Batch(m.loadDashboard(), m.loadPullRequestDetails())
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
			return m, m.schedulePRPreview()
		}
		if m.showHelp {
			if msg.String() == "?" || msg.String() == "esc" || msg.String() == "enter" {
				m.showHelp = false
			}
			if msg.String() == "q" || msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, m.schedulePRPreview()
		}
		if m.detailOpen {
			updated, cmd := m.updatePullRequestDetailKey(msg)
			if !m.detailOpen && cmd == nil {
				return m, m.schedulePRPreview()
			}
			return updated, cmd
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
		case "enter":
			if m.activeSection == 0 {
				return m, m.openSelectedPullRequest()
			}
		case "?":
			m.showHelp = true
		case "/":
			if m.activeSection == 0 {
				m.searching = true
			}
		case "esc":
			if m.activeSection == 0 {
				m.query = ""
				m.filter = allPRs
				m.selectedPR = 0
			}
		case "a", "m", "d":
			if m.activeSection == 0 {
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
			m.clearPRPreviews()
			m.loading = true
			m.apiError = ""
			m.issueError = ""
			m.notificationError = ""
			m.requestID++
			return m, m.loadDashboard()
		}
	}

	return m, m.schedulePRPreview()
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
	case 0:
		return len(m.filteredPRs())
	case 1:
		return len(m.issues)
	case 2:
		return len(m.notifications)
	default:
		return 0
	}
}

func (m *model) moveSelection(delta int) {
	switch m.activeSection {
	case 0:
		m.selectedPR += delta
		m.clampSelectedPR()
	case 1:
		m.selectedIssue = min(max(0, m.selectedIssue+delta), max(0, len(m.issues)-1))
	case 2:
		m.selectedNotification = min(max(0, m.selectedNotification+delta), max(0, len(m.notifications)-1))
	}
}

func (m *model) setSelection(index int) {
	switch m.activeSection {
	case 0:
		m.selectedPR = index
		m.clampSelectedPR()
	case 1:
		m.selectedIssue = min(max(0, index), max(0, len(m.issues)-1))
	case 2:
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
