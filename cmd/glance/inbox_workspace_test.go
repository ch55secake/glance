package main

import (
	"fmt"
	"strings"
	"testing"

	"glance/internal/githubapi"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestWideInboxWorkspaceTracksSelection(t *testing.T) {
	for _, size := range []struct{ width, height int }{{80, 16}, {80, 24}, {120, 36}} {
		for section := 0; section < 3; section++ {
			m := initialModel()
			m.width, m.height, m.activeSection = size.width, size.height, section
			m.loading = false
			for i := 0; i < 18; i++ {
				m.pullRequests = append(m.pullRequests, githubapi.PullRequest{Number: i + 1, Repository: "team/repo", Title: fmt.Sprintf("Review PR %d", i+1)})
				m.issues = append(m.issues, githubapi.Issue{Number: i + 1, Repository: "team/repo", Title: fmt.Sprintf("Resolve issue %d", i+1)})
				m.notifications = append(m.notifications, githubapi.Notification{Repository: githubapi.NotificationRepository{FullName: "team/repo"}, Subject: githubapi.NotificationSubject{Title: fmt.Sprintf("Thread %d", i+1)}})
			}
			m.setSelection(17)
			view := m.View()
			if !strings.Contains(view, "SELECTED  18 / 18") ||
				!strings.Contains(view, "▸ "+[]string{"Review PR 18", "Resolve issue 18", "Thread 18"}[section]) ||
				(section != 2 && !strings.Contains(view, "team/repo#18")) {
				t.Errorf("%dx%d section %d lost selected item: %s", size.width, size.height, section, view)
			}
			lines := strings.Split(view, "\n")
			if len(lines) != size.height {
				t.Errorf("%dx%d section %d has %d rows", size.width, size.height, section, len(lines))
			}
			for _, line := range lines {
				if lipgloss.Width(line) > size.width {
					t.Errorf("%dx%d section %d overflows: %q", size.width, size.height, section, line)
				}
			}
			m.setSelection(0)
			updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
			if cmd != nil || !strings.Contains(updated.(model).View(), "SELECTED  2 / 18") {
				t.Errorf("section %d selection should update the preview without fetching data", section)
			}
		}
	}
}

func TestInboxWorkspaceFallsBackOnNarrowTerminals(t *testing.T) {
	m := initialModel()
	m.width, m.height = 79, 16
	m.loading = false
	m.pullRequests = []githubapi.PullRequest{{Number: 42, Title: "A pull request", Repository: "team/repo"}}
	if view := m.View(); strings.Contains(view, "SELECTED") || !strings.Contains(view, "#42") {
		t.Fatalf("narrow list lost its single-column layout: %s", view)
	}
	m.width = 80
	if view := m.View(); !strings.Contains(view, "SELECTED  1 / 1") || !strings.Contains(view, "team/repo#42") {
		t.Fatalf("wide list did not switch to the workspace: %s", view)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || !updated.(model).detailOpen {
		t.Fatal("opening a PR from the workspace did not start the existing review flow")
	}
}

func TestInboxWorkspaceWrapsUntrustedTitles(t *testing.T) {
	m := initialModel()
	m.width, m.height = 80, 24
	m.loading = false
	m.pullRequests = []githubapi.PullRequest{{
		Number: 42, Repository: "team/repo", Title: strings.Repeat("✨変更", 40) + "\x1b[31m\nunsafe",
	}}
	view := m.View()
	if strings.Contains(view, "\x1b[31m") || strings.Contains(view, "\nunsafe") {
		t.Fatalf("unsafe title escaped the preview: %q", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > m.width {
			t.Errorf("wide preview exceeds %d columns: %q", m.width, line)
		}
	}
}
