package main

import (
	"context"
	"errors"
	"strings"
	"testing"

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

func TestViewShowsDashboardAndPlaceholder(t *testing.T) {
	view := initialModel().View()
	for _, want := range []string{"glance", "Overview", "Pull requests", "Welcome to glance", "Connecting to GitHub..."} {
		if !strings.Contains(view, want) {
			t.Errorf("view %q does not contain %q", view, want)
		}
	}
}

func TestViewerIsShownInOverview(t *testing.T) {
	m := initialModel()
	updated, cmd := m.Update(viewerLoadedMsg{viewer: githubapi.User{Login: "octocat"}})
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
	updated, cmd := m.Update(viewerLoadedMsg{err: errors.New("GitHub API is unavailable")})
	if cmd != nil {
		t.Fatal("viewer error unexpectedly returned a command")
	}

	view := updated.(model).View()
	if !strings.Contains(view, "GitHub API is unavailable") {
		t.Errorf("view %q does not show the API error", view)
	}
}

func TestInitLoadsViewerThroughGhAPI(t *testing.T) {
	runner := &modelTestRunner{stdout: []byte(`{"login":"octocat"}`)}
	m := initialModel()
	m.client = githubapi.NewClientWithRunner(runner)

	result := m.Init()()
	msg, ok := result.(viewerLoadedMsg)
	if !ok {
		t.Fatalf("Init() message = %T, want viewerLoadedMsg", result)
	}
	if msg.err != nil {
		t.Fatalf("Init() error = %v", msg.err)
	}
	if got := strings.Join(runner.args, " "); got != "api user" {
		t.Fatalf("gh args = %q, want api user", got)
	}

	updated, _ := m.Update(msg)
	if view := updated.(model).View(); !strings.Contains(view, "Connected as @octocat") {
		t.Errorf("view %q does not show the authenticated user", view)
	}
}

func TestGhAuthenticationErrorIsShownInOverview(t *testing.T) {
	runner := &modelTestRunner{
		stderr: []byte("gh: not logged into any GitHub hosts"),
		err:    errors.New("exit status 1"),
	}
	m := initialModel()
	m.client = githubapi.NewClientWithRunner(runner)
	msg := m.Init()().(viewerLoadedMsg)
	updated, _ := m.Update(msg)

	if view := updated.(model).View(); !strings.Contains(view, "gh auth login") {
		t.Errorf("view %q does not show gh auth login guidance", view)
	}
}

type modelTestRunner struct {
	stdout []byte
	stderr []byte
	err    error
	args   []string
}

func (r *modelTestRunner) Run(_ context.Context, args ...string) ([]byte, []byte, error) {
	r.args = append([]string(nil), args...)
	return r.stdout, r.stderr, r.err
}
