package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"glance/internal/githubapi"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestNavigation(t *testing.T) {
	tests := []struct {
		name  string
		start int
		key   tea.KeyType
		want  int
	}{
		{name: "next section", start: 0, key: tea.KeyRight, want: 1},
		{name: "previous section", start: 1, key: tea.KeyLeft, want: 0},
		{name: "wrap forward", start: len(sections) - 1, key: tea.KeyRight, want: 0},
		{name: "wrap backward", start: 0, key: tea.KeyLeft, want: len(sections) - 1},
		{name: "tab advances", start: 0, key: tea.KeyTab, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := initialModel()
			m.activeSection = tt.start
			updated, cmd := m.Update(tea.KeyMsg{Type: tt.key})
			if cmd != nil {
				t.Fatal("navigation unexpectedly returned a command")
			}
			if got := updated.(model).activeSection; got != tt.want {
				t.Fatalf("active section = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestQuitKey(t *testing.T) {
	_, cmd := initialModel().Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("quit key returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("quit key did not return a quit message")
	}
}

func TestViewShowsDashboardAndLoadingState(t *testing.T) {
	view := initialModel().View()
	for _, want := range []string{"glance", "Overview", "Pull requests", "Welcome to glance", "Connecting to GitHub"} {
		if !strings.Contains(view, want) {
			t.Errorf("view %q does not contain %q", view, want)
		}
	}
}

func TestViewerIsShownInOverview(t *testing.T) {
	m := initialModel()
	updated, cmd := m.Update(dashboardLoadedMsg{viewer: githubapi.User{Login: "octocat"}})
	if cmd != nil {
		t.Fatal("viewer result unexpectedly returned a command")
	}

	view := updated.(model).View()
	if !strings.Contains(view, "Connected as @octocat") {
		t.Errorf("view %q does not show the authenticated user", view)
	}
}

func TestGitHubAPIErrorIsShownInOverview(t *testing.T) {
	m := initialModel()
	updated, cmd := m.Update(dashboardLoadedMsg{err: errors.New("GitHub API is unavailable")})
	if cmd != nil {
		t.Fatal("viewer error unexpectedly returned a command")
	}

	view := updated.(model).View()
	if !strings.Contains(view, "GitHub API is unavailable") {
		t.Errorf("view %q does not show the API error", view)
	}
}

func TestInitLoadsViewerThroughGhAPI(t *testing.T) {
	runner := &modelTestRunner{responses: map[string][]byte{
		"user":          []byte(`{"login":"octocat"}`),
		"user/repos":    []byte(`[]`),
		"notifications": []byte(`[]`),
	}}
	m := initialModel()
	m.client = githubapi.NewClientWithRunner(runner)

	result := m.Init()()
	msg, ok := result.(dashboardLoadedMsg)
	if !ok {
		t.Fatalf("Init() message = %T, want dashboardLoadedMsg", result)
	}
	if msg.err != nil {
		t.Fatalf("Init() error = %v", msg.err)
	}
	if msg.viewer.Login != "octocat" || len(msg.pullRequests) != 0 {
		t.Fatalf("dashboard result = %+v, want viewer and empty pull request list", msg)
	}
	if len(runner.calls) != 4 || runner.calls[0][1] != "user" {
		t.Fatalf("gh calls = %#v, want viewer and all three dashboard requests", runner.calls)
	}
	repoRequests, notificationRequests := 0, 0
	for _, call := range runner.calls[1:] {
		switch call[len(call)-1] {
		case "user/repos":
			repoRequests++
		case "notifications":
			notificationRequests++
		}
	}
	if repoRequests != 2 || notificationRequests != 1 {
		t.Fatalf("dashboard data calls = %#v, want two repository-backed requests and notifications", runner.calls)
	}

	updated, _ := m.Update(msg)
	if view := updated.(model).View(); !strings.Contains(view, "Connected as @octocat") {
		t.Errorf("view %q does not show the authenticated user", view)
	}
}

func TestInitLoadsIssuesAndNotifications(t *testing.T) {
	runner := &modelTestRunner{responses: map[string][]byte{
		"user":                   []byte(`{"login":"octocat"}`),
		"user/repos":             []byte(`[[{"name":"repo","full_name":"octo/repo","owner":{"login":"octo"},"has_issues":true}]]`),
		"repos/octo/repo/pulls":  []byte(`[[{"number":1,"title":"PR","updated_at":"2026-10-21T12:00:00Z","user":{"login":"hubot"}}]]`),
		"repos/octo/repo/issues": []byte(`[[{"number":2,"title":"Issue","state":"open","updated_at":"2026-10-21T13:00:00Z","user":{"login":"octocat"},"labels":[{"name":"bug"}],"comments":3}]]`),
		"notifications":          []byte(`[[{"id":"thread-1","unread":true,"reason":"mention","updated_at":"2026-10-21T14:00:00Z","subject":{"title":"Review needed","type":"PullRequest"},"repository":{"full_name":"octo/repo"}}]]`),
	}}
	m := initialModel()
	m.client = githubapi.NewClientWithRunner(runner)
	msg, ok := m.Init()().(dashboardLoadedMsg)
	if !ok {
		t.Fatal("Init() did not return dashboardLoadedMsg")
	}
	if msg.err != nil || msg.pullRequestErr != nil || msg.issueErr != nil || msg.notificationErr != nil {
		t.Fatalf("dashboard loading errors = viewer:%v PR:%v issue:%v notifications:%v", msg.err, msg.pullRequestErr, msg.issueErr, msg.notificationErr)
	}
	if len(msg.pullRequests) != 1 || len(msg.issues) != 1 || len(msg.notifications) != 1 {
		t.Fatalf("loaded dashboard data: PRs=%d issues=%d notifications=%d", len(msg.pullRequests), len(msg.issues), len(msg.notifications))
	}
	if msg.issues[0].Repository != "octo/repo" || msg.notifications[0].Subject.Title != "Review needed" {
		t.Fatalf("incorrect issue/notification data: %+v / %+v", msg.issues, msg.notifications)
	}
}

func TestGhAuthenticationErrorIsShownInOverview(t *testing.T) {
	runner := &modelTestRunner{
		responses: map[string][]byte{"user": nil},
		stderr:    []byte("gh: not logged into any GitHub hosts"),
		err:       errors.New("exit status 1"),
	}
	m := initialModel()
	m.client = githubapi.NewClientWithRunner(runner)
	msg := m.Init()().(dashboardLoadedMsg)
	updated, _ := m.Update(msg)

	if view := updated.(model).View(); !strings.Contains(view, "gh auth login") {
		t.Errorf("view %q does not show gh auth login guidance", view)
	}
}

func TestPullRequestListRendersRepositoryAndStatus(t *testing.T) {
	m := initialModel()
	m.activeSection = 1
	m.height = 30
	m.loading = false
	m.pullRequests = []githubapi.PullRequest{
		{
			Number: 42, Title: "Add useful dashboard", Repository: "octo/project",
			User: githubapi.User{Login: "octocat"}, UpdatedAt: time.Now().Add(-2 * time.Hour),
		},
		{
			Number: 7, Title: "Work in progress", Repository: "octo/other", Draft: true,
			User: githubapi.User{Login: "hubot"},
		},
	}

	view := m.View()
	for _, want := range []string{"PULL REQUESTS", "2 open", "octo/project#42", "Add useful dashboard", "@octocat", "octo/other#7", "DRAFT", "@hubot", "updated 2h ago"} {
		if !strings.Contains(view, want) {
			t.Errorf("view %q does not contain %q", view, want)
		}
	}
}

func TestPullRequestEmptyAndErrorStates(t *testing.T) {
	m := initialModel()
	m.activeSection = 1
	m.loading = false
	if view := m.View(); !strings.Contains(view, "No open pull requests found") {
		t.Errorf("empty view %q does not explain there are no PRs", view)
	}

	m.apiError = "GitHub API rate limit exceeded"
	if view := m.View(); !strings.Contains(view, "GitHub API rate limit exceeded") || !strings.Contains(view, "Press r to try again") {
		t.Errorf("error view %q does not show error and refresh guidance", view)
	}
}

func TestPullRequestNavigationAndViewport(t *testing.T) {
	m := initialModel()
	m.activeSection = 1
	m.height = 18
	m.loading = false
	for i := range 6 {
		m.pullRequests = append(m.pullRequests, githubapi.PullRequest{
			Number: i + 1, Title: fmt.Sprintf("PR title %d", i+1), Repository: "octo/project",
		})
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)
	if m.selectedPR != 1 {
		t.Fatalf("selected PR = %d, want 1 after down", m.selectedPR)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = updated.(model)
	if m.selectedPR != 5 {
		t.Fatalf("selected PR = %d, want last PR after end", m.selectedPR)
	}
	view := m.View()
	if !strings.Contains(view, "PR title 5") || !strings.Contains(view, "PR title 6") || strings.Contains(view, "PR title 1") {
		t.Errorf("viewport view does not follow selection: %q", view)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := updated.(model).selectedPR; got != 4 {
		t.Errorf("selected PR = %d after up, want 4", got)
	}
}

func TestIssueAndNotificationScreens(t *testing.T) {
	m := initialModel()
	m.loading = false
	m.activeSection = 2
	m.issues = []githubapi.Issue{{
		Number: 12, Title: "Document API behavior", Repository: "octo/docs", Comments: 3,
		User: githubapi.User{Login: "octocat"}, Labels: []githubapi.Label{{Name: "documentation"}},
	}}
	issueView := m.View()
	for _, text := range []string{"ISSUES", "1 open", "#12  octo/docs", "Document API behavior", "@octocat", "3 comments", "documentation"} {
		if !strings.Contains(issueView, text) {
			t.Errorf("issue view does not contain %q: %s", text, issueView)
		}
	}

	m.activeSection = 3
	m.notifications = []githubapi.Notification{{
		ID: "thread-1", Unread: true, Reason: "review_requested", Repository: githubapi.NotificationRepository{FullName: "octo/docs"},
		Subject: githubapi.NotificationSubject{Title: "Please review", Type: "PullRequest"},
	}}
	notificationView := m.View()
	for _, text := range []string{"NOTIFICATIONS", "1 unread", "UNREAD", "octo/docs", "Please review", "review_requested"} {
		if !strings.Contains(notificationView, text) {
			t.Errorf("notification view does not contain %q: %s", text, notificationView)
		}
	}
}

func TestIssueAndNotificationNavigation(t *testing.T) {
	m := initialModel()
	m.loading = false
	m.activeSection = 2
	m.issues = []githubapi.Issue{{Number: 1}, {Number: 2}, {Number: 3}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)
	if m.selectedIssue != 1 {
		t.Fatalf("selected issue = %d, want 1", m.selectedIssue)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = updated.(model)
	if m.selectedIssue != 2 {
		t.Fatalf("selected issue = %d, want 2 after end", m.selectedIssue)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(model)
	m.notifications = []githubapi.Notification{{ID: "a"}, {ID: "b"}}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := updated.(model).selectedNotification; got != 1 {
		t.Fatalf("selected notification = %d, want 1", got)
	}
}

func TestIssueAndNotificationErrorsAreIndependent(t *testing.T) {
	m := initialModel()
	m.loading = true
	updated, _ := m.Update(dashboardLoadedMsg{
		viewer:        githubapi.User{Login: "octocat"},
		issueErr:      errors.New("issues unavailable"),
		notifications: []githubapi.Notification{{ID: "thread-1", Unread: true, Repository: githubapi.NotificationRepository{FullName: "octo/repo"}, Subject: githubapi.NotificationSubject{Title: "Still available"}}},
	})
	m = updated.(model)
	if m.issueError != "issues unavailable" || len(m.notifications) != 1 || m.notificationError != "" {
		t.Fatalf("dataset errors interfered: issue=%q notification=%q data=%+v", m.issueError, m.notificationError, m.notifications)
	}
	m.activeSection = 2
	if view := m.View(); !strings.Contains(view, "Could not load issues") || !strings.Contains(view, "issues unavailable") {
		t.Errorf("issue view lost its API error: %s", view)
	}
	m.activeSection = 3
	if view := m.View(); !strings.Contains(view, "Still available") {
		t.Errorf("notification data hidden by issue error: %s", view)
	}
}

func TestRefreshReloadsDashboard(t *testing.T) {
	runner := &modelTestRunner{responses: map[string][]byte{
		"user":       []byte(`{"login":"octocat"}`),
		"user/repos": []byte(`[]`),
	}}
	m := initialModel()
	m.client = githubapi.NewClientWithRunner(runner)
	m.loading = false
	m.apiError = "stale error"
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	refreshed := updated.(model)
	if cmd == nil || !refreshed.loading || refreshed.apiError != "" {
		t.Fatalf("refresh state = %+v, command = %v; want loading with cleared error", refreshed, cmd != nil)
	}
}

func key(text string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)}
}

func TestFiltersAndSearchAreScopedToPullRequests(t *testing.T) {
	m := initialModel()
	m.loading = false
	m.viewer.Login = "octocat"
	m.pullRequests = []githubapi.PullRequest{
		{Repository: "team/alpha", Number: 1, Title: "Fix search", User: githubapi.User{Login: "octocat"}},
		{Repository: "team/beta", Number: 2, Title: "Add tests", User: githubapi.User{Login: "hubot"}, Draft: true},
	}
	updated, _ := m.Update(key("m"))
	if updated.(model).filter != allPRs {
		t.Fatal("filter changed outside PR section")
	}
	m.activeSection = 1
	for _, tc := range []struct {
		key        string
		wantFilter prFilter
		wantNumber int
	}{
		{"m", minePRs, 1}, {"d", draftPRs, 1}, {"a", allPRs, 2},
	} {
		updated, _ = m.Update(key(tc.key))
		m = updated.(model)
		if m.filter != tc.wantFilter || len(m.filteredPRs()) != tc.wantNumber {
			t.Fatalf("filter %q: mode %v, results %d", tc.key, m.filter, len(m.filteredPRs()))
		}
	}
	updated, _ = m.Update(key("/"))
	m = updated.(model)
	for _, char := range "BETA" {
		updated, _ = m.Update(key(string(char)))
		m = updated.(model)
	}
	if !m.searching || len(m.filteredPRs()) != 1 || m.filteredPRs()[0].Number != 2 {
		t.Fatalf("search query %q matched %+v", m.query, m.filteredPRs())
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.searching || m.query != "BETA" {
		t.Fatalf("enter lost search query: %+v", m)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	if m.query != "" || m.filter != allPRs || len(m.filteredPRs()) != 2 {
		t.Fatalf("esc did not clear filters: %+v", m)
	}
}

func TestSearchEditingAndNoResults(t *testing.T) {
	m := initialModel()
	m.activeSection = 1
	m.loading = false
	m.pullRequests = []githubapi.PullRequest{{Title: "Hello", Repository: "team/hello"}}
	for _, message := range []tea.KeyMsg{key("/"), key("x"), key("界")} {
		updated, _ := m.Update(message)
		m = updated.(model)
	}
	if m.query != "x界" || !strings.Contains(m.View(), "No matching pull requests") {
		t.Fatalf("search state = %q, view = %q", m.query, m.View())
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = updated.(model)
	if m.query != "x" {
		t.Fatalf("backspace left %q, want x", m.query)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.(model).searching {
		t.Fatal("esc did not exit search mode")
	}
}

func TestRefreshPreservesSelectionAndIgnoresStaleResponse(t *testing.T) {
	m := initialModel()
	m.loading = false
	m.activeSection = 1
	m.pullRequests = []githubapi.PullRequest{{Repository: "team/a", Number: 1}, {Repository: "team/b", Number: 2}}
	m.selectedPR = 1
	m.issues = []githubapi.Issue{{Repository: "team/a", Number: 3}, {Repository: "team/b", Number: 4}}
	m.selectedIssue = 1
	m.notifications = []githubapi.Notification{{ID: "thread-a"}, {ID: "thread-b"}}
	m.selectedNotification = 1
	updated, _ := m.Update(key("r"))
	m = updated.(model)
	if !m.loading || m.requestID != 1 || m.selectedPR != 1 {
		t.Fatalf("refresh state lost selection: %+v", m)
	}
	updated, _ = m.Update(dashboardLoadedMsg{requestID: 0, err: errors.New("old failure")})
	m = updated.(model)
	if !m.loading || m.apiError != "" {
		t.Fatalf("stale response changed state: %+v", m)
	}
	updated, _ = m.Update(dashboardLoadedMsg{
		requestID:     1,
		pullRequests:  []githubapi.PullRequest{{Repository: "team/b", Number: 2}, {Repository: "team/a", Number: 1}},
		issues:        []githubapi.Issue{{Repository: "team/b", Number: 4}, {Repository: "team/a", Number: 3}},
		notifications: []githubapi.Notification{{ID: "thread-b"}, {ID: "thread-a"}},
	})
	m = updated.(model)
	if m.loading || m.selectedPR != 0 || m.selectedIssue != 0 || m.selectedNotification != 0 {
		t.Fatalf("refreshed selection not preserved: %+v", m)
	}
}

func TestViewFitsTerminalAndSanitizesTitles(t *testing.T) {
	for _, size := range []struct{ width, height int }{{30, 16}, {50, 20}, {90, 30}} {
		m := initialModel()
		m.width, m.height = size.width, size.height
		m.loading = false
		m.activeSection = 1
		m.pullRequests = []githubapi.PullRequest{{Repository: "team/project", Number: 42, Title: "bad\x1b[31m\n multiline title", User: githubapi.User{Login: "octocat"}}}
		view := m.View()
		if strings.Contains(view, "\x1b[31m") || strings.Contains(view, "multiline\n") {
			t.Fatalf("unsafe title rendered: %q", view)
		}
		for _, line := range strings.Split(view, "\n") {
			if got := lipgloss.Width(line); got > size.width {
				t.Errorf("%dx%d: line width %d exceeds terminal: %q", size.width, size.height, got, line)
			}
		}
		if lines := strings.Count(view, "\n") + 1; lines > size.height {
			t.Errorf("%dx%d: view uses %d rows", size.width, size.height, lines)
		}
	}
}

func TestHelpOverlay(t *testing.T) {
	m := initialModel()
	m.height = 16
	updated, _ := m.Update(key("?"))
	m = updated.(model)
	if !m.showHelp || !strings.Contains(m.View(), "SHORTCUTS") {
		t.Fatal("help overlay not shown")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.(model).showHelp {
		t.Fatal("help overlay did not close")
	}
}

func TestDashboardLayoutsStayWithinTerminal(t *testing.T) {
	for _, size := range []struct{ width, height int }{{30, 16}, {45, 18}, {80, 24}, {120, 36}} {
		for _, state := range []string{"overview", "list", "search", "help", "error", "empty", "issues", "notifications", "issue-error", "notification-error"} {
			t.Run(fmt.Sprintf("%dx%d/%s", size.width, size.height, state), func(t *testing.T) {
				m := initialModel()
				m.width, m.height = size.width, size.height
				m.viewer.Login = "octocat"
				m.loading = false
				m.pullRequests = make([]githubapi.PullRequest, 18)
				for i := range m.pullRequests {
					m.pullRequests[i] = githubapi.PullRequest{
						Number: i + 1, Repository: "company/project", Title: "Improve the dashboard experience",
						User: githubapi.User{Login: "octocat"}, UpdatedAt: time.Now().Add(-2 * time.Hour),
					}
				}
				switch state {
				case "list":
					m.activeSection = 1
					m.selectedPR = 14
				case "search":
					m.activeSection = 1
					m.searching = true
					m.query = "project"
				case "help":
					m.showHelp = true
				case "error":
					m.apiError = "GitHub denied access; check repository access and the permissions granted to gh."
					m.activeSection = 1
				case "empty":
					m.pullRequests = nil
					m.activeSection = 1
				case "issues":
					m.activeSection = 2
					m.issues = []githubapi.Issue{{Number: 13, Repository: "company/project", Title: "An issue title", User: githubapi.User{Login: "octocat"}}}
				case "notifications":
					m.activeSection = 3
					m.notifications = []githubapi.Notification{{ID: "one", Unread: true, Repository: githubapi.NotificationRepository{FullName: "company/project"}, Subject: githubapi.NotificationSubject{Type: "Issue", Title: "A notification title"}}}
				case "issue-error":
					m.activeSection = 2
					m.issueError = "GitHub denied access; check repository access and the permissions granted to gh."
				case "notification-error":
					m.activeSection = 3
					m.notificationError = "GitHub denied access; check repository access and the permissions granted to gh."
				}
				view := m.View()
				for _, line := range strings.Split(view, "\n") {
					if got := lipgloss.Width(line); got > size.width {
						t.Errorf("line width %d exceeds %d: %q", got, size.width, line)
					}
				}
				if lines := strings.Count(view, "\n") + 1; lines > size.height {
					t.Errorf("view uses %d rows, terminal has %d:\n%s", lines, size.height, view)
				}
			})
		}
	}
}

func TestIssueAndNotificationEmptyStates(t *testing.T) {
	m := initialModel()
	m.loading = false
	for _, tc := range []struct {
		section int
		want    string
	}{
		{section: 2, want: "No open issues found"},
		{section: 3, want: "No notifications were found"},
	} {
		m.activeSection = tc.section
		if view := m.View(); !strings.Contains(view, tc.want) {
			t.Errorf("section %d empty state missing %q: %s", tc.section, tc.want, view)
		}
	}
}

func TestStatusColorsAndMonochromeFallback(t *testing.T) {
	previous := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
	m := initialModel()
	m.activeSection = 1
	m.loading = false
	m.viewer.Login = "octocat"
	m.pullRequests = []githubapi.PullRequest{
		{Repository: "team/one", Number: 1, Title: "Open work", User: githubapi.User{Login: "octocat"}},
		{Repository: "team/two", Number: 2, Title: "Draft work", Draft: true},
	}
	lipgloss.SetColorProfile(termenv.TrueColor)
	colored := m.View()
	if !strings.Contains(colored, "48;2;97;73;42") ||
		!strings.Contains(colored, "OPEN") || !strings.Contains(colored, "DRAFT") {
		t.Fatalf("true-color view lacks color-coded PR states: %q", colored)
	}
	m.selectedPR = 1
	if next := m.View(); !strings.Contains(next, "48;2;32;77;65") {
		t.Fatalf("true-color view lacks OPEN badge: %q", next)
	}
	for _, width := range []int{30, 45, 80} {
		m.width = width
		for _, line := range strings.Split(m.View(), "\n") {
			if lipgloss.Width(line) > m.width {
				t.Errorf("colored line exceeds %d-column terminal: %q", width, line)
			}
		}
	}
	lipgloss.SetColorProfile(termenv.Ascii)
	plain := m.View()
	if strings.Contains(plain, "\x1b[") || !strings.Contains(plain, "OPEN") || !strings.Contains(plain, "DRAFT") {
		t.Fatalf("monochrome view lost readable statuses: %q", plain)
	}
}

type modelTestRunner struct {
	mu        sync.Mutex
	responses map[string][]byte
	stderr    []byte
	err       error
	calls     [][]string
}

func (r *modelTestRunner) Run(_ context.Context, args ...string) ([]byte, []byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, append([]string(nil), args...))
	return r.responses[args[len(args)-1]], r.stderr, r.err
}
