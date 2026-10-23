package main

import (
	"strings"
	"testing"
	"time"

	"glance/internal/githubapi"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func overviewFixture() model {
	m := initialModel()
	m.loading = false
	m.viewer.Login = "octocat"
	m.notifications = []githubapi.Notification{
		{ID: "read", Unread: false},
		{ID: "review", Unread: true, Reason: "review_requested", Subject: githubapi.NotificationSubject{Title: "Review this change"}, Repository: githubapi.NotificationRepository{FullName: "team/code"}},
		{ID: "unread", Unread: true, Reason: "mention", Subject: githubapi.NotificationSubject{Title: "Mentioned in discussion"}, Repository: githubapi.NotificationRepository{FullName: "team/docs"}},
	}
	m.pullRequests = []githubapi.PullRequest{
		{Number: 12, Repository: "team/code", Title: "My fix", State: "open", User: githubapi.User{Login: "octocat"}},
		{Number: 13, Repository: "team/other", Title: "Other fix", State: "open", User: githubapi.User{Login: "friend"}},
		{Number: 14, Repository: "team/drafts", Title: "My draft", State: "open", Draft: true, User: githubapi.User{Login: "octocat"}},
	}
	m.issues = []githubapi.Issue{{Number: 22, Repository: "team/docs", Title: "Investigate regression"}}
	return m
}

func TestOverviewPriorityQueueAndNavigation(t *testing.T) {
	m := overviewFixture()
	items := m.overviewItems()
	wants := []string{"REVIEW", "YOURS", "UNREAD", "OPEN", "DRAFT", "ISSUE"}
	if len(items) != len(wants) {
		t.Fatalf("queue has %d items, want %d: %+v", len(items), len(wants), items)
	}
	for i, want := range wants {
		if items[i].label != want {
			t.Errorf("queue item %d: got %q, want %q", i, items[i].label, want)
		}
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if cmd != nil || m.activeSection != 3 || m.selectedNotification != 1 {
		t.Fatalf("enter on requested review did not open notification inbox: %+v", m)
	}
	m.activeSection = 0
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if cmd == nil || !m.detailOpen || m.detailKey != "team/code#12" || m.activeSection != 0 {
		t.Fatalf("enter on own PR did not open its review: %+v", m)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	if m.detailOpen || m.activeSection != 0 || m.overviewSelection != 1 {
		t.Fatalf("back from PR did not return to the overview queue: %+v", m)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = updated.(model)
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if cmd != nil || m.activeSection != 2 || m.selectedIssue != 0 {
		t.Fatalf("enter on issue did not open issue inbox: %+v", m)
	}
}

func TestOverviewQueueLayoutAndMonochrome(t *testing.T) {
	previous := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
	for _, profile := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		lipgloss.SetColorProfile(profile)
		for _, size := range []struct{ width, height int }{{30, 16}, {45, 18}, {80, 24}, {120, 36}} {
			m := overviewFixture()
			m.width, m.height = size.width, size.height
			m.overviewSelection = len(m.overviewItems()) - 1
			view := m.View()
			for _, want := range []string{"◈ OVERVIEW", "NEXT UP", "▸ Investigate", "6/6 selected"} {
				if !strings.Contains(view, want) {
					t.Errorf("%dx%d view missing %q: %s", size.width, size.height, want, view)
				}
			}
			if profile == termenv.Ascii && strings.Contains(view, "\x1b[") {
				t.Errorf("monochrome overview emits colors: %q", view)
			}
			if rows := strings.Count(view, "\n") + 1; rows > m.height {
				t.Errorf("%dx%d overview uses %d rows", size.width, size.height, rows)
			}
			for _, line := range strings.Split(view, "\n") {
				if got := lipgloss.Width(line); got > m.width {
					t.Errorf("%dx%d overview line uses %d columns: %q", size.width, size.height, got, line)
				}
			}
		}
	}
}

func TestOverviewEmptyPartialFailureAndRefresh(t *testing.T) {
	m := initialModel()
	m.loading = false
	m.viewer.Login = "octocat"
	if view := m.View(); !strings.Contains(view, "Nothing needs attention") {
		t.Errorf("empty overview has no guidance: %s", view)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || updated.(model).detailOpen {
		t.Fatal("enter on empty overview opened a PR")
	}
	m.apiError = "rate limited"
	if view := m.View(); !strings.Contains(view, "Could not refresh PRs") || !strings.Contains(view, "Inbox data unavailable") {
		t.Errorf("partial failure falsely claims the inbox is clear: %s", view)
	}
	m = overviewFixture()
	m.overviewSelection = len(m.overviewItems()) - 1
	updated, _ = m.Update(dashboardLoadedMsg{
		viewer:        githubapi.User{Login: "octocat"},
		pullRequests:  m.pullRequests[:1],
		notifications: m.notifications[:1],
	})
	m = updated.(model)
	if m.overviewSelection >= len(m.overviewItems()) {
		t.Errorf("refresh left selection outside the new queue: %+v", m)
	}
}

func TestOverviewRefreshKeepsSelectedItemWhenPriorityChanges(t *testing.T) {
	m := overviewFixture()
	m.overviewSelection = 1 // own PR, after the review request
	notifications := append([]githubapi.Notification(nil), m.notifications...)
	notifications[1].Unread = false
	updated, _ := m.Update(dashboardLoadedMsg{
		viewer:        githubapi.User{Login: "octocat"},
		pullRequests:  m.pullRequests,
		issues:        m.issues,
		notifications: notifications,
	})
	m = updated.(model)
	if got := m.overviewItems()[m.overviewSelection].key; got != "pr:team/code#12" {
		t.Errorf("overview refresh switched the selected item to %q", got)
	}
}

func TestOverviewQueueDoesNotRenderUnsafeContent(t *testing.T) {
	m := overviewFixture()
	m.notifications[1].Subject.Title = "bad\x1b[31m\nsecond line"
	m.pullRequests[0].Title = strings.Repeat("long title", 20)
	m.pullRequests[0].UpdatedAt = time.Now()
	m.width = 45
	view := m.View()
	if strings.Contains(view, "\x1b[31m") || strings.Contains(view, "second line\n") {
		t.Errorf("overview rendered unsafe notification title: %q", view)
	}
}
