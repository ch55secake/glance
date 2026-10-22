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
	for _, want := range []string{"Open pull requests (2)", "[OPEN] octo/project#42", "Add useful dashboard", "@octocat", "[DRAFT] octo/other#7", "@hubot", "updated 2h ago"} {
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
