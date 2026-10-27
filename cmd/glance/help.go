package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m model) helpViewportHeight() int { return max(1, m.height-7) }

func (m model) maxHelpScroll() int {
	return max(0, len(m.helpContent(max(1, m.width-4)))-m.helpViewportHeight())
}

func (m model) helpContent(width int) []string {
	type group struct {
		name string
		keys []shortcut
	}
	groups := []group{{"EVERYWHERE", []shortcut{
		{":", "Search commands and actions"}, {"t", "Choose and save a color theme"}, {"?", "Show keyboard shortcuts"},
		{"1–3", "Switch inbox sections"}, {"r", "Refresh the current view"}, {"q / ctrl+c", "Quit glance"},
	}}}
	if m.detailOpen {
		groups = append(groups, group{"PR REVIEW", []shortcut{
			{"↑/↓ · j/k", "Scroll the active pane"}, {"tab / shift+tab", "Switch between diff and readiness"},
			{"PgUp / PgDn", "Scroll a screen"}, {"Home / End", "Jump to the first or last line"},
			{"f", "Find a changed file by path"}, {"[ / ]", "Jump to the previous / next file"},
			{"d", "Jump straight to the diff"}, {"v", "Toggle viewed for the current file (local)"},
			{"c", "Write a conversation comment on this PR"},
			{"C", "Choose a changed line for an inline comment"},
			{"m", "Choose a merge method, then confirm"}, {"/", "Return to editing the GitHub filter"}, {"esc", "Return to the inbox"},
		}})
	} else {
		groups = append(groups, group{"INBOX", []shortcut{
			{"←/→ · h/l · 1–3", "Switch sections"}, {"↑/↓ · j/k", "Select an item or scroll its preview"},
			{"tab / shift+tab", "Switch between list and preview"}, {"esc", "Return focus to the list"},
			{"PgUp / PgDn", "Move a screen through the active pane"}, {"Home / End", "Jump to the first or last item or line"},
		}})
		if m.activeSection == 0 {
			groups = append(groups, group{"PULL REQUESTS", []shortcut{
				{"enter", "Open the selected PR and its diff"}, {"/", "Edit the GitHub filter"},
			}}, group{"FILTER EDITOR", []shortcut{
				{"←/→", "Move the cursor"}, {"Home/End · ctrl+a/e", "Jump to the start or end"},
				{"ctrl+←/→ · alt+b/f", "Move by a word"}, {"Backspace / Delete", "Delete before or after the cursor"},
				{"ctrl+w / ctrl+u", "Delete a word / clear the filter"},
				{"enter / esc", "Apply / cancel changes"},
			}})
		}
	}
	var lines []string
	for i, group := range groups {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "  "+accentStyle.Render(group.name))
		for _, key := range group.keys {
			if width < 55 {
				lines = append(lines, "  "+textStyle.Bold(true).Render(clipLine(key.key, width)))
				for _, part := range wrapPreviewWords(key.label, max(1, width-2)) {
					lines = append(lines, "    "+mutedStyle.Render(part))
				}
				continue
			}
			padding := strings.Repeat(" ", max(1, 23-lipgloss.Width(key.key)))
			for j, part := range wrapPreviewWords(key.label, width-23) {
				prefix := strings.Repeat(" ", 23)
				if j == 0 {
					prefix = textStyle.Bold(true).Render(key.key) + padding
				}
				lines = append(lines, "  "+prefix+mutedStyle.Render(part))
			}
		}
	}
	return lines
}
