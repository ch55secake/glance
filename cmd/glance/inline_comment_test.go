package main

import (
	"strings"
	"testing"

	"glance/internal/githubapi"

	tea "github.com/charmbracelet/bubbletea"
)

func TestBuildInlineCommentTargetsMapsDiffLinesAndSides(t *testing.T) {
	targets := buildInlineCommentTargets([]githubapi.PullRequestFile{{
		Filename: "src/app.go",
		Patch:    "+++ b/src/app.go\n@@ -4,3 +4,4 @@ func run() {\n context\n-old value\n+new value\n tail\n\\ No newline at end of file\n",
	}})
	want := []inlineCommentTarget{
		{path: "src/app.go", line: 5, side: "LEFT", code: "old value", change: "deleted"},
		{path: "src/app.go", line: 5, side: "RIGHT", code: "new value", change: "added"},
	}
	if len(targets) != len(want) {
		t.Fatalf("inline targets = %+v, want %+v", targets, want)
	}
	for i := range want {
		if targets[i] != want[i] {
			t.Errorf("target %d = %+v, want %+v", i, targets[i], want[i])
		}
	}
}

func TestInlineCommentPickerPostsCommentOnSelectedLine(t *testing.T) {
	runner := &modelTestRunner{responses: map[string][]byte{
		"repos/octo/project/pulls/42/comments": []byte(`{"id":51,"body":"Nit: please rename this variable.","path":"src/app.go","line":10,"side":"RIGHT","commit_id":"head123","html_url":"https://github.com/octo/project/pull/42#discussion_r51"}`),
	}}
	m := initialModel()
	m.width, m.height = 100, 28
	m.client = githubapi.NewClientWithRunner(runner)
	m.detailOpen = true
	m.detailKey = "octo/project#42"
	m.diffLoaded = true
	m.prDetails = githubapi.PullRequestDetails{
		PullRequest: githubapi.PullRequest{Number: 42, Repository: "octo/project", State: "open"},
		Head:        githubapi.PullRequestBranch{SHA: "head123"},
	}
	m.diffFiles = []githubapi.PullRequestFile{{
		Filename: "src/app.go",
		Patch:    "@@ -10 +10 @@\n-oldName\n+newName",
	}}
	m.inlineCommentTargets = buildInlineCommentTargets(m.diffFiles)
	updated, cmd := m.Update(key("C"))
	m = updated.(model)
	if cmd != nil || !m.inlineCommentPickerOpen || !strings.Contains(m.View(), "COMMENT ON CODE") {
		t.Fatalf("C did not open the diff target picker: %s", m.View())
	}
	updated, _ = m.Update(key("j"))
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.inlineCommentPickerOpen || !m.commentComposerOpen || !m.commentOnDiff || m.commentTarget.side != "RIGHT" || m.commentTarget.line != 10 || m.commentTarget.code != "newName" {
		t.Fatalf("selected diff line was not carried into the composer: %+v", m)
	}
	updated, _ = m.Update(key("Nit: please rename this variable."))
	m = updated.(model)
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(model)
	if cmd == nil || !m.commentSending || !strings.Contains(m.View(), "src/app.go R10") {
		t.Fatalf("inline comment was not submitted with its target visible: %s", m.View())
	}
	result, ok := cmd().(pullRequestCommentPostedMsg)
	if !ok || result.err != nil || !result.inline || result.reviewComment.Line != 10 || result.reviewComment.Side != "RIGHT" {
		t.Fatalf("inline comment request returned %+v", result)
	}
	updated, followUp := m.Update(result)
	m = updated.(model)
	if followUp != nil || m.commentComposerOpen || !strings.Contains(m.commentNotice, "Inline comment posted") {
		t.Fatalf("inline comment success was not acknowledged: state=%+v", m)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("GitHub calls = %#v, want one diff comment", runner.calls)
	}
	for _, want := range []string{"body=Nit: please rename this variable.", "commit_id=head123", "path=src/app.go", "line=10", "side=RIGHT"} {
		found := false
		for _, arg := range runner.calls[0] {
			found = found || arg == want
		}
		if !found {
			t.Errorf("diff comment request missing %q: %#v", want, runner.calls[0])
		}
	}
}

func TestInlineCommentRequiresAnOpenPRAndDiffTargets(t *testing.T) {
	m := initialModel()
	m.detailOpen = true
	m.diffLoaded = true
	m.prDetails = githubapi.PullRequestDetails{
		PullRequest: githubapi.PullRequest{Number: 42, State: "open"},
		Head:        githubapi.PullRequestBranch{SHA: "head123"},
	}
	m.inlineCommentTargets = []inlineCommentTarget{{path: "app.go", line: 1, side: "RIGHT"}}
	if !m.canCommentOnDiff() {
		t.Fatal("open PR with a loaded diff and head SHA cannot receive inline comments")
	}
	for name, mutate := range map[string]func(*model){
		"closed":           func(m *model) { m.prDetails.State = "closed" },
		"merged":           func(m *model) { m.prDetails.Merged = true },
		"missing head SHA": func(m *model) { m.prDetails.Head.SHA = "" },
		"no diff targets":  func(m *model) { m.inlineCommentTargets = nil },
		"diff loading":     func(m *model) { m.diffLoading = true },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := m
			mutate(&candidate)
			if candidate.canCommentOnDiff() {
				t.Fatal("inline comment was offered in an ineligible state")
			}
		})
	}
}
