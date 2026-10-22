package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"glance/internal/githubapi"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
		"user":       []byte(`{"login":"octocat"}`),
		"user/repos": []byte(`[]`),
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
	if len(runner.calls) != 2 || runner.calls[0][1] != "user" || runner.calls[1][len(runner.calls[1])-1] != "user/repos" {
		t.Fatalf("gh calls = %#v, want viewer and paginated repositories requests", runner.calls)
	}

	updated, _ := m.Update(msg)
	if view := updated.(model).View(); !strings.Contains(view, "Connected as @octocat") {
		t.Errorf("view %q does not show the authenticated user", view)
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
	updated, _ = m.Update(dashboardLoadedMsg{requestID: 1, pullRequests: []githubapi.PullRequest{
		{Repository: "team/b", Number: 2}, {Repository: "team/a", Number: 1},
	}})
	m = updated.(model)
	if m.loading || m.selectedPR != 0 || m.pullRequests[0].Repository != "team/b" {
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
		for _, state := range []string{"overview", "list", "search", "help", "error", "empty"} {
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

type modelTestRunner struct {
	responses map[string][]byte
	stderr    []byte
	err       error
	calls     [][]string
}

func (r *modelTestRunner) Run(_ context.Context, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, append([]string(nil), args...))
	return r.responses[args[len(args)-1]], r.stderr, r.err
}
