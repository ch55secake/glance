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
const defaultPRQuery = "is:pr is:open author:@me"

type model struct {
	width                int
	height               int
	activeSection        int
	theme                string
	themePickerOpen      bool
	themePickerSelection int
	themeConfigError     string
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
	inboxPreviewFocus    bool
	inboxPreviewScroll   int
	inboxLines           []string
	inboxLinesKey        string
	inboxLinesWidth      int
	query                string
	searchDraft          string
	searchCursor         int
	searchOffset         int
	searching            bool
	showHelp             bool
	helpScroll           int
	paletteOpen          bool
	paletteQuery         string
	paletteSelection     int
	requestID            int
	prRequestID          int
	prLoading            bool
	detailOpen           bool
	detailLoading        bool
	detailRequestID      int
	detailKey            string
	detailError          string
	detailScroll         int
	detailLines          []string
	detailLinesWidth     int
	reviewFiles          []reviewFileAnchor
	reviewedFiles        map[string]reviewedFileSnapshot
	diffFileSnapshots    []reviewedFileSnapshot
	pendingReviewPath    string
	pendingReviewOffset  int
	filePickerOpen       bool
	filePickerQuery      string
	filePickerSelection  int
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
	commentComposerOpen  bool
	commentDraft         string
	commentCursor        int
	commentScroll        int
	commentSending       bool
	commentRequestID     int
	commentError         string
	commentNotice        string
	client               *githubapi.Client
}

type dashboardLoadedMsg struct {
	requestID       int
	prRequestID     int
	viewer          githubapi.User
	pullRequests    []githubapi.PullRequest
	issues          []githubapi.Issue
	notifications   []githubapi.Notification
	err             error
	pullRequestErr  error
	issueErr        error
	notificationErr error
}

type pullRequestsSearchedMsg struct {
	requestID    int
	pullRequests []githubapi.PullRequest
	err          error
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

type pullRequestCommentPostedMsg struct {
	requestID int
	key       string
	comment   githubapi.PullRequestComment
	err       error
}

type mergeDialogState int

const (
	noMergeDialog mergeDialogState = iota
	chooseMergeMethod
	confirmMerge
)

func initialModel() model {
	applyTheme(defaultThemeID)
	return model{width: 80, height: 24, loading: true, query: defaultPRQuery, theme: defaultThemeID, client: githubapi.NewClient()}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.loadDashboard(), scheduleAutoRefresh())
}

func (m model) loadDashboard() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), dashboardLoadTimeout)
		defer cancel()

		result := dashboardLoadedMsg{requestID: m.requestID, prRequestID: m.prRequestID}
		viewer, err := m.client.Viewer(ctx)
		if err != nil {
			result.err = err
			return result
		}
		result.viewer = viewer
		var requests sync.WaitGroup
		requests.Add(3)
		go func() {
			defer requests.Done()
			result.pullRequests, result.pullRequestErr = m.client.SearchPullRequests(ctx, m.query)
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

func (m *model) startDashboardRefresh() tea.Cmd {
	m.clearPRPreviews()
	m.loading = true
	m.prLoading = true
	m.apiError = ""
	m.issueError = ""
	m.notificationError = ""
	m.requestID++
	m.prRequestID++
	return m.loadDashboard()
}

func (m model) searchPullRequests() tea.Cmd {
	requestID, query, client := m.prRequestID, m.query, m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), dashboardLoadTimeout)
		defer cancel()
		results, err := client.SearchPullRequests(ctx, query)
		return pullRequestsSearchedMsg{requestID: requestID, pullRequests: results, err: err}
	}
}

func (m *model) openSelectedPullRequest() tea.Cmd {
	items := m.pullRequests
	if m.selectedPR < 0 || m.selectedPR >= len(items) {
		return nil
	}
	pr := items[m.selectedPR]
	m.detailKey = fmt.Sprintf("%s#%d", pr.Repository, pr.Number)
	m.commentRequestID++
	m.detailOpen = true
	m.detailLoading = true
	m.detailError = ""
	m.prDetails = githubapi.PullRequestDetails{}
	m.mergeError = ""
	m.mergeNotice = ""
	m.commentComposerOpen = false
	m.commentDraft = ""
	m.commentCursor = 0
	m.commentScroll = 0
	m.commentSending = false
	m.commentError = ""
	m.commentNotice = ""
	m.mergeDialog = noMergeDialog
	m.mergeInProgress = false
	m.detailScroll = 0
	m.detailLines = nil
	m.reviewFiles = nil
	m.reviewedFiles = nil
	m.diffFileSnapshots = nil
	m.pendingReviewPath = ""
	m.pendingReviewOffset = 0
	m.filePickerOpen = false
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
	m.cacheDetailLines()
	switch key {
	case "esc":
		m.detailOpen = false
		m.detailScroll = 0
	case "?":
		m.showHelp = true
		m.helpScroll = 0
	case "1", "2", "3":
		section, _ := strconv.Atoi(key)
		m.switchSection(section - 1)
	case "/":
		m.beginSearch()
	case "t":
		m.openThemePicker()
	case "f":
		m.openFilePicker()
	case "]":
		m.stepReviewFile(1)
	case "[":
		m.stepReviewFile(-1)
	case "d":
		m.jumpToReviewFile(0)
	case "v":
		m.toggleReviewedFile()
	case "c":
		if m.canCommentOnPullRequest() {
			m.openCommentComposer()
		}
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
	case "tab", "shift+tab":
		if m.hasReviewSidebar() {
			m.sidebarFocus = !m.sidebarFocus
		}
	case "r", "R":
		if !m.detailLoading && !m.diffLoading {
			if path, offset := m.reviewPosition(); path != "" {
				m.pendingReviewPath, m.pendingReviewOffset = path, offset
			}
			m.detailLines = nil
			m.detailLoading = true
			m.detailError = ""
			m.diffLoading = true
			m.diffLoaded = false
			m.diffError = ""
			m.diffFiles = nil
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
	lastFile := 0
	if len(m.reviewFiles) > 0 {
		lastFile = m.reviewFiles[len(m.reviewFiles)-1].row
	}
	return max(lastFile, len(m.pullRequestDetailsView(left))-visible, 0)
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
	document := m.renderReview(width)
	m.detailLines, m.reviewFiles = document.lines, document.files
	m.detailLinesWidth = width
}

func (m model) detailViewportHeight() int {
	return max(1, m.height-3-len(m.mergeActionPanel(max(1, m.width-4))))
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	selection := m.inboxSelectionKey()
	updated, cmd := m.update(msg)
	next := updated.(model)
	if selection != next.inboxSelectionKey() {
		next.inboxPreviewScroll = 0
		next.inboxLines = nil
	}
	if next.hasInboxPreview() && !next.detailOpen && !next.paletteOpen && !next.showHelp {
		next.cacheInboxPreview()
	}
	if !next.hasInboxPreview() {
		next.inboxPreviewFocus = false
		next.inboxPreviewScroll = 0
	} else if _, key := msg.(tea.KeyMsg); !key && next.inboxPreviewScroll > 0 {
		next.inboxPreviewScroll = min(next.inboxPreviewScroll, next.maxInboxPreviewScroll())
	}
	if next.showHelp {
		next.helpScroll = min(next.helpScroll, next.maxHelpScroll())
	}
	return next, cmd
}

func (m model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		file, offset := m.pendingReviewPath, m.pendingReviewOffset
		if file == "" {
			file, offset = m.reviewPosition()
		}
		m.width = msg.Width
		m.height = msg.Height
		if m.commentComposerOpen {
			m.keepCommentCursorVisible()
		}
		if m.searching {
			m.keepSearchCursorVisible()
		}
		if m.detailOpen {
			m.cacheDetailLines()
			m.restoreReviewPosition(file, offset)
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
		m.inboxLines = nil
		selectedPR := m.selectedKey()
		selectedIssue := m.selectedIssueKey()
		selectedNotification := m.selectedNotificationID()
		m.loading = false
		if msg.err != nil {
			if msg.prRequestID == m.prRequestID {
				m.prLoading = false
			}
			m.apiError = safeText(msg.err.Error())
			m.issueError = m.apiError
			m.notificationError = m.apiError
		} else {
			m.viewer = msg.viewer
			m.issueError = errorString(msg.issueErr)
			m.notificationError = errorString(msg.notificationErr)
			if msg.prRequestID == m.prRequestID {
				m.prLoading = false
				m.apiError = errorString(msg.pullRequestErr)
				if msg.pullRequestErr == nil {
					m.pullRequests = msg.pullRequests
				}
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
	case pullRequestsSearchedMsg:
		if msg.requestID != m.prRequestID {
			break
		}
		m.inboxLines = nil
		m.prLoading = false
		m.apiError = errorString(msg.err)
		if msg.err == nil {
			m.pullRequests = msg.pullRequests
			m.selectedPR = 0
		}
	case previewReadyMsg:
		if msg.epoch != m.previewEpoch || m.selectedKey() != msg.key || m.prLoading || !m.hasInboxPreview() || m.height < 20 && !m.inboxPreviewFocus || m.activeSection != 0 || m.detailOpen || m.showHelp || m.paletteOpen || m.searching {
			break
		}
		if len(m.previewPending) >= 2 || m.previewPending[msg.key] || m.previewErrors[msg.key] != "" {
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
			m.inboxLines = nil
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
		file, offset := m.reviewPosition()
		m.detailLoading = false
		if msg.err != nil {
			m.detailError = safeText(msg.err.Error())
			m.detailLines = nil
			m.cacheDetailLines()
			m.restoreReviewPosition(file, offset)
			m.detailScroll = min(m.detailScroll, m.maxDetailScroll())
			break
		}
		m.prDetails = msg.details
		m.storePRPreview(msg.key, msg.details)
		m.detailError = ""
		m.detailLines = nil
		m.cacheDetailLines()
		m.restoreReviewPosition(file, offset)
		m.detailScroll = min(m.detailScroll, m.maxDetailScroll())
		m.sidebarScroll = min(m.sidebarScroll, m.maxSidebarScroll())
	case pullRequestFilesLoadedMsg:
		if msg.requestID != m.diffRequestID || msg.key != m.detailKey {
			break
		}
		file, offset := m.pendingReviewPath, m.pendingReviewOffset
		if file == "" {
			file, offset = m.reviewPosition()
		}
		pickerPath := m.filePickerSelectedPath()
		m.diffLoading = false
		if msg.err != nil {
			m.diffError = safeText(msg.err.Error())
			m.diffLoaded = false
			m.detailLines = nil
			m.cacheDetailLines()
			m.restoreReviewPosition(file, offset)
			m.detailScroll = min(m.detailScroll, m.maxDetailScroll())
			break
		}
		m.retainViewedFiles(msg.files)
		m.diffFiles = msg.files
		m.diffLoaded = true
		m.diffError = ""
		m.detailLines = nil
		m.cacheDetailLines()
		m.restoreReviewPosition(file, offset)
		m.detailScroll = min(m.detailScroll, m.maxDetailScroll())
		m.pendingReviewPath, m.pendingReviewOffset = "", 0
		m.restoreFilePickerSelection(pickerPath)
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
		m.prLoading = true
		m.requestID++
		m.prRequestID++
		m.detailLoading = true
		m.detailRequestID++
		m.detailLines = nil
		m.cacheDetailLines()
		return m, tea.Batch(m.loadDashboard(), m.loadPullRequestDetails())
	case pullRequestCommentPostedMsg:
		if msg.requestID != m.commentRequestID || msg.key != m.detailKey {
			break
		}
		m.commentSending = false
		if msg.err != nil {
			m.commentError = safeText(msg.err.Error())
			break
		}
		m.commentComposerOpen = false
		m.commentDraft = ""
		m.commentCursor = 0
		m.commentScroll = 0
		m.commentError = ""
		m.commentNotice = "Comment posted on " + safeText(msg.key) + "."
	case autoRefreshMsg:
		nextRefresh := scheduleAutoRefresh()
		if m.loading || m.prLoading || m.mergeInProgress || m.mergeDialog != noMergeDialog || m.commentSending {
			return m, nextRefresh
		}
		return m, tea.Batch(nextRefresh, m.startDashboardRefresh())
	case tea.KeyMsg:
		if m.commentComposerOpen {
			return m.updateCommentComposerKey(msg)
		}
		if m.themePickerOpen {
			return m.updateThemePickerKey(msg)
		}
		if m.filePickerOpen {
			return m.updateFilePickerKey(msg)
		}
		if m.paletteOpen {
			return m.updatePaletteKey(msg)
		}
		if m.searching {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.searching = false
				m.searchDraft = ""
				m.searchCursor, m.searchOffset = 0, 0
			case "enter":
				m.searching = false
				query := strings.TrimSpace(m.searchDraft)
				m.searchDraft = ""
				m.searchCursor, m.searchOffset = 0, 0
				if query == "" {
					query = defaultPRQuery
				}
				cmd := m.applyPRQuery(query)
				return m, cmd
			case "backspace", "ctrl+h":
				m.deleteSearchRune(true)
			case "delete":
				m.deleteSearchRune(false)
			case "left":
				m.searchCursor = max(0, m.searchCursor-1)
			case "right":
				m.searchCursor = min(len([]rune(m.searchDraft)), m.searchCursor+1)
			case "ctrl+left", "alt+left", "alt+b":
				m.searchCursor = searchWordBoundary(m.searchDraft, m.searchCursor, -1)
			case "ctrl+right", "alt+right", "alt+f":
				m.searchCursor = searchWordBoundary(m.searchDraft, m.searchCursor, 1)
			case "ctrl+w", "alt+backspace":
				start := searchWordBoundary(m.searchDraft, m.searchCursor, -1)
				runes := []rune(m.searchDraft)
				m.searchDraft = string(runes[:start]) + string(runes[m.searchCursor:])
				m.searchCursor = start
			case "home", "ctrl+a":
				m.searchCursor = 0
			case "end", "ctrl+e":
				m.searchCursor = len([]rune(m.searchDraft))
			case "ctrl+u":
				m.searchDraft = ""
				m.searchCursor, m.searchOffset = 0, 0
			case " ":
				m.insertSearchText(" ")
			default:
				if msg.Type == tea.KeyRunes && !msg.Alt {
					m.insertSearchText(safeText(string(msg.Runes)))
				}
			}
			if m.searching {
				m.keepSearchCursorVisible()
			}
			return m, m.schedulePRPreview()
		}
		if m.showHelp {
			switch msg.String() {
			case "?", "esc", "enter":
				m.showHelp = false
			case "q", "ctrl+c":
				return m, tea.Quit
			case "up", "k":
				m.helpScroll = max(0, m.helpScroll-1)
			case "down", "j":
				m.helpScroll = min(m.maxHelpScroll(), m.helpScroll+1)
			case "pgup", "ctrl+u":
				m.helpScroll = max(0, m.helpScroll-m.helpViewportHeight())
			case "pgdown", "ctrl+d":
				m.helpScroll = min(m.maxHelpScroll(), m.helpScroll+m.helpViewportHeight())
			case "home":
				m.helpScroll = 0
			case "end":
				m.helpScroll = m.maxHelpScroll()
			}
			return m, m.schedulePRPreview()
		}
		if msg.String() == ":" && (!m.detailOpen || m.mergeDialog == noMergeDialog && !m.mergeInProgress) {
			m.paletteOpen = true
			m.paletteQuery = ""
			m.paletteSelection = 0
			return m, nil
		}
		if m.detailOpen {
			updated, cmd := m.updatePullRequestDetailKey(msg)
			next := updated.(model)
			if !next.detailOpen && cmd == nil {
				return next, next.schedulePRPreview()
			}
			return updated, cmd
		}
		if m.inboxPreviewFocus && m.scrollInboxPreview(msg.String()) {
			return m, m.schedulePRPreview()
		}
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "left", "h":
			m.switchSection((m.activeSection + len(sections) - 1) % len(sections))
		case "right", "l":
			m.switchSection((m.activeSection + 1) % len(sections))
		case "1", "2", "3":
			section, _ := strconv.Atoi(msg.String())
			m.switchSection(section - 1)
		case "tab", "shift+tab":
			if m.hasInboxPreview() {
				m.inboxPreviewFocus = !m.inboxPreviewFocus
			}
		case "up", "k":
			m.moveSelection(-1)
		case "down", "j":
			m.moveSelection(1)
		case "pgup", "ctrl+u":
			m.moveSelection(-m.inboxPageSize())
		case "pgdown", "ctrl+d":
			m.moveSelection(m.inboxPageSize())
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
			m.helpScroll = 0
		case "t":
			m.openThemePicker()
		case "/":
			if m.activeSection == 0 {
				m.beginSearch()
			}
		case "r", "R":
			if m.loading {
				break
			}
			return m, m.startDashboardRefresh()
		}
	}

	return m, m.schedulePRPreview()
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
		return len(m.pullRequests)
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
	items := m.pullRequests
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
	for i, pr := range m.pullRequests {
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
	if m.selectedPR >= len(m.pullRequests) {
		m.selectedPR = max(0, len(m.pullRequests)-1)
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
