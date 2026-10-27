package main

import (
	"fmt"
	"strconv"
	"strings"

	"glance/internal/githubapi"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type inlineCommentTarget struct {
	path         string
	line         int
	side         string
	code         string
	change       string
	preview      []inlineCommentPreviewLine
	previewIndex int
}

type inlineCommentPreviewLine struct {
	oldLine int
	newLine int
	code    string
	change  string
	hunk    int
}

func buildInlineCommentTargets(files []githubapi.PullRequestFile) []inlineCommentTarget {
	var targets []inlineCommentTarget
	for _, file := range files {
		oldLine, newLine := 0, 0
		hunk := 0
		var lines []inlineCommentPreviewLine
		for _, raw := range strings.Split(file.Patch, "\n") {
			if match := hunkHeader.FindStringSubmatch(raw); match != nil {
				oldLine, _ = strconv.Atoi(match[1])
				newLine, _ = strconv.Atoi(match[2])
				hunk++
				continue
			}
			switch {
			case strings.HasPrefix(raw, "+++ ") || strings.HasPrefix(raw, "--- ") || strings.HasPrefix(raw, "\\"):
				continue
			case strings.HasPrefix(raw, "+"):
				if newLine > 0 {
					lines = append(lines, inlineCommentPreviewLine{oldLine: oldLine, newLine: newLine, code: raw[1:], change: "added", hunk: hunk})
				}
				newLine++
			case strings.HasPrefix(raw, "-"):
				if oldLine > 0 {
					lines = append(lines, inlineCommentPreviewLine{oldLine: oldLine, newLine: newLine, code: raw[1:], change: "deleted", hunk: hunk})
				}
				oldLine++
			case strings.HasPrefix(raw, " "):
				if newLine > 0 {
					lines = append(lines, inlineCommentPreviewLine{oldLine: oldLine, newLine: newLine, code: raw[1:], change: "context", hunk: hunk})
				}
				oldLine++
				newLine++
			}
		}
		for i, line := range lines {
			if line.change == "context" {
				continue
			}
			start, end := i, i+1
			if i > 0 && lines[i-1].hunk == line.hunk {
				start--
			}
			if end < len(lines) && lines[end].hunk == line.hunk {
				end++
			}
			side := "RIGHT"
			if line.change == "deleted" {
				side = "LEFT"
			}
			lineNumber := line.newLine
			if side == "LEFT" {
				lineNumber = line.oldLine
			}
			targets = append(targets, inlineCommentTarget{
				path: file.Filename, line: lineNumber, side: side, code: line.code, change: line.change,
				preview: append([]inlineCommentPreviewLine(nil), lines[start:end]...), previewIndex: i - start,
			})
		}
	}
	return targets
}

func (target inlineCommentTarget) label() string {
	side := "new"
	if target.side == "LEFT" {
		side = "old"
	}
	return fmt.Sprintf("%s line %d", side, target.line)
}

func (target inlineCommentTarget) previewView(width, maxLines int) []string {
	if len(target.preview) == 0 {
		return nil
	}
	visible := min(len(target.preview), max(1, maxLines))
	start := min(max(0, target.previewIndex-visible/2), len(target.preview)-visible)
	result := make([]string, 0, visible)
	for i, line := range target.preview[start : start+visible] {
		index := start + i
		marker, prefix := "  ", "  "
		style := mutedStyle
		switch line.change {
		case "added":
			prefix, style = "+ ", addLineStyle
		case "deleted":
			prefix, style = "- ", delLineStyle
		}
		if index == target.previewIndex {
			marker = "› "
		}
		lineNumber := line.newLine
		if target.side == "LEFT" || line.change == "deleted" {
			lineNumber = line.oldLine
		}
		label := fmt.Sprintf("%s%4d %s", marker, lineNumber, prefix)
		codeWidth := max(1, width-lipgloss.Width(label))
		result = append(result, style.Render(label+clipLine(safeText(line.code), codeWidth)))
	}
	return result
}

func (target inlineCommentTarget) previewLineCount(height int) int {
	maxLines := 1
	if height >= 18 {
		maxLines = 3
	}
	return min(len(target.preview), maxLines)
}

func (m model) canCommentOnDiff() bool {
	return m.detailOpen && !m.detailLoading && m.detailError == "" && m.prDetails.Number > 0 &&
		m.prDetails.State == "open" && !m.prDetails.Merged && m.prDetails.Head.SHA != "" &&
		m.diffLoaded && !m.diffLoading && len(m.inlineCommentTargets) > 0 && !m.commentSending
}

func (m *model) openInlineCommentPicker() {
	if !m.canCommentOnDiff() {
		return
	}
	m.inlineCommentPickerOpen = true
	m.inlineCommentQuery = ""
	m.inlineCommentSelection = 0
}

func (m model) matchingInlineCommentTargets() []int {
	words := strings.Fields(strings.ToLower(m.inlineCommentQuery))
	var matches []int
	for i, target := range m.inlineCommentTargets {
		text := strings.ToLower(fmt.Sprintf("%s %s %s", target.path, target.label(), target.code))
		match := true
		for _, word := range words {
			if !strings.Contains(text, word) {
				match = false
				break
			}
		}
		if match {
			matches = append(matches, i)
		}
	}
	return matches
}

func (m model) updateInlineCommentPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	matches := m.matchingInlineCommentTargets()
	m.inlineCommentSelection = min(max(0, m.inlineCommentSelection), max(0, len(matches)-1))
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.inlineCommentPickerOpen = false
		m.inlineCommentQuery = ""
	case "up", "k", "ctrl+p", "shift+tab":
		m.inlineCommentSelection = max(0, m.inlineCommentSelection-1)
	case "down", "j", "ctrl+n", "tab":
		m.inlineCommentSelection = min(max(0, len(matches)-1), m.inlineCommentSelection+1)
	case "pgup":
		m.inlineCommentSelection = max(0, m.inlineCommentSelection-max(1, m.height/2))
	case "pgdown", "ctrl+d":
		m.inlineCommentSelection = min(max(0, len(matches)-1), m.inlineCommentSelection+max(1, m.height/2))
	case "home":
		m.inlineCommentSelection = 0
	case "end":
		m.inlineCommentSelection = max(0, len(matches)-1)
	case "enter":
		if len(matches) > 0 {
			target := m.inlineCommentTargets[matches[m.inlineCommentSelection]]
			m.inlineCommentPickerOpen = false
			m.openCommentComposer()
			m.commentOnDiff = true
			m.commentTarget = target
		}
	case "backspace", "ctrl+h":
		m.inlineCommentQuery = dropLastRune(m.inlineCommentQuery)
		m.inlineCommentSelection = 0
	case "ctrl+u":
		m.inlineCommentQuery = ""
		m.inlineCommentSelection = 0
	case " ":
		m.inlineCommentQuery += " "
		m.inlineCommentSelection = 0
	default:
		if msg.Type == tea.KeyRunes && !msg.Alt {
			m.inlineCommentQuery += safeText(string(msg.Runes))
			m.inlineCommentSelection = 0
		}
	}
	return m, nil
}

func (m model) inlineCommentPickerView(width int) []string {
	cardWidth := min(width, 88)
	inner := max(1, cardWidth-4)
	query := "⌕ " + searchViewport(m.inlineCommentQuery, len([]rune(m.inlineCommentQuery)), 0, inner-2)
	if m.inlineCommentQuery == "" {
		query = "⌕ ▌" + mutedStyle.Render(clipLine("Search file, line, or code…", inner-3))
	}
	content := []string{
		accentStyle.Render("COMMENT ON CODE"),
		textStyle.Render(clipLine(query, inner)),
	}
	if m.height >= 14 {
		content = append(content, "", mutedStyle.Render("Choose a changed line; its code and nearby context appear below"))
	}
	matches := m.matchingInlineCommentTargets()
	selected := min(max(0, m.inlineCommentSelection), max(0, len(matches)-1))
	if len(matches) == 0 {
		content = append(content, mutedStyle.Render(clipLine("No diff lines match this search", inner)))
	} else {
		target := m.inlineCommentTargets[matches[selected]]
		previewLines := target.previewLineCount(m.height)
		fixedRows := len(content) + 2 + previewLines + 2
		if m.height >= 14 {
			fixedRows += 2
		}
		visible := min(len(matches), max(1, m.height-fixedRows))
		start := min(max(0, selected-visible/2), len(matches)-visible)
		for row := start; row < start+visible; row++ {
			target := m.inlineCommentTargets[matches[row]]
			style := textStyle
			marker := "  "
			if row == selected {
				style = selectedStyle.Bold(true)
				marker = "▸ "
			}
			pathLine := fmt.Sprintf("%s%s  %s", marker, target.label(), safeText(target.path))
			content = append(content, style.Width(inner).Render(clipLine(pathLine, inner)))
		}
		content = append(content, "", accentStyle.Render(clipLine(fmt.Sprintf("Selected: %s in %s", target.label(), safeText(target.path)), inner)))
		content = append(content, target.previewView(inner, previewLines)...)
	}
	if m.height >= 14 {
		content = append(content, "", mutedStyle.Render("↑/↓ choose  ·  enter comment  ·  esc cancel"))
	}
	card := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(themeAccentColor)).Padding(0, 1).Width(inner + 2).Render(strings.Join(content, "\n"))
	margin := strings.Repeat(" ", 2+max(0, (width-cardWidth)/2))
	lines := strings.Split(card, "\n")
	for i := range lines {
		lines[i] = margin + lines[i]
	}
	return lines
}
