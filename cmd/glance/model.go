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

var sections = []string{"Overview", "Pull requests", "Issues", "Notifications"}

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
	prDetails            githubapi.PullRequestDetails
	diffOpen             bool
	diffLoading          bool
	diffLoaded           bool
	diffRequestID        int
	diffError            string
	diffFiles            []githubapi.PullRequestFile
	selectedDiffFile     int
	diffScroll           int
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
	m.diffOpen = false
	m.diffLoading = false
	m.diffLoaded = false
	m.diffError = ""
	m.diffFiles = nil
	m.selectedDiffFile = 0
	m.diffScroll = 0
	m.detailRequestID++
	return m.loadPullRequestDetails()
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
		case "left", "h":
			m.mergeMethodIndex = (m.mergeMethodIndex + len(methods) - 1) % len(methods)
		case "right", "l":
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
		m.detailScroll = max(0, m.detailScroll-1)
	case "down", "j":
		m.detailScroll = min(m.maxDetailScroll(), m.detailScroll+1)
	case "home":
		m.detailScroll = 0
	case "end":
		m.detailScroll = m.maxDetailScroll()
	case "r", "R":
		if !m.detailLoading {
			m.detailLoading = true
			m.detailError = ""
			m.detailRequestID++
			return m, m.loadPullRequestDetails()
		}
	case "d", "D", "v", "V":
		return m, m.openPullRequestDiff()
	case "m", "M":
		m.detailScroll = 0
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
	visible := max(1, m.height-6)
	return max(0, len(m.pullRequestDetailsView(max(1, m.width-4)))-visible)
}

func (m *model) openPullRequestDiff() tea.Cmd {
	m.diffOpen = true
	m.diffScroll = 0
	if m.diffLoaded || m.diffLoading {
		return nil
	}
	m.diffLoading = true
	m.diffError = ""
	return m.loadPullRequestFiles()
}

func (m model) maxDiffScroll() int {
	visible := max(1, m.height-6)
	return max(0, len(m.pullRequestDiffView(max(1, m.width-4)))-visible)
}

func (m model) updatePullRequestDiffKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "esc":
		m.diffOpen = false
	case "?":
		m.showHelp = true
	case "up", "k":
		m.diffScroll = max(0, m.diffScroll-1)
	case "down", "j":
		m.diffScroll = min(m.maxDiffScroll(), m.diffScroll+1)
	case "home":
		m.diffScroll = 0
	case "end":
		m.diffScroll = m.maxDiffScroll()
	case "left", "h":
		if len(m.diffFiles) > 0 {
			m.selectedDiffFile = (m.selectedDiffFile + len(m.diffFiles) - 1) % len(m.diffFiles)
			m.diffScroll = 0
		}
	case "right", "l":
		if len(m.diffFiles) > 0 {
			m.selectedDiffFile = (m.selectedDiffFile + 1) % len(m.diffFiles)
			m.diffScroll = 0
		}
	case "r", "R":
		if !m.diffLoading {
			m.diffLoading = true
			m.diffLoaded = false
			m.diffError = ""
			m.diffFiles = nil
			m.selectedDiffFile = 0
			m.diffScroll = 0
			return m, m.loadPullRequestFiles()
		}
	}
	return m, nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
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
	case pullRequestDetailsLoadedMsg:
		if msg.requestID != m.detailRequestID || msg.key != m.detailKey {
			break
		}
		m.detailLoading = false
		if msg.err != nil {
			m.detailError = safeText(msg.err.Error())
			break
		}
		m.prDetails = msg.details
		m.detailError = ""
		m.detailScroll = min(m.detailScroll, m.maxDetailScroll())
	case pullRequestFilesLoadedMsg:
		if msg.requestID != m.diffRequestID || msg.key != m.detailKey {
			break
		}
		m.diffLoading = false
		if msg.err != nil {
			m.diffError = safeText(msg.err.Error())
			m.diffLoaded = false
			break
		}
		m.diffFiles = msg.files
		m.diffLoaded = true
		m.diffError = ""
		m.selectedDiffFile = min(max(0, m.selectedDiffFile), max(0, len(m.diffFiles)-1))
		m.diffScroll = min(m.diffScroll, m.maxDiffScroll())
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
		m.loading = true
		m.requestID++
		m.detailLoading = true
		m.detailRequestID++
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
			return m, nil
		}
		if m.showHelp {
			if msg.String() == "?" || msg.String() == "esc" || msg.String() == "enter" {
				m.showHelp = false
			}
			if msg.String() == "q" || msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}
		if m.diffOpen {
			return m.updatePullRequestDiffKey(msg)
		}
		if m.detailOpen {
			return m.updatePullRequestDetailKey(msg)
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
			if m.activeSection == 1 {
				return m, m.openSelectedPullRequest()
			}
		case "?":
			m.showHelp = true
		case "/":
			if m.activeSection == 1 {
				m.searching = true
			}
		case "esc":
			if m.activeSection == 1 {
				m.query = ""
				m.filter = allPRs
				m.selectedPR = 0
			}
		case "a", "m", "d":
			if m.activeSection == 1 {
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
			m.loading = true
			m.apiError = ""
			m.issueError = ""
			m.notificationError = ""
			m.requestID++
			return m, m.loadDashboard()
		}
	}

	return m, nil
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
	case 1:
		return len(m.filteredPRs())
	case 2:
		return len(m.issues)
	case 3:
		return len(m.notifications)
	default:
		return 0
	}
}

func (m *model) moveSelection(delta int) {
	switch m.activeSection {
	case 1:
		m.selectedPR += delta
		m.clampSelectedPR()
	case 2:
		m.selectedIssue = min(max(0, m.selectedIssue+delta), max(0, len(m.issues)-1))
	case 3:
		m.selectedNotification = min(max(0, m.selectedNotification+delta), max(0, len(m.notifications)-1))
	}
}

func (m *model) setSelection(index int) {
	switch m.activeSection {
	case 1:
		m.selectedPR = index
		m.clampSelectedPR()
	case 2:
		m.selectedIssue = min(max(0, index), max(0, len(m.issues)-1))
	case 3:
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
