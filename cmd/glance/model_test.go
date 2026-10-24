package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"glance/internal/githubapi"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
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
	for _, want := range []string{"glance", "Pull requests", "1 Pull requests", "Loading open pull requests"} {
		if !strings.Contains(view, want) {
			t.Errorf("view %q does not contain %q", view, want)
		}
	}
	if strings.Contains(view, "Overview") {
		t.Errorf("removed overview still appears in the landing screen: %s", view)
	}
}

func TestPullRequestsAreTheLandingSection(t *testing.T) {
	for _, width := range []int{30, 80, 120} {
		m := initialModel()
		m.width = width
		if m.activeSection != 0 || len(sections) != 3 || sections[0] != "Pull requests" {
			t.Fatalf("landing section is not the PR inbox: section=%d tabs=%v", m.activeSection, sections)
		}
		for i := range sections {
			m.activeSection = i
			if view := m.View(); strings.Contains(view, "Overview") || strings.Contains(view, "OVERVIEW") {
				t.Errorf("removed overview visible in section %d at width %d: %s", i, width, view)
			}
		}
		m.activeSection = 0
		if width < 65 && !strings.Contains(m.navigation(width-4), "1 / 3") {
			t.Errorf("narrow tab counter still includes the removed page: %s", m.navigation(width-4))
		}
	}
}

func TestViewerIsShownInHeader(t *testing.T) {
	m := initialModel()
	updated, cmd := m.Update(dashboardLoadedMsg{viewer: githubapi.User{Login: "octocat"}})
	if cmd != nil {
		t.Fatal("viewer result unexpectedly returned a command")
	}

	view := updated.(model).View()
	if !strings.Contains(view, "@octocat") {
		t.Errorf("view %q does not show the authenticated user", view)
	}
}

func TestGitHubAPIErrorIsShownInPRInbox(t *testing.T) {
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
		"user":          []byte(`{"login":"octocat"}`),
		"user/repos":    []byte(`[]`),
		"notifications": []byte(`[]`),
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
	if len(runner.calls) != 4 || runner.calls[0][1] != "user" {
		t.Fatalf("gh calls = %#v, want viewer and all three dashboard requests", runner.calls)
	}
	repoRequests, notificationRequests := 0, 0
	for _, call := range runner.calls[1:] {
		switch call[len(call)-1] {
		case "user/repos":
			repoRequests++
		case "notifications":
			notificationRequests++
		}
	}
	if repoRequests != 2 || notificationRequests != 1 {
		t.Fatalf("dashboard data calls = %#v, want two repository-backed requests and notifications", runner.calls)
	}

	updated, _ := m.Update(msg)
	if view := updated.(model).View(); !strings.Contains(view, "@octocat") {
		t.Errorf("view %q does not show the authenticated user", view)
	}
}

func TestInitLoadsIssuesAndNotifications(t *testing.T) {
	runner := &modelTestRunner{responses: map[string][]byte{
		"user":                   []byte(`{"login":"octocat"}`),
		"user/repos":             []byte(`[[{"name":"repo","full_name":"octo/repo","owner":{"login":"octo"},"has_issues":true}]]`),
		"repos/octo/repo/pulls":  []byte(`[[{"number":1,"title":"PR","updated_at":"2026-10-21T12:00:00Z","user":{"login":"hubot"}}]]`),
		"repos/octo/repo/issues": []byte(`[[{"number":2,"title":"Issue","state":"open","updated_at":"2026-10-21T13:00:00Z","user":{"login":"octocat"},"labels":[{"name":"bug"}],"comments":3}]]`),
		"notifications":          []byte(`[[{"id":"thread-1","unread":true,"reason":"mention","updated_at":"2026-10-21T14:00:00Z","subject":{"title":"Review needed","type":"PullRequest"},"repository":{"full_name":"octo/repo"}}]]`),
	}}
	m := initialModel()
	m.client = githubapi.NewClientWithRunner(runner)
	msg, ok := m.Init()().(dashboardLoadedMsg)
	if !ok {
		t.Fatal("Init() did not return dashboardLoadedMsg")
	}
	if msg.err != nil || msg.pullRequestErr != nil || msg.issueErr != nil || msg.notificationErr != nil {
		t.Fatalf("dashboard loading errors = viewer:%v PR:%v issue:%v notifications:%v", msg.err, msg.pullRequestErr, msg.issueErr, msg.notificationErr)
	}
	if len(msg.pullRequests) != 1 || len(msg.issues) != 1 || len(msg.notifications) != 1 {
		t.Fatalf("loaded dashboard data: PRs=%d issues=%d notifications=%d", len(msg.pullRequests), len(msg.issues), len(msg.notifications))
	}
	if msg.issues[0].Repository != "octo/repo" || msg.notifications[0].Subject.Title != "Review needed" {
		t.Fatalf("incorrect issue/notification data: %+v / %+v", msg.issues, msg.notifications)
	}
}

func TestGhAuthenticationErrorIsShownInPRInbox(t *testing.T) {
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
	m.activeSection = 0
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

func TestPullRequestRowsKeepSelectionAndNumbersOnNarrowScreens(t *testing.T) {
	m := initialModel()
	m.activeSection = 0
	m.loading = false
	m.pullRequests = []githubapi.PullRequest{
		{Number: 42, Title: "Improve the selected PR title", Repository: "octo/project", User: githubapi.User{Login: "octocat"}},
		{Number: 7, Title: "Draft title", Repository: "very-long-owner/very-long-repository-name", Draft: true, User: githubapi.User{Login: "hubot"}},
	}
	for _, width := range []int{30, 45, 80, 120} {
		m.width = width
		rows := m.prRow(m.pullRequests[0], width-4, true)
		if !strings.Contains(rows[1], "▸ ") || !strings.Contains(rows[1], "Improve the selected") {
			t.Errorf("selected title not distinguished at width %d: %q", width, rows)
		}
		if !strings.Contains(rows[2], "@octocat") || !strings.Contains(rows[2], "updated") {
			t.Errorf("selected author and timestamp missing at width %d: %q", width, rows)
		}
		draft := m.prRow(m.pullRequests[1], width-4, false)
		if !strings.Contains(draft[0], "#7") || !strings.Contains(draft[0], "DRAFT") {
			t.Errorf("long repository hid the PR number or draft state at width %d: %q", width, draft)
		}
		for _, row := range append(rows, draft...) {
			if got := lipgloss.Width(row); got > width {
				t.Errorf("PR row exceeds width %d (%d): %q", width, got, row)
			}
		}
	}
	m.selectedPR = 1
	if view := m.View(); !strings.Contains(view, "2/2 selected") {
		t.Errorf("list footer does not show selection: %s", view)
	}
	m.filter = draftPRs
	m.selectedPR = 0
	if view := m.View(); !strings.Contains(view, "1/2 shown") || !strings.Contains(view, "1/1 selected") {
		t.Errorf("filtered list does not show its size and selection: %s", view)
	}
}

func TestPullRequestListSelectionVisibleWithoutColor(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
	m := initialModel()
	m.activeSection = 0
	m.loading = false
	m.pullRequests = []githubapi.PullRequest{
		{Number: 1, Repository: "octo/repo", Title: "First PR"},
		{Number: 2, Repository: "octo/repo", Title: "Second PR", Draft: true},
	}
	m.selectedPR = 1
	view := m.View()
	if strings.Contains(view, "\x1b[") || !strings.Contains(view, "▸ Second PR") || !strings.Contains(view, "2/2 selected") {
		t.Errorf("monochrome list lost the selection marker or position: %s", view)
	}
}

func TestPullRequestEmptyAndErrorStates(t *testing.T) {
	m := initialModel()
	m.activeSection = 0
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
	m.activeSection = 0
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

func TestOpeningPullRequestLoadsAndDisplaysDetails(t *testing.T) {
	runner := &modelTestRunner{responses: map[string][]byte{
		"repos/octo/project/pulls/42":                  []byte(`{"number":42,"title":"Useful change","state":"open","body":"A detailed description.\nSecond paragraph.","updated_at":"2026-10-21T12:00:00Z","user":{"login":"octocat"},"head":{"ref":"feature","sha":"abc123"},"base":{"ref":"main","sha":"def456"},"requested_reviewers":[{"login":"hubot"}],"mergeable":true,"mergeable_state":"clean"}`),
		"repos/octo/project/pulls/42/reviews":          []byte(`[[{"user":{"login":"reviewer"},"state":"APPROVED","submitted_at":"2026-10-21T12:00:00Z"}]]`),
		"repos/octo/project/commits/abc123/check-runs": []byte(`[{"total_count":1,"check_runs":[{"name":"CI","status":"completed","conclusion":"success"}]}]`),
		"repos/octo/project":                           []byte(`{"allow_merge_commit":true,"allow_squash_merge":true,"allow_rebase_merge":false}`),
		"repos/octo/project/pulls/42/files":            []byte(`[[{"filename":"README.md","status":"modified","additions":1,"deletions":0,"patch":"@@ -1 +1 @@\n+review diff"}]]`),
	}}
	m := initialModel()
	m.client = githubapi.NewClientWithRunner(runner)
	m.loading = false
	m.activeSection = 0
	m.viewer.Login = "octocat"
	m.filter = minePRs
	m.query = "useful"
	m.pullRequests = []githubapi.PullRequest{{Number: 42, Title: "Useful change", Repository: "octo/project", User: githubapi.User{Login: "octocat"}}}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if cmd == nil || !m.detailOpen || m.detailKey != "octo/project#42" {
		t.Fatalf("enter did not open selected PR: state=%+v command=%v", m, cmd != nil)
	}
	if m.searching || m.filter != minePRs || m.query != "useful" {
		t.Fatalf("opening details changed list filters/search: %+v", m)
	}
	result := cmd()
	batch, ok := result.(tea.BatchMsg)
	if !ok {
		t.Fatalf("details command returned %T, want batch of details and diff requests", result)
	}
	for _, load := range batch {
		updated, _ = m.Update(load())
		m = updated.(model)
	}
	if m.detailLoading || m.detailError != "" || m.prDetails.Head.SHA != "abc123" || !m.diffLoaded || m.diffLoading {
		t.Fatalf("details were not loaded: %+v", m)
	}
	view := strings.Join(m.pullRequestDetailsView(76), "\n")
	for _, text := range []string{"octo/project#42", "Useful change", "Branches: feature", "main", "@hubot", "APPROVED", "PASS  CI", "A detailed description."} {
		if !strings.Contains(view, text) {
			t.Errorf("details view missing %q: %s", text, view)
		}
	}
	if !strings.Contains(view, "review diff") {
		t.Errorf("diff was not rendered automatically in the PR details: %s", view)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	back := updated.(model)
	if back.detailOpen || back.selectedPR != 0 || back.filter != minePRs || back.query != "useful" {
		t.Fatalf("returning from detail lost inbox state: %+v", back)
	}
}

func TestOpeningPullRequestAutomaticallyLoadsContinuousDiff(t *testing.T) {
	files := []githubapi.PullRequestFile{
		{Filename: "README.md", Status: "modified", Additions: 1, Deletions: 1, Patch: "@@ -1 +1 @@\n-old text\n+new text" + strings.Repeat("\n context line", 35)},
		{Filename: "src/app.go", Status: "added", Additions: 2, Patch: "@@ -0,0 +1,2 @@\n+package app\n+func Run() {}"},
	}
	page, err := json.Marshal(files)
	if err != nil {
		t.Fatal(err)
	}
	runner := &modelTestRunner{responses: map[string][]byte{
		"repos/octo/project/pulls/42":                   []byte(`{"number":42,"title":"Review me","state":"open","body":"PR details","user":{"login":"octocat"},"head":{"ref":"feature","sha":"headsha"},"base":{"ref":"main","sha":"basesha"}}`),
		"repos/octo/project/pulls/42/reviews":           []byte(`[]`),
		"repos/octo/project/commits/headsha/check-runs": []byte(`[{"total_count":0,"check_runs":[]}]`),
		"repos/octo/project":                            []byte(`{"allow_merge_commit":true}`),
		"repos/octo/project/pulls/42/files":             []byte("[" + string(page) + "]"),
	}}
	m := initialModel()
	m.client = githubapi.NewClientWithRunner(runner)
	m.loading = false
	m.activeSection = 0
	m.pullRequests = []githubapi.PullRequest{{Number: 42, Title: "Review me", Repository: "octo/project"}}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if cmd == nil || !m.detailOpen || !m.diffLoading || len(runner.calls) != 0 {
		t.Fatalf("opening PR did not start loading the diff: state=%+v calls=%#v command=%v", m, runner.calls, cmd != nil)
	}
	batchResult := cmd()
	batch, ok := batchResult.(tea.BatchMsg)
	if !ok {
		t.Fatalf("open PR command returned %T, want batch", batchResult)
	}
	for _, load := range batch {
		updated, _ = m.Update(load())
		m = updated.(model)
	}
	if !m.diffLoaded || m.diffLoading || len(m.diffFiles) != 2 {
		t.Fatalf("changed files were not loaded: %+v", m)
	}
	fullDiff := strings.Join(m.pullRequestDetailsView(76), "\n")
	for _, text := range []string{"CHANGED FILES", "README.md", "File 1/2", "-old text", "+new text", "src/app.go", "File 2/2", "+func Run() {}"} {
		if !strings.Contains(fullDiff, text) {
			t.Errorf("continuous diff missing %q: %s", text, fullDiff)
		}
	}
	view := m.View()
	for _, text := range []string{"CHANGED FILES", "README.md"} {
		if !strings.Contains(view, text) {
			t.Errorf("diff view missing %q: %s", text, view)
		}
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = updated.(model)
	if m.detailScroll == 0 || !strings.Contains(m.View(), "src/app.go") || !strings.Contains(m.View(), "package app") {
		t.Fatalf("scrolling did not reach the next file: scroll=%d view=%s", m.detailScroll, m.View())
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	if m.detailOpen || !m.diffLoaded {
		t.Fatalf("returning to the inbox lost loaded diff: %+v", m)
	}
}

func TestDiffKeepsLineNumbersAndWrapsLongLines(t *testing.T) {
	patch := "@@ -10,2 +20,3 @@ func Test()\n context\n-old value\n+new value\n+" + strings.Repeat("X", 84) + "END\n\\ No newline at end of file"
	lines := renderPatch(patch, 30)
	plain := strings.Join(lines, "\n")
	for _, want := range []string{"@@ -10,2 +20,3 @@", "10   20 │  context", "11      │ -old value", "     21 │ +new value", "22 │ +", "END", "↳"} {
		if !strings.Contains(plain, want) {
			t.Errorf("rendered diff missing %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "…") {
		t.Errorf("long diff line was truncated:\n%s", plain)
	}
	for _, line := range lines {
		if width := lipgloss.Width(line); width > 32 {
			t.Errorf("diff line exceeded 32 columns including indentation (%d): %q", width, line)
		}
	}
}

func TestDiffStatusColorsAndMonochromeFallback(t *testing.T) {
	previous := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
	lipgloss.SetColorProfile(termenv.TrueColor)
	colored := strings.Join(renderPatch("@@ -1 +1 @@\n-old\n+new", 40), "\n")
	for _, code := range []string{"48;2;48;40;70", "48;2;69;43;54", "48;2;24;58;52"} {
		if !strings.Contains(colored, code) {
			t.Errorf("diff missing hunk/add/delete background %q: %s", code, colored)
		}
	}
	lipgloss.SetColorProfile(termenv.Ascii)
	plain := strings.Join(renderPatch("@@ -1 +1 @@\n-old\n+new", 40), "\n")
	if strings.Contains(plain, "\x1b[") || !strings.Contains(plain, "-old") || !strings.Contains(plain, "+new") {
		t.Errorf("monochrome diff lost readable line indicators: %s", plain)
	}
}

func TestDiffWrapsLongFilePaths(t *testing.T) {
	m := initialModel()
	m.diffLoaded = true
	m.diffFiles = []githubapi.PullRequestFile{{
		Filename:         "src/a/very/long/directory/renamed-file.go",
		PreviousFilename: "src/old/file.go",
		Status:           "renamed",
		Patch:            "@@ -1 +1 @@\n-before\n+after",
	}}
	lines := m.pullRequestDiffLines(26)
	text := strings.Join(lines, "\n")
	if !strings.Contains(text, "src/old/file.go") || !strings.Contains(text, "src/a/") || !strings.Contains(text, "very/long/directory/rena") || !strings.Contains(text, "med-file.go") {
		t.Errorf("renamed path was truncated: %s", text)
	}
	for _, line := range lines {
		if lipgloss.Width(line) > 28 {
			t.Errorf("file path exceeded terminal width: %q", line)
		}
	}
}

func TestMergeActionsStayVisibleWhileScrollingDiff(t *testing.T) {
	m := initialModel()
	m.width, m.height = 45, 18
	m.loading = false
	m.detailOpen = true
	m.detailKey = "octo/project#42"
	m.prDetails = githubapi.PullRequestDetails{
		PullRequest:         githubapi.PullRequest{Number: 42, Repository: "octo/project", Title: "Review me", State: "open"},
		Base:                githubapi.PullRequestBranch{Ref: "main"},
		Head:                githubapi.PullRequestBranch{SHA: "abc123"},
		AllowedMergeMethods: []githubapi.MergeMethod{githubapi.MergeCommit, githubapi.MergeSquash, githubapi.MergeRebase},
	}
	m.diffLoaded = true
	m.diffFiles = []githubapi.PullRequestFile{{Filename: "src/file.go", Patch: "@@ -1 +1 @@\n" + strings.Repeat("+changed line\n", 35)}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = updated.(model)
	if m.detailScroll == 0 || !strings.Contains(m.View(), "ACTIONS") || !strings.Contains(m.View(), "+changed line") {
		t.Fatalf("scrolling lost actions or diff: %s", m.View())
	}
	end := m.detailScroll
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	m = updated.(model)
	if m.detailScroll >= end || m.detailScroll < 0 {
		t.Fatalf("page up did not move through the diff: %d → %d", end, m.detailScroll)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = updated.(model)
	if m.detailScroll != end {
		t.Fatalf("page down did not return to the end: %d, want %d", m.detailScroll, end)
	}
	updated, _ = m.Update(key("m"))
	m = updated.(model)
	chooser := m.View()
	for _, want := range []string{"Choose merge method", "Merge commit", "Squash and merge", "Rebase and merge"} {
		if !strings.Contains(chooser, want) {
			t.Errorf("merge chooser missing %q: %s", want, chooser)
		}
	}
	if m.detailScroll != end {
		t.Errorf("opening merge chooser lost scroll position: got %d, want %d", m.detailScroll, end)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if view := m.View(); !strings.Contains(view, "Confirm merge") || !strings.Contains(view, "y confirm") {
		t.Errorf("confirmation disappeared behind the diff: %s", view)
	}
}

func TestPullRequestDiffLayoutsAndUnavailablePatches(t *testing.T) {
	for _, size := range []struct{ width, height int }{{30, 16}, {45, 18}, {80, 24}, {100, 18}, {120, 36}} {
		for _, state := range []string{"normal", "binary", "error"} {
			t.Run(fmt.Sprintf("%dx%d/%s", size.width, size.height, state), func(t *testing.T) {
				m := initialModel()
				m.width, m.height = size.width, size.height
				m.detailOpen = true
				m.diffLoaded = true
				m.detailKey = "octo/project#42"
				m.prDetails = githubapi.PullRequestDetails{PullRequest: githubapi.PullRequest{Number: 42, Repository: "octo/project", State: "open"}}
				m.diffFiles = []githubapi.PullRequestFile{{
					Filename: "src/a-very-long-file-name.go", Status: "modified", Additions: 2, Deletions: 1,
					Patch: "@@ -1,3 +1,4 @@\n package main\n-old(" + strings.Repeat("x", 120) + ")\n+new(" + strings.Repeat("y", 120) + ")",
				}}
				if state == "binary" {
					m.diffFiles[0].Patch = ""
				}
				if state == "error" {
					m.diffError = strings.Repeat("GitHub denied access; check repository access and permissions. ", 3)
				}
				view := m.View()
				for _, line := range strings.Split(view, "\n") {
					if got := lipgloss.Width(line); got > size.width {
						t.Errorf("line width %d exceeds terminal: %q", got, line)
					}
				}
				if lines := strings.Count(view, "\n") + 1; lines > size.height {
					t.Errorf("view uses %d rows, terminal has %d", lines, size.height)
				}
				if state == "binary" && !strings.Contains(strings.Join(m.pullRequestDetailsView(size.width-4), "\n"), "GitHub did not provide") {
					t.Error("missing explanation for unavailable patch")
				}
			})
		}
	}
}

func TestMergeFlowRequiresMethodChoiceAndConfirmation(t *testing.T) {
	runner := &modelTestRunner{responses: map[string][]byte{
		"repos/octo/project/pulls/42/merge": []byte(`{"sha":"merged123","merged":true,"message":"Pull Request successfully merged"}`),
	}}
	m := initialModel()
	m.client = githubapi.NewClientWithRunner(runner)
	m.detailOpen = true
	m.detailKey = "octo/project#42"
	mergeable := true
	m.prDetails = githubapi.PullRequestDetails{
		PullRequest: githubapi.PullRequest{Number: 42, Repository: "octo/project", State: "open"},
		Head:        githubapi.PullRequestBranch{SHA: "head123"}, Mergeable: &mergeable,
		AllowedMergeMethods: []githubapi.MergeMethod{githubapi.MergeCommit, githubapi.MergeSquash},
	}
	updated, cmd := m.Update(key("m"))
	m = updated.(model)
	if cmd != nil || m.mergeDialog != chooseMergeMethod {
		t.Fatalf("m did not open merge method chooser: %+v", m)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.mergeDialog != confirmMerge || m.mergeMethod != githubapi.MergeSquash {
		t.Fatalf("method selection did not require confirmation: %+v", m)
	}
	if !strings.Contains(m.View(), "Confirm merge") || !strings.Contains(m.View(), "Squash and merge") {
		t.Errorf("confirmation view does not describe operation: %s", m.View())
	}
	updated, cmd = m.Update(key("n"))
	m = updated.(model)
	if cmd != nil || m.mergeDialog != noMergeDialog || m.mergeInProgress || len(runner.calls) != 0 {
		t.Fatalf("cancelled merge sent a request: state=%+v calls=%#v", m, runner.calls)
	}
	updated, _ = m.Update(key("m"))
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	updated, cmd = m.Update(key("y"))
	m = updated.(model)
	if cmd == nil || !m.mergeInProgress || m.mergeDialog != noMergeDialog {
		t.Fatalf("explicit confirmation did not start merge: %+v", m)
	}
	merged := cmd().(pullRequestMergedMsg)
	updated, followUp := m.Update(merged)
	m = updated.(model)
	if followUp == nil || m.mergeInProgress || !m.loading || !m.detailLoading || !strings.Contains(m.mergeNotice, "squash") {
		t.Fatalf("successful merge did not refresh PR/dashboard: state=%+v command=%v", m, followUp != nil)
	}
	foundMethod, foundSHA := false, false
	for _, arg := range runner.calls[0] {
		foundMethod = foundMethod || arg == "merge_method=squash"
		foundSHA = foundSHA || arg == "sha=head123"
	}
	if !foundMethod || !foundSHA {
		t.Fatalf("merge request did not use selected method and current head SHA: %#v", runner.calls)
	}
}

func TestMergeMethodChooserUsesVerticalNavigation(t *testing.T) {
	m := initialModel()
	m.detailOpen = true
	m.detailKey = "octo/project#42"
	m.prDetails = githubapi.PullRequestDetails{
		PullRequest:         githubapi.PullRequest{Number: 42, State: "open"},
		Head:                githubapi.PullRequestBranch{SHA: "abc"},
		AllowedMergeMethods: []githubapi.MergeMethod{githubapi.MergeCommit, githubapi.MergeSquash, githubapi.MergeRebase},
	}
	updated, _ := m.Update(key("m"))
	m = updated.(model)
	if m.mergeDialog != chooseMergeMethod || !strings.Contains(m.View(), "↑/↓ or j/k choose") {
		t.Fatalf("vertical merge chooser is not visible: %s", m.View())
	}
	for _, input := range []tea.KeyMsg{{Type: tea.KeyRight}, key("l"), {Type: tea.KeyLeft}, key("h")} {
		updated, _ = m.Update(input)
		m = updated.(model)
		if m.mergeMethodIndex != 0 {
			t.Fatalf("horizontal input moved the vertical chooser: %d", m.mergeMethodIndex)
		}
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(model)
	if m.mergeMethodIndex != 2 {
		t.Fatalf("up did not wrap to the last method: %d", m.mergeMethodIndex)
	}
	updated, _ = m.Update(key("j"))
	m = updated.(model)
	if m.mergeMethodIndex != 0 {
		t.Fatalf("j did not wrap to the first method: %d", m.mergeMethodIndex)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)
	updated, _ = m.Update(key("k"))
	m = updated.(model)
	if m.mergeMethodIndex != 0 {
		t.Fatalf("k did not move up: %d", m.mergeMethodIndex)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.mergeDialog != confirmMerge || m.mergeMethod != githubapi.MergeCommit {
		t.Fatalf("vertical method selection failed to reach confirmation: %+v", m)
	}
}

func TestMergeFailureIsShownWithoutRefreshingDashboard(t *testing.T) {
	runner := &modelTestRunner{
		stderr: []byte("HTTP 409: Head branch was modified. Review and try the merge again."),
		err:    errors.New("exit status 1"),
	}
	m := initialModel()
	m.client = githubapi.NewClientWithRunner(runner)
	m.loading = false
	m.detailOpen = true
	m.detailKey = "octo/project#42"
	m.prDetails = githubapi.PullRequestDetails{
		PullRequest:         githubapi.PullRequest{Number: 42, Repository: "octo/project", State: "open"},
		Head:                githubapi.PullRequestBranch{SHA: "oldsha"},
		AllowedMergeMethods: []githubapi.MergeMethod{githubapi.MergeCommit},
	}
	updated, _ := m.Update(key("m"))
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	updated, cmd := m.Update(key("y"))
	m = updated.(model)
	if cmd == nil {
		t.Fatal("confirmed merge returned no command")
	}
	result := cmd().(pullRequestMergedMsg)
	updated, followUp := m.Update(result)
	m = updated.(model)
	if followUp != nil || m.mergeInProgress || m.loading || !strings.Contains(m.mergeError, "changed since it was loaded") {
		t.Fatalf("stale-head failure state = %+v, follow-up command = %v", m, followUp != nil)
	}
	if !strings.Contains(m.View(), "refresh the PR") && !strings.Contains(m.View(), "changed since it was loaded") {
		t.Errorf("stale-head guidance missing from detail screen: %s", m.View())
	}
}

func TestPullRequestDetailsScrollAndLayout(t *testing.T) {
	for _, size := range []struct{ width, height int }{{30, 16}, {45, 18}, {80, 24}, {100, 18}, {120, 36}} {
		for _, state := range []string{"normal", "choosing", "confirming", "merge-error"} {
			t.Run(fmt.Sprintf("%dx%d/%s", size.width, size.height, state), func(t *testing.T) {
				m := initialModel()
				m.width, m.height = size.width, size.height
				m.detailOpen = true
				m.detailKey = "octo/project#42"
				m.prDetails = githubapi.PullRequestDetails{
					PullRequest:         githubapi.PullRequest{Number: 42, Repository: "octo/project", Title: "A useful pull request", State: "open"},
					Body:                strings.Repeat("Long description text with useful context.\n", 40),
					Head:                githubapi.PullRequestBranch{Ref: "feature", SHA: "headsha"},
					Base:                githubapi.PullRequestBranch{Ref: "main"},
					AllowedMergeMethods: []githubapi.MergeMethod{githubapi.MergeCommit},
				}
				switch state {
				case "choosing":
					m.mergeDialog = chooseMergeMethod
				case "confirming":
					m.mergeDialog = confirmMerge
					m.mergeMethod = githubapi.MergeCommit
				case "merge-error":
					m.mergeError = strings.Repeat("Merge failed; refresh and check branch protection. ", 4)
				}
				view := m.View()
				for _, line := range strings.Split(view, "\n") {
					if got := lipgloss.Width(line); got > size.width {
						t.Errorf("line width %d exceeds terminal: %q", got, line)
					}
				}
				if lines := strings.Count(view, "\n") + 1; lines > size.height {
					t.Errorf("view uses %d rows, terminal has %d", lines, size.height)
				}
				switch state {
				case "choosing":
					if !strings.Contains(view, "Choose merge method") || !strings.Contains(view, "Merge commit") {
						t.Errorf("merge chooser is not visible at %dx%d: %s", size.width, size.height, view)
					}
				case "confirming":
					if !strings.Contains(view, "Confirm merge") || !strings.Contains(view, "y confirm") {
						t.Errorf("merge confirmation is not visible at %dx%d: %s", size.width, size.height, view)
					}
				case "merge-error":
					if !strings.Contains(view, "Merge:") {
						t.Errorf("merge error is not visible at %dx%d: %s", size.width, size.height, view)
					}
				}
				if state == "normal" {
					updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnd})
					if updated.(model).detailScroll == 0 && updated.(model).maxDetailScroll() > 0 {
						t.Error("End did not scroll the detail description")
					}
				}
			})
		}
	}
}

func TestIssueAndNotificationScreens(t *testing.T) {
	m := initialModel()
	m.loading = false
	m.activeSection = 1
	m.issues = []githubapi.Issue{{
		Number: 12, Title: "Document API behavior", Repository: "octo/docs", Comments: 3,
		User: githubapi.User{Login: "octocat"}, Labels: []githubapi.Label{{Name: "documentation"}},
	}}
	issueView := m.View()
	for _, text := range []string{"ISSUES", "1 open", "#12  octo/docs", "Document API behavior", "@octocat", "3 comments", "documentation"} {
		if !strings.Contains(issueView, text) {
			t.Errorf("issue view does not contain %q: %s", text, issueView)
		}
	}

	m.activeSection = 2
	m.notifications = []githubapi.Notification{{
		ID: "thread-1", Unread: true, Reason: "review_requested", Repository: githubapi.NotificationRepository{FullName: "octo/docs"},
		Subject: githubapi.NotificationSubject{Title: "Please review", Type: "PullRequest"},
	}}
	notificationView := m.View()
	for _, text := range []string{"NOTIFICATIONS", "1 unread", "UNREAD", "octo/docs", "Please review", "review_requested"} {
		if !strings.Contains(notificationView, text) {
			t.Errorf("notification view does not contain %q: %s", text, notificationView)
		}
	}
}

func TestIssueAndNotificationNavigation(t *testing.T) {
	m := initialModel()
	m.loading = false
	m.activeSection = 1
	m.issues = []githubapi.Issue{{Number: 1}, {Number: 2}, {Number: 3}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)
	if m.selectedIssue != 1 {
		t.Fatalf("selected issue = %d, want 1", m.selectedIssue)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = updated.(model)
	if m.selectedIssue != 2 {
		t.Fatalf("selected issue = %d, want 2 after end", m.selectedIssue)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(model)
	m.notifications = []githubapi.Notification{{ID: "a"}, {ID: "b"}}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := updated.(model).selectedNotification; got != 1 {
		t.Fatalf("selected notification = %d, want 1", got)
	}
}

func TestIssueAndNotificationErrorsAreIndependent(t *testing.T) {
	m := initialModel()
	m.loading = true
	updated, _ := m.Update(dashboardLoadedMsg{
		viewer:        githubapi.User{Login: "octocat"},
		issueErr:      errors.New("issues unavailable"),
		notifications: []githubapi.Notification{{ID: "thread-1", Unread: true, Repository: githubapi.NotificationRepository{FullName: "octo/repo"}, Subject: githubapi.NotificationSubject{Title: "Still available"}}},
	})
	m = updated.(model)
	if m.issueError != "issues unavailable" || len(m.notifications) != 1 || m.notificationError != "" {
		t.Fatalf("dataset errors interfered: issue=%q notification=%q data=%+v", m.issueError, m.notificationError, m.notifications)
	}
	m.activeSection = 1
	if view := m.View(); !strings.Contains(view, "Could not load issues") || !strings.Contains(view, "issues unavailable") {
		t.Errorf("issue view lost its API error: %s", view)
	}
	m.activeSection = 2
	if view := m.View(); !strings.Contains(view, "Still available") {
		t.Errorf("notification data hidden by issue error: %s", view)
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
	m.activeSection = 1
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
	m.activeSection = 0
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
	m.activeSection = 0
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
	m.activeSection = 0
	m.pullRequests = []githubapi.PullRequest{{Repository: "team/a", Number: 1}, {Repository: "team/b", Number: 2}}
	m.selectedPR = 1
	m.issues = []githubapi.Issue{{Repository: "team/a", Number: 3}, {Repository: "team/b", Number: 4}}
	m.selectedIssue = 1
	m.notifications = []githubapi.Notification{{ID: "thread-a"}, {ID: "thread-b"}}
	m.selectedNotification = 1
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
	updated, _ = m.Update(dashboardLoadedMsg{
		requestID:     1,
		pullRequests:  []githubapi.PullRequest{{Repository: "team/b", Number: 2}, {Repository: "team/a", Number: 1}},
		issues:        []githubapi.Issue{{Repository: "team/b", Number: 4}, {Repository: "team/a", Number: 3}},
		notifications: []githubapi.Notification{{ID: "thread-b"}, {ID: "thread-a"}},
	})
	m = updated.(model)
	if m.loading || m.selectedPR != 0 || m.selectedIssue != 0 || m.selectedNotification != 0 {
		t.Fatalf("refreshed selection not preserved: %+v", m)
	}
}

func TestViewFitsTerminalAndSanitizesTitles(t *testing.T) {
	for _, size := range []struct{ width, height int }{{30, 16}, {50, 20}, {90, 30}} {
		m := initialModel()
		m.width, m.height = size.width, size.height
		m.loading = false
		m.activeSection = 0
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
		for _, state := range []string{"list", "search", "help", "error", "empty", "issues", "notifications", "issue-error", "notification-error"} {
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
					m.activeSection = 0
					m.selectedPR = 14
				case "search":
					m.activeSection = 0
					m.searching = true
					m.query = "project"
				case "help":
					m.showHelp = true
				case "error":
					m.apiError = "GitHub denied access; check repository access and the permissions granted to gh."
					m.activeSection = 0
				case "empty":
					m.pullRequests = nil
					m.activeSection = 0
				case "issues":
					m.activeSection = 1
					m.issues = []githubapi.Issue{{Number: 13, Repository: "company/project", Title: "An issue title", User: githubapi.User{Login: "octocat"}}}
				case "notifications":
					m.activeSection = 2
					m.notifications = []githubapi.Notification{{ID: "one", Unread: true, Repository: githubapi.NotificationRepository{FullName: "company/project"}, Subject: githubapi.NotificationSubject{Type: "Issue", Title: "A notification title"}}}
				case "issue-error":
					m.activeSection = 1
					m.issueError = "GitHub denied access; check repository access and the permissions granted to gh."
				case "notification-error":
					m.activeSection = 2
					m.notificationError = "GitHub denied access; check repository access and the permissions granted to gh."
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

func TestIssueAndNotificationEmptyStates(t *testing.T) {
	m := initialModel()
	m.loading = false
	for _, tc := range []struct {
		section int
		want    string
	}{
		{section: 1, want: "No open issues found"},
		{section: 2, want: "No notifications were found"},
	} {
		m.activeSection = tc.section
		if view := m.View(); !strings.Contains(view, tc.want) {
			t.Errorf("section %d empty state missing %q: %s", tc.section, tc.want, view)
		}
	}
}

func TestStatusColorsAndMonochromeFallback(t *testing.T) {
	previous := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
	m := initialModel()
	m.activeSection = 0
	m.loading = false
	m.viewer.Login = "octocat"
	m.pullRequests = []githubapi.PullRequest{
		{Repository: "team/one", Number: 1, Title: "Open work", User: githubapi.User{Login: "octocat"}},
		{Repository: "team/two", Number: 2, Title: "Draft work", Draft: true},
	}
	lipgloss.SetColorProfile(termenv.TrueColor)
	colored := m.View()
	if !strings.Contains(colored, "48;2;97;73;42") ||
		!strings.Contains(colored, "OPEN") || !strings.Contains(colored, "DRAFT") {
		t.Fatalf("true-color view lacks color-coded PR states: %q", colored)
	}
	m.selectedPR = 1
	if next := m.View(); !strings.Contains(next, "48;2;32;77;65") {
		t.Fatalf("true-color view lacks OPEN badge: %q", next)
	}
	for _, width := range []int{30, 45, 80} {
		m.width = width
		for _, line := range strings.Split(m.View(), "\n") {
			if lipgloss.Width(line) > m.width {
				t.Errorf("colored line exceeds %d-column terminal: %q", width, line)
			}
		}
	}
	lipgloss.SetColorProfile(termenv.Ascii)
	plain := m.View()
	if strings.Contains(plain, "\x1b[") || !strings.Contains(plain, "OPEN") || !strings.Contains(plain, "DRAFT") {
		t.Fatalf("monochrome view lost readable statuses: %q", plain)
	}
}

type modelTestRunner struct {
	mu        sync.Mutex
	responses map[string][]byte
	stderr    []byte
	err       error
	calls     [][]string
}

func (r *modelTestRunner) Run(_ context.Context, args ...string) ([]byte, []byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, append([]string(nil), args...))
	return r.responses[args[len(args)-1]], r.stderr, r.err
}
