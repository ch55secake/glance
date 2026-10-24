package main

import (
	"errors"
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
			if !strings.Contains(updated.(model).View(), "SELECTED  2 / 18") || (cmd != nil) != (section == 0 && size.height >= 20) {
				t.Errorf("section %d selection should update immediately; only PRs schedule delayed context", section)
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

func TestPRPreviewLoadsAfterSelectionAndCachesContext(t *testing.T) {
	m := initialModel()
	m.width, m.height = 120, 36
	m.loading = false
	m.pullRequests = []githubapi.PullRequest{
		{Number: 1, Repository: "team/repo", Title: "First"},
		{Number: 2, Repository: "team/repo", Title: "Second"},
	}
	updated, delayed := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)
	if delayed == nil || !strings.Contains(m.View(), "SELECTED  2 / 2") {
		t.Fatal("selection did not update immediately and schedule delayed details")
	}
	updated, _ = m.Update(previewReadyMsg{key: "team/repo#1", epoch: m.previewEpoch})
	if updated.(model).previewPending["team/repo#1"] {
		t.Fatal("stale selection began loading")
	}
	updated, cmd := m.Update(previewReadyMsg{key: "team/repo#2", epoch: m.previewEpoch})
	m = updated.(model)
	if cmd == nil || !m.previewPending["team/repo#2"] {
		t.Fatal("settled selection did not begin loading")
	}
	updated, duplicate := m.Update(previewReadyMsg{key: "team/repo#2", epoch: m.previewEpoch})
	if duplicate != nil {
		t.Fatal("duplicate timer began another request")
	}
	m = updated.(model)
	mergeable := true
	updated, _ = m.Update(previewLoadedMsg{key: "team/repo#2", epoch: m.previewEpoch, details: githubapi.PullRequestDetails{
		PullRequest: githubapi.PullRequest{Number: 2},
		Body:        "An important change with enough context to decide what to review.",
		Mergeable:   &mergeable,
		CheckRuns:   []githubapi.CheckRun{{Name: "Build", Conclusion: "success"}},
		Reviews:     []githubapi.PullRequestReview{{User: githubapi.User{Login: "reviewer"}, State: "APPROVED"}},
	}})
	m = updated.(model)
	view := m.View()
	for _, want := range []string{"An important change", "Mergeable", "checks ✓1", "Reviews  1", "Build", "@reviewer"} {
		if !strings.Contains(view, want) {
			t.Errorf("loaded preview missing %q: %s", want, view)
		}
	}
	if m.schedulePRPreview() != nil || len(m.previewCache) != 1 {
		t.Fatal("cached preview would be fetched again")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	updated, _ = updated.(model).Update(tea.KeyMsg{Type: tea.KeyLeft})
	if updated.(model).schedulePRPreview() != nil {
		t.Fatal("switching sections lost cached PR preview")
	}
	updated, _ = updated.(model).Update(key("r"))
	m = updated.(model)
	if len(m.previewCache) != 0 || m.previewEpoch == 0 {
		t.Fatal("refresh did not invalidate the preview cache")
	}
	updated, _ = m.Update(previewLoadedMsg{key: "team/repo#2", epoch: m.previewEpoch - 1, err: errors.New("old request")})
	if updated.(model).previewErrors["team/repo#2"] != "" {
		t.Fatal("stale request after refresh changed the preview")
	}
}

func TestOtherInboxPreviewsUseAvailableContext(t *testing.T) {
	m := initialModel()
	m.width, m.height = 120, 36
	m.loading = false
	m.activeSection = 1
	m.issues = []githubapi.Issue{{Number: 12, Repository: "team/repo", Title: "Investigate timeout", Body: "Reproduce by starting a long job and checking its status.", Comments: 3}}
	if view := m.View(); !strings.Contains(view, "Reproduce by starting a long job") || !strings.Contains(view, "3 comments") {
		t.Errorf("issue preview lost description: %s", view)
	}
	m.activeSection = 2
	m.notifications = []githubapi.Notification{{Reason: "review_requested", Subject: githubapi.NotificationSubject{Title: "Review this PR", Type: "PullRequest"}}}
	if view := m.View(); !strings.Contains(view, "Your review was requested") || !strings.Contains(view, "review_requested") {
		t.Errorf("notification preview lost its reason: %s", view)
	}
}

func TestPRPreviewCacheBoundedAndErrorsDoNotRetryUntilRefresh(t *testing.T) {
	m := initialModel()
	m.width, m.height = 120, 36
	m.loading = false
	m.pullRequests = []githubapi.PullRequest{{Number: 1, Repository: "team/repo"}}
	for i := 1; i <= previewCacheLimit+3; i++ {
		m.storePRPreview(fmt.Sprintf("team/repo#%d", i), githubapi.PullRequestDetails{PullRequest: githubapi.PullRequest{Number: i}})
	}
	if len(m.previewCache) != previewCacheLimit || len(m.previewOrder) != previewCacheLimit {
		t.Fatalf("preview cache grew without bound: %d", len(m.previewCache))
	}
	updated, _ := m.Update(previewLoadedMsg{key: "team/repo#1", epoch: m.previewEpoch, err: errors.New("API unavailable")})
	m = updated.(model)
	if m.schedulePRPreview() != nil || !strings.Contains(m.View(), "Context unavailable") {
		t.Fatal("preview error did not preserve fallback and suppress automatic retries")
	}
	m.clearPRPreviews()
	if m.schedulePRPreview() == nil {
		t.Fatal("refresh should allow retrying a failed preview")
	}
}
