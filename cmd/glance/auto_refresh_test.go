package main

import (
	"slices"
	"testing"
	"time"

	"glance/internal/githubapi"

	tea "github.com/charmbracelet/bubbletea"
)

func TestInitialProgramSchedulesOneFiveMinuteAutoRefresh(t *testing.T) {
	m := initialModel()
	result := m.Init()()
	batch, ok := result.(tea.BatchMsg)
	if !ok || len(batch) != 2 || batch[0] == nil || batch[1] == nil {
		t.Fatalf("Init returned %T, want dashboard request and one refresh timer", result)
	}
	if autoRefreshInterval != 5*time.Minute {
		t.Fatalf("automatic refresh interval = %s, want 5m", autoRefreshInterval)
	}
}

func TestAutoRefreshReloadsEveryInboxUsingAppliedPRQuery(t *testing.T) {
	runner := &modelTestRunner{responses: map[string][]byte{
		"user":          []byte(`{"login":"octocat"}`),
		"user/repos":    []byte(`[]`),
		"search/issues": []byte(`[{"items":[]}]`),
		"notifications": []byte(`[]`),
	}}
	m := initialModel()
	m.client = githubapi.NewClientWithRunner(runner)
	m.loading = false
	m.query = "is:pr is:open repo:team/project"
	m.searching = true
	m.searchDraft = "is:pr is:open repo:team/draft"
	m.activeSection = 2
	m.selectedNotification = 3
	m.requestID, m.prRequestID = 7, 9
	updated, cmd := m.Update(autoRefreshMsg{})
	m = updated.(model)
	if cmd == nil || !m.loading || !m.prLoading || m.requestID != 8 || m.prRequestID != 10 {
		t.Fatalf("auto refresh did not start a dashboard request: %+v", m)
	}
	if m.query != "is:pr is:open repo:team/project" || m.searchDraft != "is:pr is:open repo:team/draft" ||
		m.activeSection != 2 || m.selectedNotification != 3 {
		t.Fatal("auto refresh changed the applied filter, draft, section, or selection")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("auto refresh command = %T, want refresh timer and dashboard load", cmd())
	}
	loadedMsg := batch[1]()
	loaded, ok := loadedMsg.(dashboardLoadedMsg)
	if !ok || loaded.err != nil || loaded.pullRequestErr != nil || loaded.issueErr != nil || loaded.notificationErr != nil {
		t.Fatalf("dashboard reload result = %T, %+v", loadedMsg, loaded)
	}
	if len(runner.calls) != 4 {
		t.Fatalf("auto refresh made %d API calls, want viewer and all inboxes", len(runner.calls))
	}
	foundQuery := false
	for _, call := range runner.calls {
		if slices.Contains(call, "q=is:pr is:open repo:team/project") {
			foundQuery = true
		}
	}
	if !foundQuery {
		t.Fatalf("automatic PR search did not use the applied query: %#v", runner.calls)
	}
}

func TestAutoRefreshSkipsOverlappingRequestsAndRearms(t *testing.T) {
	for _, state := range []string{"dashboard", "PR search", "merge in progress", "merge dialog"} {
		t.Run(state, func(t *testing.T) {
			m := initialModel()
			m.loading = state == "dashboard"
			m.prLoading = state == "PR search"
			m.mergeInProgress = state == "merge in progress"
			if state == "merge dialog" {
				m.mergeDialog = chooseMergeMethod
			}
			m.requestID, m.prRequestID = 5, 6
			updated, cmd := m.Update(autoRefreshMsg{})
			m = updated.(model)
			if cmd == nil || m.requestID != 5 || m.prRequestID != 6 ||
				m.loading != (state == "dashboard") || m.prLoading != (state == "PR search") ||
				m.mergeInProgress != (state == "merge in progress") || (m.mergeDialog != noMergeDialog) != (state == "merge dialog") {
				t.Fatalf("auto refresh overlapped an active %s request: %+v", state, m)
			}
		})
	}
}
