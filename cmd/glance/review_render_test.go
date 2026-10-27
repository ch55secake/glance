package main

import (
	"fmt"
	"strings"
	"testing"

	"glance/internal/githubapi"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestReviewScrollReusesRenderedDiff(t *testing.T) {
	m := initialModel()
	m.width, m.height = 120, 24
	m.detailOpen = true
	m.detailKey = "octo/repo#42"
	m.detailRequestID = 1
	m.diffRequestID = 1
	m.detailLoading = true
	m.diffLoading = true
	details := githubapi.PullRequestDetails{
		PullRequest: githubapi.PullRequest{Number: 42, Repository: "octo/repo", State: "open"},
		Body:        "# Details\n\n```go\nfunc main() {}\n```",
	}
	updated, _ := m.Update(pullRequestDetailsLoadedMsg{requestID: 1, key: m.detailKey, details: details})
	m = updated.(model)
	patch := "@@ -1 +1 @@\n" + strings.Repeat("+func main() {}\n", 900)
	updated, _ = m.Update(pullRequestFilesLoadedMsg{requestID: 1, key: m.detailKey,
		files: []githubapi.PullRequestFile{{Filename: "main.go", Patch: patch}}})
	m = updated.(model)
	if len(m.detailLines) < 900 {
		t.Fatalf("rendered diff was not cached: %d lines", len(m.detailLines))
	}
	cached := &m.detailLines[0]
	for i := range 30 {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
		m = updated.(model)
		_ = m.View()
		if &m.detailLines[0] != cached || &m.pullRequestDetailsView(m.detailLinesWidth)[0] != cached {
			t.Fatalf("scroll %d re-rendered the review", i)
		}
	}
	if m.detailScroll == 0 {
		t.Fatal("paging did not scroll the cached diff")
	}
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = updated.(model)
	if &m.detailLines[0] != cached {
		t.Fatal("height-only resize re-rendered the diff")
	}
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = updated.(model)
	if &m.detailLines[0] == cached || m.detailLinesWidth != 76 {
		t.Fatal("width change did not reflow the diff")
	}
	updated, _ = m.Update(pullRequestFilesLoadedMsg{requestID: 1, key: m.detailKey,
		files: []githubapi.PullRequestFile{{Filename: "main.go", Patch: "@@ -1 +1 @@\n+new"}}})
	m = updated.(model)
	if len(m.detailLines) >= 900 || !strings.Contains(strings.Join(m.detailLines, "\n"), "+new") {
		t.Fatal("refreshed diff was not rendered into the cache")
	}
}

func BenchmarkReviewScrollLargeDiff(b *testing.B) {
	m := initialModel()
	m.width, m.height = 120, 24
	m.detailOpen = true
	m.prDetails = githubapi.PullRequestDetails{PullRequest: githubapi.PullRequest{Number: 42, State: "open"}}
	m.diffLoaded = true
	m.diffFiles = []githubapi.PullRequestFile{{Filename: "main.go", Patch: "@@ -1 +1 @@\n" + strings.Repeat("+func main() {}\n", 900)}}
	m.cacheDetailLines()
	for _, cache := range []bool{true, false} {
		name := "cached"
		if !cache {
			name = "uncached"
		}
		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				copy := m
				if !cache {
					copy.detailLines = nil
				}
				updated, _ := copy.Update(tea.KeyMsg{Type: tea.KeyPgDown})
				copy = updated.(model)
				_ = copy.View()
			}
		})
	}
}

func TestReviewSidebarShowsReadinessAndScrollableChecks(t *testing.T) {
	mergeable := false
	m := initialModel()
	m.width, m.height = 120, 24
	m.detailOpen = true
	m.detailKey = "octo/repo#42"
	m.prDetails = githubapi.PullRequestDetails{
		PullRequest: githubapi.PullRequest{Number: 42, Repository: "octo/repo", Title: "Review", State: "open"},
		Mergeable:   &mergeable, MergeableState: "dirty",
		RequestedReviewers: []githubapi.User{{Login: "hubot"}},
		Reviews:            []githubapi.PullRequestReview{{User: githubapi.User{Login: "reviewer"}, State: "APPROVED"}},
	}
	for i := range 14 {
		conclusion := "success"
		if i == 0 {
			conclusion = "failure"
		}
		if i == 13 {
			conclusion = ""
		}
		m.prDetails.CheckRuns = append(m.prDetails.CheckRuns, githubapi.CheckRun{
			Name: fmt.Sprintf("CI check %02d", i), Status: "in_progress", Conclusion: conclusion,
		})
	}
	m.diffLoaded = true
	m.diffFiles = []githubapi.PullRequestFile{{Filename: "app.go", Patch: "@@ -1 +1 @@\n" + strings.Repeat("+func main() {}\n", 25)}}
	if left := strings.Join(m.pullRequestDetailsView(82), "\n"); strings.Contains(left, "REVIEW ACTIVITY") || strings.Contains(left, "@reviewer: APPROVED") {
		t.Errorf("review activity is still in the wide diff column: %s", left)
	}
	view := m.View()
	for _, want := range []string{"CHANGED FILES", "READINESS", "blocked (dirty)", "CHECKS", "FAIL  CI check 00", "PASS  CI check 01"} {
		if !strings.Contains(view, want) {
			t.Errorf("wide review missing %q:\n%s", want, view)
		}
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = updated.(model)
	if !strings.Contains(m.View(), "FAIL  CI check 00") {
		t.Error("right-side check status disappeared when scrolling the diff")
	}
	mainScroll := m.detailScroll
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = updated.(model)
	if !m.sidebarFocus || m.sidebarScroll == 0 || m.detailScroll != mainScroll || !strings.Contains(m.View(), "REVIEW ACTIVITY") || !strings.Contains(m.View(), "@reviewer: APPROVED") {
		t.Fatalf("sidebar scrolling changed diff position or hid review activity: main=%d sidebar=%d\n%s", m.detailScroll, m.sidebarScroll, m.View())
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyHome})
	m = updated.(model)
	if m.sidebarScroll != 0 || m.detailScroll != mainScroll || !strings.Contains(m.View(), "FAIL  CI check 00") {
		t.Fatalf("returning to checks moved the diff or hid checks: %s", m.View())
	}
	for _, line := range strings.Split(m.View(), "\n") {
		if got := lipgloss.Width(line); got > m.width {
			t.Errorf("wide review line exceeds %d cells (%d): %q", m.width, got, line)
		}
	}
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(model)
	if m.sidebarFocus || m.sidebarScroll != 0 || m.detailScroll > m.maxDetailScroll() {
		t.Errorf("narrow resize did not restore main scroll focus: %+v", m)
	}
}

func TestReviewFocusIsVisualWithoutRoutineFooter(t *testing.T) {
	previous := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
	for _, profile := range []termenv.Profile{termenv.TrueColor, termenv.Ascii} {
		lipgloss.SetColorProfile(profile)
		m := initialModel()
		m.width, m.height = 120, 24
		m.detailOpen = true
		m.detailKey = "octo/repo#42"
		m.prDetails.Number = 42
		m.diffLoaded = true
		m.diffFiles = []githubapi.PullRequestFile{{Filename: "main.go", Patch: "@@ -1 +1 @@\n" + strings.Repeat("+line\n", 30)}}
		for _, focused := range []bool{false, true} {
			m.sidebarFocus = focused
			view := m.View()
			lines := strings.Split(view, "\n")
			if len(lines) != m.height {
				t.Errorf("focus %t: review uses %d rows, want %d", focused, len(lines), m.height)
			}
			for _, footerText := range []string{"◆ ACTIONS", "m merge", "tab switch pane", "PgUp/PgDn page", "esc back"} {
				if strings.Contains(view, footerText) {
					t.Errorf("focus %t: routine shortcut footer still takes space (%q): %s", focused, footerText, view)
				}
			}
			if strings.Contains(view, "FOCUSED") {
				t.Errorf("focus %t: pane focus is redundantly labeled: %s", focused, view)
			}
			breadcrumb := m.navigation(m.width - 4)
			sidebar := m.reviewSidebar(32, m.detailViewportHeight())[0]
			if focused && (!strings.Contains(sidebar, "▎") || strings.Contains(breadcrumb, "▎")) ||
				!focused && (!strings.Contains(breadcrumb, "▎") || strings.Contains(sidebar, "▎")) {
				t.Errorf("focus %t: active pane lacks its visual indicator: %q / %q", focused, breadcrumb, sidebar)
			}
			if profile == termenv.TrueColor {
				active := breadcrumb
				if focused {
					active = sidebar
				}
				if !strings.Contains(active, "48;2;") {
					t.Errorf("focus %t: active pane lacks highlight color: %q", focused, active)
				}
			} else if strings.Contains(view, "\x1b[") {
				t.Errorf("focus %t: monochrome review emits color: %q", focused, view)
			}
		}
	}
}

func TestReviewDiffHasNoPermanentActionFooterAcrossWidths(t *testing.T) {
	for _, width := range []int{30, 45, 50, 72, 120} {
		m := initialModel()
		m.width, m.height = width, 16
		m.detailOpen = true
		m.prDetails.Number = 42
		view := m.View()
		if strings.Contains(view, "◆ ACTIONS") || strings.Contains(view, "m merge") || strings.Contains(view, "esc back") {
			t.Errorf("routine review footer remains at width %d: %s", width, view)
		}
		if len(strings.Split(view, "\n")) != m.height {
			t.Errorf("diff does not use the freed rows at width %d: %s", width, view)
		}
		for _, line := range strings.Split(view, "\n") {
			if got := lipgloss.Width(line); got > m.width {
				t.Errorf("review line exceeds %d columns (%d): %q", m.width, got, line)
			}
		}
	}
}

func TestMergeRemainsAccessibleInPaletteAndChooserStaysPinned(t *testing.T) {
	m := initialModel()
	m.width, m.height = 100, 16
	m.detailOpen = true
	m.detailKey = "octo/repo#42"
	m.prDetails = githubapi.PullRequestDetails{
		PullRequest:         githubapi.PullRequest{Number: 42, State: "open"},
		Head:                githubapi.PullRequestBranch{SHA: "sha"},
		AllowedMergeMethods: []githubapi.MergeMethod{githubapi.MergeSquash},
	}
	m = applyKeys(m, key(":"), key("merge"), tea.KeyMsg{Type: tea.KeyEnter})
	if m.paletteOpen || m.mergeDialog != chooseMergeMethod || !strings.Contains(m.View(), "Choose merge method") {
		t.Fatalf("palette did not launch the pinned merge chooser:\n%s", m.View())
	}
	lines := strings.Split(m.View(), "\n")
	if len(lines) != m.height || !strings.Contains(lines[len(lines)-1], "enter continue") {
		t.Fatalf("merge chooser controls did not stay pinned:\n%s", m.View())
	}
	m = applyKeys(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mergeDialog != noMergeDialog || strings.Contains(m.View(), "◆ ACTIONS") {
		t.Fatal("closing the chooser restored the permanent footer")
	}
}

func TestMergeChooserTakesFocusFromReviewPanes(t *testing.T) {
	m := initialModel()
	m.width, m.height = 120, 24
	m.detailOpen = true
	m.prDetails.Number = 42
	m.prDetails.State = "open"
	m.prDetails.Head.SHA = "sha"
	m.prDetails.AllowedMergeMethods = []githubapi.MergeMethod{githubapi.MergeCommit}
	updated, _ := m.Update(key("m"))
	m = updated.(model)
	if m.mergeDialog != chooseMergeMethod || strings.Contains(m.navigation(116), "▎") ||
		strings.Contains(m.reviewSidebar(32, m.detailViewportHeight())[0], "▎") ||
		!strings.Contains(m.View(), "▸ 1  Merge commit") {
		t.Errorf("merge chooser did not become the visual focus: %s", m.View())
	}
}

func TestNarrowReviewStacksChecksAfterDiff(t *testing.T) {
	m := initialModel()
	m.width, m.height = 80, 24
	m.detailOpen = true
	m.detailKey = "octo/repo#42"
	m.prDetails = githubapi.PullRequestDetails{
		PullRequest:        githubapi.PullRequest{Number: 42, Repository: "octo/repo", State: "open"},
		Body:               "Description",
		CheckRuns:          []githubapi.CheckRun{{Name: "test", Status: "completed", Conclusion: "success"}},
		RequestedReviewers: []githubapi.User{{Login: "reviewer"}},
		Reviews:            []githubapi.PullRequestReview{{User: githubapi.User{Login: "hubot"}, State: "APPROVED"}},
	}
	lines := strings.Join(m.pullRequestDetailsView(76), "\n")
	if !strings.Contains(lines, "Mergeability: unknown") || !strings.Contains(lines, "PASS  test") ||
		strings.Index(lines, "DESCRIPTION") >= strings.Index(lines, "CHANGED FILES") ||
		strings.Index(lines, "CHANGED FILES") >= strings.Index(lines, "PASS  test") ||
		strings.Index(lines, "PASS  test") >= strings.Index(lines, "REVIEW ACTIVITY") ||
		!strings.Contains(lines, "@reviewer") || !strings.Contains(lines, "@hubot: APPROVED") {
		t.Errorf("narrow review did not keep description and diff ahead of checks:\n%s", lines)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if updated.(model).sidebarFocus {
		t.Fatal("tab stole focus on a stacked narrow layout")
	}
}

func TestSidebarReviewActivityKeepsAllReviewersAndReviews(t *testing.T) {
	m := initialModel()
	m.width, m.height = 120, 24
	m.detailOpen = true
	m.detailKey = "octo/repo#42"
	m.prDetails.Number = 42
	for i := range 10 {
		login := fmt.Sprintf("reviewer%02d", i)
		m.prDetails.RequestedReviewers = append(m.prDetails.RequestedReviewers, githubapi.User{Login: login})
		m.prDetails.Reviews = append(m.prDetails.Reviews, githubapi.PullRequestReview{
			User: githubapi.User{Login: login}, State: "CHANGES_REQUESTED",
		})
	}
	// Duplicate submissions from one reviewer should still render once.
	m.prDetails.Reviews = append(m.prDetails.Reviews, m.prDetails.Reviews[0])
	activity := strings.Join(m.reviewActivityLines(30), "\n")
	for _, want := range []string{"◇ REVIEW ACTIVITY", "@reviewer00", "@reviewer09: CHANGES_", "REQUESTED"} {
		if !strings.Contains(activity, want) {
			t.Errorf("activity missing %q: %s", want, activity)
		}
	}
	if strings.Count(activity, "@reviewer00: ") != 1 {
		t.Errorf("repeat review was not deduplicated: %s", activity)
	}
	for _, line := range m.reviewActivityLines(30) {
		if lipgloss.Width(line) > 32 {
			t.Errorf("review activity exceeds sidebar width: %q", line)
		}
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = updated.(model)
	if !strings.Contains(m.View(), "@reviewer09: CHANGES_") || m.sidebarScroll != m.maxSidebarScroll() {
		t.Errorf("last review unreachable in sidebar: %s", m.View())
	}
}

func TestLongCheckNamesWrapWithoutHidingStatus(t *testing.T) {
	m := initialModel()
	m.width, m.height = 100, 18
	m.detailOpen = true
	name := "integration/" + strings.Repeat("very-long-check-name/", 3) + "END"
	m.prDetails.CheckRuns = []githubapi.CheckRun{{Name: name, Conclusion: "failure"}}
	lines := m.checkStatusLines(30)
	if !strings.Contains(strings.Join(lines, ""), "END") || !strings.Contains(lines[0], "FAIL") || len(lines) < 2 {
		t.Errorf("check name or status was truncated: %q", lines)
	}
	for _, line := range lines {
		if got := lipgloss.Width(line); got > 32 {
			t.Errorf("check exceeded sidebar width: %d", got)
		}
	}
}

func TestReadinessSymbolsAreDistinctWithoutColor(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
	for _, check := range []struct{ conclusion, status, symbol string }{
		{"success", "completed", "✓ PASS"},
		{"failure", "completed", "✕ FAIL"},
		{"skipped", "completed", "– SKIP"},
		{"cancelled", "completed", "! CANCEL"},
		{"", "in_progress", "◌ RUNNING"},
		{"", "", "? UNKNOWN"},
	} {
		label, _ := checkStyle(githubapi.CheckRun{Conclusion: check.conclusion, Status: check.status})
		if label != check.symbol {
			t.Errorf("check %q/%q: got %q, want %q", check.conclusion, check.status, label, check.symbol)
		}
	}
	m := initialModel()
	m.width = 100
	m.detailOpen = true
	for _, tc := range []struct {
		mergeable *bool
		want      string
	}{
		{nil, "? Mergeability: unknown"},
		{new(true), "✓ Mergeability: mergeable"},
		{new(false), "✕ Mergeability: blocked"},
	} {
		m.prDetails.Mergeable = tc.mergeable
		line := m.mergeabilityLine(30)
		if !strings.Contains(line, tc.want) || lipgloss.Width(line) > 32 || strings.Contains(line, "\x1b[") {
			t.Errorf("monochrome readiness lost its symbol or exceeded the sidebar: %q", line)
		}
	}
}

func TestMarkdownRendersFormattingAndFencedCode(t *testing.T) {
	previous := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
	body := "# Changes\n\n- First item\n- Second item\n\n```go\nfunc main() { println(\"hi\") }\n```"
	for _, profile := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		lipgloss.SetColorProfile(profile)
		lines := markdownLines(body, 46)
		text := ansi.Strip(strings.Join(lines, "\n"))
		for _, want := range []string{"Changes", "First item", "Second item", "func main", "println"} {
			if !strings.Contains(text, want) {
				t.Errorf("Markdown rendering missing %q (profile %v): %s", want, profile, text)
			}
		}
		for _, line := range lines {
			if got := lipgloss.Width(line); got > 48 {
				t.Errorf("Markdown exceeded terminal width (%d): %q", got, line)
			}
		}
		if profile == termenv.Ascii && strings.Contains(strings.Join(lines, "\n"), "\x1b[") {
			t.Errorf("monochrome Markdown has ANSI sequences: %q", strings.Join(lines, "\n"))
		}
	}
}

func TestDiffHighlightsGoSyntaxWithoutLosingLineColors(t *testing.T) {
	previous := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
	lipgloss.SetColorProfile(termenv.TrueColor)
	patch := "@@ -1 +1 @@\n-func before() {}\n+func after() { println(\"hello\") }"
	text := strings.Join(renderFilePatch(patch, "main.go", 55), "\n")
	for _, code := range []string{"38;2;214;197;255", "48;2;24;58;52", "48;2;69;43;54", "func", "hello"} {
		if !strings.Contains(text, code) {
			t.Errorf("syntax-highlighted diff missing %q: %s", code, text)
		}
	}
	for _, line := range strings.Split(text, "\n") {
		if got := lipgloss.Width(line); got > 57 {
			t.Errorf("highlighted diff exceeded width (%d): %q", got, line)
		}
	}
}

func TestMarkdownLongCodeIsNotTruncated(t *testing.T) {
	code := "const message = \"" + strings.Repeat("payload", 13) + "END\""
	for _, width := range []int{26, 48} {
		lines := markdownLines("```js\n"+code+"\n```", width)
		plain := strings.ReplaceAll(ansi.Strip(strings.Join(lines, "")), " ", "")
		if !strings.Contains(plain, strings.Repeat("payload", 13)+"END") {
			t.Errorf("Markdown code is truncated at width %d: %q", width, ansi.Strip(strings.Join(lines, "\n")))
		}
		for _, line := range lines {
			if lipgloss.Width(line) > width+2 {
				t.Errorf("Markdown code exceeds width %d: %d", width, lipgloss.Width(line))
			}
		}
	}
}
