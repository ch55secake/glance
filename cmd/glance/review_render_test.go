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

func TestReviewSidebarShowsReadinessAndScrollableChecks(t *testing.T) {
	mergeable := false
	m := initialModel()
	m.width, m.height = 120, 24
	m.detailOpen = true
	m.detailKey = "octo/repo#42"
	m.prDetails = githubapi.PullRequestDetails{
		PullRequest: githubapi.PullRequest{Number: 42, Repository: "octo/repo", Title: "Review", State: "open"},
		Mergeable:   &mergeable, MergeableState: "dirty",
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
	view := m.View()
	for _, want := range []string{"CHANGED FILES", "READINESS", "blocked (dirty)", "CHECKS", "FAIL  CI check 00", "PASS  CI check 01", "tab, j/k"} {
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
	if !m.checksFocus || m.checksScroll == 0 || m.detailScroll != mainScroll || !strings.Contains(m.View(), "RUNNING  CI check 13") {
		t.Fatalf("check scrolling changed diff position or hid last check: main=%d checks=%d\n%s", m.detailScroll, m.checksScroll, m.View())
	}
	for _, line := range strings.Split(m.View(), "\n") {
		if got := lipgloss.Width(line); got > m.width {
			t.Errorf("wide review line exceeds %d cells (%d): %q", m.width, got, line)
		}
	}
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(model)
	if m.checksFocus || m.checksScroll != 0 || m.detailScroll > m.maxDetailScroll() {
		t.Errorf("narrow resize did not restore main scroll focus: %+v", m)
	}
}

func TestNarrowReviewStacksChecksAfterDiff(t *testing.T) {
	m := initialModel()
	m.width, m.height = 80, 24
	m.detailOpen = true
	m.detailKey = "octo/repo#42"
	m.prDetails = githubapi.PullRequestDetails{
		PullRequest: githubapi.PullRequest{Number: 42, Repository: "octo/repo", State: "open"},
		Body:        "Description",
		CheckRuns:   []githubapi.CheckRun{{Name: "test", Status: "completed", Conclusion: "success"}},
	}
	lines := strings.Join(m.pullRequestDetailsView(76), "\n")
	if !strings.Contains(lines, "Mergeability: unknown") || !strings.Contains(lines, "PASS  test") ||
		strings.Index(lines, "DESCRIPTION") >= strings.Index(lines, "CHANGED FILES") ||
		strings.Index(lines, "CHANGED FILES") >= strings.Index(lines, "PASS  test") {
		t.Errorf("narrow review did not keep description and diff ahead of checks:\n%s", lines)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if updated.(model).checksFocus {
		t.Fatal("tab stole focus on a stacked narrow layout")
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
