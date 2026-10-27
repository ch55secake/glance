package main

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m model) canCommentOnPullRequest() bool {
	return m.detailOpen && !m.detailLoading && m.detailError == "" &&
		m.prDetails.Number > 0 && !m.commentSending
}

func (m *model) openCommentComposer() {
	m.commentRequestID++
	m.commentComposerOpen = true
	m.commentDraft = ""
	m.commentCursor = 0
	m.commentScroll = 0
	m.commentError = ""
	m.commentNotice = ""
	m.commentOnDiff = false
	m.commentTarget = inlineCommentTarget{}
}

func (m *model) postPullRequestComment() tea.Cmd {
	body := strings.TrimSpace(m.commentDraft)
	if body == "" {
		m.commentError = "Write a comment before posting."
		return nil
	}
	if len([]rune(body)) > 65536 {
		m.commentError = "Comments must be 65,536 characters or fewer."
		return nil
	}
	m.commentRequestID++
	requestID, key, client := m.commentRequestID, m.detailKey, m.client
	inline, target, commitSHA := m.commentOnDiff, m.commentTarget, m.prDetails.Head.SHA
	owner, repository, number, err := parsePullRequestKey(key)
	m.commentSending = true
	m.commentError = ""
	return func() tea.Msg {
		if err != nil {
			return pullRequestCommentPostedMsg{requestID: requestID, key: key, inline: inline, target: target, err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), dashboardLoadTimeout)
		defer cancel()
		if inline {
			comment, err := client.CreatePullRequestReviewComment(ctx, owner, repository, number, commitSHA, target.path, target.line, target.side, body)
			return pullRequestCommentPostedMsg{requestID: requestID, key: key, reviewComment: comment, inline: true, target: target, err: err}
		}
		comment, err := client.CreatePullRequestComment(ctx, owner, repository, number, body)
		return pullRequestCommentPostedMsg{requestID: requestID, key: key, comment: comment, err: err}
	}
}

func (m model) updateCommentComposerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.commentSending {
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		return m, nil
	}
	previousDraft := m.commentDraft
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.commentComposerOpen = false
		m.commentDraft = ""
		m.commentCursor = 0
		m.commentScroll = 0
		m.commentError = ""
		m.commentOnDiff = false
		m.commentTarget = inlineCommentTarget{}
	case "ctrl+s":
		return m, m.postPullRequestComment()
	case "enter":
		m.insertCommentText("\n")
	case "backspace", "ctrl+h":
		m.deleteCommentRune(true)
	case "delete":
		m.deleteCommentRune(false)
	case "left":
		m.commentCursor = max(0, m.commentCursor-1)
	case "right":
		m.commentCursor = min(len([]rune(m.commentDraft)), m.commentCursor+1)
	case "ctrl+left", "alt+left", "alt+b":
		m.commentCursor = searchWordBoundary(m.commentDraft, m.commentCursor, -1)
	case "ctrl+right", "alt+right", "alt+f":
		m.commentCursor = searchWordBoundary(m.commentDraft, m.commentCursor, 1)
	case "ctrl+w", "alt+backspace":
		start := searchWordBoundary(m.commentDraft, m.commentCursor, -1)
		runes := []rune(m.commentDraft)
		m.commentDraft = string(runes[:start]) + string(runes[m.commentCursor:])
		m.commentCursor = start
	case "home", "ctrl+a":
		m.commentCursor = commentLineStart(m.commentDraft, m.commentCursor)
	case "end", "ctrl+e":
		m.commentCursor = commentLineEnd(m.commentDraft, m.commentCursor)
	case "up":
		m.moveCommentCursorVertically(-1)
	case "down":
		m.moveCommentCursorVertically(1)
	case "ctrl+u":
		m.commentDraft = ""
		m.commentCursor = 0
	case " ":
		m.insertCommentText(" ")
	default:
		if msg.Type == tea.KeyRunes && !msg.Alt {
			m.insertCommentText(sanitizeCommentInput(string(msg.Runes)))
		}
	}
	if m.commentDraft != previousDraft {
		m.commentError = ""
	}
	m.keepCommentCursorVisible()
	return m, nil
}

func sanitizeCommentInput(text string) string {
	var result strings.Builder
	for _, char := range text {
		switch {
		case char == '\n':
			result.WriteRune(char)
		case char == '\r':
			continue
		case char == '\t':
			result.WriteString("  ")
		case unicode.IsControl(char):
			continue
		default:
			result.WriteRune(char)
		}
	}
	return result.String()
}

func (m *model) insertCommentText(text string) {
	runes := []rune(m.commentDraft)
	cursor := min(max(0, m.commentCursor), len(runes))
	addition := []rune(text)
	m.commentDraft = string(runes[:cursor]) + text + string(runes[cursor:])
	m.commentCursor = cursor + len(addition)
}

func (m *model) deleteCommentRune(backward bool) {
	runes := []rune(m.commentDraft)
	cursor := min(max(0, m.commentCursor), len(runes))
	if backward {
		if cursor == 0 {
			return
		}
		m.commentDraft = string(runes[:cursor-1]) + string(runes[cursor:])
		m.commentCursor = cursor - 1
		return
	}
	if cursor < len(runes) {
		m.commentDraft = string(runes[:cursor]) + string(runes[cursor+1:])
	}
}

func commentCursorPosition(text string, cursor int) (line, column int) {
	runes := []rune(text)
	cursor = min(max(0, cursor), len(runes))
	for _, char := range runes[:cursor] {
		if char == '\n' {
			line++
			column = 0
		} else {
			column++
		}
	}
	return line, column
}

func commentLineStart(text string, cursor int) int {
	runes := []rune(text)
	cursor = min(max(0, cursor), len(runes))
	for cursor > 0 && runes[cursor-1] != '\n' {
		cursor--
	}
	return cursor
}

func commentLineEnd(text string, cursor int) int {
	runes := []rune(text)
	cursor = min(max(0, cursor), len(runes))
	for cursor < len(runes) && runes[cursor] != '\n' {
		cursor++
	}
	return cursor
}

func (m *model) moveCommentCursorVertically(direction int) {
	lines := strings.Split(m.commentDraft, "\n")
	line, column := commentCursorPosition(m.commentDraft, m.commentCursor)
	target := min(max(0, line+direction), len(lines)-1)
	position := 0
	for i := 0; i < target; i++ {
		position += len([]rune(lines[i])) + 1
	}
	m.commentCursor = position + min(column, len([]rune(lines[target])))
}

func (m model) commentViewportRows() int {
	return max(1, min(8, m.height-14))
}

func (m *model) keepCommentCursorVisible() {
	line, _ := commentCursorPosition(m.commentDraft, m.commentCursor)
	visible := m.commentViewportRows()
	if line < m.commentScroll {
		m.commentScroll = line
	} else if line >= m.commentScroll+visible {
		m.commentScroll = line - visible + 1
	}
}

func (m model) commentEditorLines(width int) []string {
	logical := strings.Split(m.commentDraft, "\n")
	cursorLine, cursorColumn := commentCursorPosition(m.commentDraft, m.commentCursor)
	lines := make([]string, len(logical))
	for i, line := range logical {
		line = safeText(line)
		if i == cursorLine {
			lines[i] = searchViewport(line, cursorColumn, 0, width)
		} else if line == "" {
			lines[i] = " "
		} else {
			lines[i] = clipLine(line, width)
		}
	}
	return lines
}

func (m model) commentComposerView(width int) []string {
	cardWidth := min(width, 72)
	inner := max(1, cardWidth-6)
	heading := "COMMENT ON PULL REQUEST"
	if m.commentOnDiff {
		heading = "COMMENT ON DIFF LINE"
	}
	if cardWidth < 34 {
		heading = "COMMENT"
	}
	draftLines := m.commentEditorLines(inner)
	visible := min(len(draftLines), m.commentViewportRows())
	start := min(max(0, m.commentScroll), max(0, len(draftLines)-visible))
	context := fmt.Sprintf("%s  ·  %s", m.detailKey, safeText(m.prDetails.Title))
	content := []string{accentStyle.Render(heading)}
	if m.commentOnDiff {
		context = fmt.Sprintf("%s  ·  %s %s", m.detailKey, safeText(m.commentTarget.path), m.commentTarget.label())
		codeStyle := textStyle
		switch m.commentTarget.change {
		case "added":
			codeStyle = addLineStyle
		case "deleted":
			codeStyle = delLineStyle
		}
		content = append(content,
			mutedStyle.Render(clipLine(context, inner)),
			codeStyle.Render(clipLine(safeText(m.commentTarget.code), inner)),
		)
	} else {
		content = append(content, mutedStyle.Render(clipLine(context, inner)))
	}
	if m.height >= 14 {
		content = append(content, "", mutedStyle.Render("Markdown supported · blank lines preserved"))
	}
	content = append(content, draftLines[start:start+visible]...)
	if m.commentError != "" {
		content = append(content, errorStyle.Render(clipLine(m.commentError, inner)))
	}
	if m.height >= 14 {
		content = append(content, "", mutedStyle.Render("ctrl+s post  ·  enter new line  ·  esc cancel"))
	}
	if m.commentSending {
		content = append(content, warnStyle.Render("Posting comment to GitHub…"))
	}
	card := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(themeAccentColor)).Padding(0, 1).Width(inner + 2).Render(strings.Join(content, "\n"))
	margin := strings.Repeat(" ", 2+max(0, (width-cardWidth)/2))
	lines := strings.Split(card, "\n")
	for i := range lines {
		lines[i] = margin + lines[i]
	}
	return lines
}
