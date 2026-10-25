package main

import (
	"slices"
	"strings"
	"testing"

	"glance/internal/githubapi"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPalettePresetSearchesGitHubAndRejectsOlderResults(t *testing.T) {
	m := reviewNavigationModel()
	runner := &modelTestRunner{responses: map[string][]byte{"search/issues": []byte(`[{"items":[]}]`)}}
	m.client = githubapi.NewClientWithRunner(runner)
	m = applyKeys(m, key(":"), key("needs my"))
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if cmd == nil || m.query != reviewRequestedQuery || m.detailOpen || m.paletteOpen || !m.prLoading {
		t.Fatal("preset did not switch from review to the requested-review inbox")
	}
	result := cmd().(pullRequestsSearchedMsg)
	if result.err != nil || len(runner.calls) != 1 || !slices.Contains(runner.calls[0], "q="+reviewRequestedQuery) {
		t.Fatalf("preset did not use the existing GitHub search request: %#v", runner.calls)
	}
	updated, _ = m.Update(pullRequestsSearchedMsg{requestID: result.requestID - 1, pullRequests: []githubapi.PullRequest{{Number: 99}}})
	m = updated.(model)
	if len(m.pullRequests) != 0 || !m.prLoading {
		t.Fatal("an older search replaced the preset's results")
	}
	updated, _ = m.Update(result)
	m = updated.(model)
	if m.prLoading || !strings.Contains(strings.Join(m.pullRequestToolbar(116), "\n"), "NEEDS MY REVIEW") {
		t.Fatal("preset results did not settle into the matching inbox context")
	}
	m.applyPRQuery(defaultPRQuery)
	if m.query != defaultPRQuery || m.searching {
		t.Fatal("returning to authored PRs left the filter in edit mode")
	}
}
