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
	path   string
	line   int
	side   string
	code   string
	change string
}

func buildInlineCommentTargets(files []githubapi.PullRequestFile) []inlineCommentTarget {
	var targets []inlineCommentTarget
	for _, file := range files {
		oldLine, newLine := 0, 0
		for _, raw := range strings.Split(file.Patch, "\n") {
			if match := hunkHeader.FindStringSubmatch(raw); match != nil {
				oldLine, _ = strconv.Atoi(match[1])
				newLine, _ = strconv.Atoi(match[2])
				continue
			}
			switch {
			case strings.HasPrefix(raw, "+++ ") || strings.HasPrefix(raw, "--- ") || strings.HasPrefix(raw, "\\"):
				continue
			case strings.HasPrefix(raw, "+"):
				if newLine > 0 {
					targets = append(targets, inlineCommentTarget{path: file.Filename, line: newLine, side: "RIGHT", code: raw[1:], change: "added"})
				}
				newLine++
			case strings.HasPrefix(raw, "-"):
				if oldLine > 0 {
					targets = append(targets, inlineCommentTarget{path: file.Filename, line: oldLine, side: "LEFT", code: raw[1:], change: "deleted"})
				}
				oldLine++
			case strings.HasPrefix(raw, " "):
				oldLine++
				newLine++
			}
		}
	}
	return targets
}

func (target inlineCommentTarget) label() string {
	side := "R"
	if target.side == "LEFT" {
		side = "L"
	}
	return fmt.Sprintf("%s%d", side, target.line)
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
		text := strings.ToLower(fmt.Sprintf("%s %d %s %s", target.path, target.line, target.side, target.code))
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
		content = append(content, "", mutedStyle.Render("Choose a line · R current side · L removed side"))
	}
	matches := m.matchingInlineCommentTargets()
	selected := min(max(0, m.inlineCommentSelection), max(0, len(matches)-1))
	if len(matches) == 0 {
		content = append(content, mutedStyle.Render(clipLine("No diff lines match this search", inner)))
	} else {
		rowHeight := 1
		if m.height >= 20 {
			rowHeight = 2
		}
		visible := min(len(matches), max(1, (m.height-10-len(content))/rowHeight))
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
			if rowHeight == 2 {
				content = append(content, mutedStyle.Width(inner).Render("    "+clipLine(safeText(target.code), inner-4)))
			}
		}
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
