package main

import (
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

func TestMissingTokenIsShownInOverview(t *testing.T) {
	t.Setenv(githubapi.TokenEnvVar, "")
	msg := loadViewer().(viewerLoadedMsg)
	if !errors.Is(msg.err, githubapi.ErrMissingToken) {
		t.Fatalf("loadViewer() error = %v, want %v", msg.err, githubapi.ErrMissingToken)
	}

	updated, _ := initialModel().Update(msg)
	if view := updated.(model).View(); !strings.Contains(view, githubapi.TokenEnvVar) {
		t.Errorf("view %q does not explain how to configure authentication", view)
	}
}
