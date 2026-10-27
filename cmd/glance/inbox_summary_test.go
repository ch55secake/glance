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
	if m.prLoading || !strings.Contains(strings.Join(m.pullRequestToolbar(116), "\n"), reviewRequestedQuery) {
		t.Fatal("preset results did not settle into the filter box")
	}
	toolbar := strings.Join(m.pullRequestToolbar(116), "\n")
	for _, removed := range []string{"MY PULL REQUESTS", "Your authored work", "NEEDS MY REVIEW", "change inbox view"} {
		if strings.Contains(toolbar, removed) {
			t.Fatalf("redundant query summary still appears: %s", toolbar)
		}
	}
	m.applyPRQuery(defaultPRQuery)
	if m.query != defaultPRQuery || m.searching {
		t.Fatal("returning to authored PRs left the filter in edit mode")
	}
}

func TestOwnedRepositoryPresetUsesAuthenticatedOwnerOnly(t *testing.T) {
	for _, login := range []string{"octocat", "my-account7"} {
		t.Run(login, func(t *testing.T) {
			m := initialModel()
			m.loading = false
			m.viewer.Login = login
			runner := &modelTestRunner{responses: map[string][]byte{"search/issues": []byte(`[{"items":[]}]`)}}
			m.client = githubapi.NewClientWithRunner(runner)
			query, ok := ownedRepositoriesPRQuery(login)
			if !ok || query != "is:pr is:open user:"+login {
				t.Fatalf("owned repository query = %q, %t", query, ok)
			}
			m = applyKeys(m, key(":"), key("owned repositories"), tea.KeyMsg{Type: tea.KeyEnter})
			if m.query != query || !m.prLoading || m.detailOpen || m.paletteOpen {
				t.Fatalf("palette did not apply owner-only query: %+v", m)
			}
			result, ok := m.searchPullRequests()().(pullRequestsSearchedMsg)
			if !ok || result.err != nil || len(runner.calls) != 1 || !slices.Contains(runner.calls[0], "q="+query) {
				t.Fatalf("search request did not use the owner's repositories: %#v", runner.calls)
			}
		})
	}
}

func TestOwnedRepositoryPresetWaitsForValidViewerLogin(t *testing.T) {
	for _, login := range []string{"", "bad login", "-leading", "trailing-", "user/owner", "bad\nname", strings.Repeat("a", 40)} {
		if query, ok := ownedRepositoriesPRQuery(login); ok {
			t.Errorf("accepted invalid viewer login %q as %q", login, query)
		}
	}
	if _, ok := ownedRepositoriesPRQuery("octocat"); !ok {
		t.Fatal("valid authenticated viewer login was rejected")
	}
	m := initialModel()
	for _, login := range []string{"", "bad login"} {
		m.viewer.Login = login
		for _, action := range m.paletteActions() {
			if action.name == "Open PRs in my repositories" {
				t.Fatalf("showed the owner query action for invalid viewer %q", login)
			}
		}
	}
}
