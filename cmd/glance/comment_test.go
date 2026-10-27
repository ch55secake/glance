package main

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"glance/internal/githubapi"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestCommentComposerPostsMultilineConversationComment(t *testing.T) {
	body := "Thanks for the update.\n\nCould you add a regression test?"
	runner := &modelTestRunner{responses: map[string][]byte{
		"repos/octo/project/issues/42/comments": []byte(`{"id":43,"body":"Thanks for the update.\n\nCould you add a regression test?","html_url":"https://github.com/octo/project/pull/42#issuecomment-43"}`),
	}}
	m := initialModel()
	m.width, m.height = 80, 24
	m.client = githubapi.NewClientWithRunner(runner)
	m.detailOpen = true
	m.detailKey = "octo/project#42"
	m.prDetails = githubapi.PullRequestDetails{
		PullRequest: githubapi.PullRequest{Number: 42, Repository: "octo/project", Title: "Review this change", State: "open"},
	}
	updated, cmd := m.Update(key("c"))
	m = updated.(model)
	if cmd != nil || !m.commentComposerOpen {
		t.Fatalf("c did not open the comment composer: %+v", m)
	}
	for _, input := range []tea.KeyMsg{key("Thanks for the update."), {Type: tea.KeyEnter}, {Type: tea.KeyEnter}, key("Could you add a regression test?")} {
		updated, _ = m.Update(input)
		m = updated.(model)
	}
	if m.commentDraft != body {
		t.Fatalf("comment draft = %q, want %q", m.commentDraft, body)
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(model)
	if cmd == nil || !m.commentSending || !strings.Contains(m.View(), "Posting comment") {
		t.Fatalf("ctrl+s did not submit the comment: %+v", m)
	}
	result, ok := cmd().(pullRequestCommentPostedMsg)
	if !ok || result.err != nil || result.comment.ID != 43 {
		t.Fatalf("comment request returned %+v", result)
	}
	updated, followUp := m.Update(result)
	m = updated.(model)
	if followUp != nil || m.commentComposerOpen || m.commentSending || m.commentDraft != "" || !strings.Contains(m.commentNotice, "Comment posted") {
		t.Fatalf("successful comment was not acknowledged: state=%+v command=%v", m, followUp != nil)
	}
	foundBody := false
	if len(runner.calls) == 1 {
		for _, arg := range runner.calls[0] {
			foundBody = foundBody || arg == "body="+body
		}
	}
	if !foundBody {
		t.Fatalf("multiline comment was not sent intact: %#v", runner.calls)
	}
	if !strings.Contains(m.View(), "Comment posted") {
		t.Fatalf("success notice is not visible: %s", m.View())
	}
}

func TestCommentComposerKeepsDraftWhenPostingFailsAndRejectsBlankText(t *testing.T) {
	runner := &modelTestRunner{stderr: []byte("HTTP 403: Resource not accessible by integration"), err: errors.New("exit status 1")}
	m := initialModel()
	m.client = githubapi.NewClientWithRunner(runner)
	m.detailOpen = true
	m.detailKey = "octo/project#42"
	m.prDetails = githubapi.PullRequestDetails{PullRequest: githubapi.PullRequest{Number: 42, State: "open"}}
	updated, _ := m.Update(key("c"))
	m = updated.(model)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(model)
	if cmd != nil || m.commentError == "" || !m.commentComposerOpen || len(runner.calls) != 0 {
		t.Fatalf("blank comment was not rejected locally: state=%+v calls=%#v", m, runner.calls)
	}
	updated, _ = m.Update(key("Draft comment"))
	m = updated.(model)
	draft := m.commentDraft
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(model)
	if cmd == nil || !m.commentSending {
		t.Fatal("non-empty comment did not begin posting")
	}
	result := cmd().(pullRequestCommentPostedMsg)
	updated, followUp := m.Update(result)
	m = updated.(model)
	if followUp != nil || !m.commentComposerOpen || m.commentSending || m.commentDraft != draft || m.commentError == "" {
		t.Fatalf("failed comment did not preserve the draft and error: state=%+v command=%v", m, followUp != nil)
	}
}

func TestCommentActionIsAvailableInTheReviewCommandPalette(t *testing.T) {
	m := initialModel()
	m.detailOpen = true
	m.detailKey = "octo/project#42"
	m.prDetails = githubapi.PullRequestDetails{PullRequest: githubapi.PullRequest{Number: 42, State: "open"}}
	findCommentAction := func() bool {
		for _, action := range m.paletteActions() {
			if action.name == "Comment on pull request" {
				return true
			}
		}
		return false
	}
	if !findCommentAction() {
		t.Fatal("loaded pull request has no comment command")
	}
	m.detailLoading = true
	if findCommentAction() {
		t.Fatal("comment command is available before PR details finish loading")
	}
}

func TestCommentEditorSupportsCursorMovementAcrossLinesAndUnicode(t *testing.T) {
	m := initialModel()
	m.commentDraft = "alpha\n界beta\nlast"
	m.commentCursor = len([]rune("alpha\n界"))
	m.moveCommentCursorVertically(1)
	line, column := commentCursorPosition(m.commentDraft, m.commentCursor)
	if line != 2 || column != 1 {
		t.Fatalf("down moved cursor to line %d column %d, want 2:1", line, column)
	}
	m.commentCursor = commentLineStart(m.commentDraft, m.commentCursor)
	line, column = commentCursorPosition(m.commentDraft, m.commentCursor)
	if line != 2 || column != 0 {
		t.Fatalf("home moved cursor to line %d column %d, want 2:0", line, column)
	}
	m.deleteCommentRune(false)
	if m.commentDraft != "alpha\n界beta\nast" {
		t.Fatalf("delete at the beginning of line did not remove its first rune: %q", m.commentDraft)
	}
	m.commentCursor = len([]rune("alpha\n界"))
	m.deleteCommentRune(true)
	if m.commentDraft != "alpha\nbeta\nast" {
		t.Fatalf("backspace did not remove the unicode rune before the cursor: %q", m.commentDraft)
	}
}

func TestCommentComposerPreservesMultilinePastedText(t *testing.T) {
	m := initialModel()
	m.detailOpen = true
	m.prDetails = githubapi.PullRequestDetails{PullRequest: githubapi.PullRequest{Number: 42}}
	m.openCommentComposer()
	updated, _ := m.Update(key("first line\n\nsecond\tline\x1b"))
	m = updated.(model)
	if m.commentDraft != "first line\n\nsecond  line" {
		t.Fatalf("pasted comment text = %q, want newlines preserved and controls stripped", m.commentDraft)
	}
}

func TestCommentComposerFitsNarrowAndWideTerminals(t *testing.T) {
	for _, size := range []struct{ width, height int }{{30, 10}, {45, 16}, {80, 24}, {120, 32}} {
		t.Run(strings.Join([]string{strconv.Itoa(size.width), strconv.Itoa(size.height)}, "x"), func(t *testing.T) {
			m := initialModel()
			m.width, m.height = size.width, size.height
			m.detailOpen = true
			m.detailKey = "octo/project#42"
			m.prDetails = githubapi.PullRequestDetails{PullRequest: githubapi.PullRequest{Number: 42, Title: "Review", State: "open"}}
			m.commentComposerOpen = true
			m.commentDraft = "A long line with unicode 界 and words that will scroll horizontally.\nSecond line\nThird line\nFourth line"
			m.commentCursor = len([]rune(m.commentDraft))
			m.keepCommentCursorVisible()
			view := m.View()
			if !strings.Contains(view, "COMMENT") || !strings.Contains(view, "ctrl+s") {
				t.Fatalf("comment controls missing: %s", view)
			}
			if rows := strings.Count(view, "\n") + 1; rows > size.height {
				t.Errorf("view uses %d rows, terminal has %d", rows, size.height)
			}
			for _, line := range strings.Split(view, "\n") {
				if width := lipgloss.Width(line); width > size.width {
					t.Errorf("line width %d exceeds terminal: %q", width, line)
				}
			}
		})
	}
}
