package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type paletteAction struct {
	name     string
	hint     string
	shortcut string
	run      func(*model) tea.Cmd
}

func (m model) paletteActions() []paletteAction {
	actions := []paletteAction{
		{"Pull requests", "Switch to your pull request inbox", "1", func(m *model) tea.Cmd { m.switchSection(0); return m.schedulePRPreview() }},
		{"Issues", "Switch to open issues", "2", func(m *model) tea.Cmd { m.switchSection(1); return nil }},
		{"Notifications", "Switch to GitHub notifications", "3", func(m *model) tea.Cmd { m.switchSection(2); return nil }},
		{"Refresh", "Reload the current view from GitHub", "r", func(m *model) tea.Cmd {
			updated, cmd := m.Update(keyForPalette("r"))
			*m = updated.(model)
			return cmd
		}},
		{"Edit GitHub filter", "Change the pull request search query", "/", func(m *model) tea.Cmd {
			m.beginSearch()
			return nil
		}},
		{"Switch theme", "Choose and save a color theme", "t", func(m *model) tea.Cmd {
			m.openThemePicker()
			return nil
		}},
	}
	if !m.detailOpen && m.activeSection == 0 && len(m.pullRequests) > 0 {
		actions = append(actions, paletteAction{"Review selected PR", "Open details and the continuous diff", "enter", func(m *model) tea.Cmd { return m.openSelectedPullRequest() }})
	}
	if !m.detailOpen && m.hasInboxPreview() {
		if m.inboxPreviewFocus {
			actions = append(actions, paletteAction{"Focus list", "Return to browsing inbox items", "tab", func(m *model) tea.Cmd { m.inboxPreviewFocus = false; return nil }})
		} else {
			actions = append(actions, paletteAction{"Focus preview", "Read and scroll the selected item's context", "tab", func(m *model) tea.Cmd { m.inboxPreviewFocus = true; return m.schedulePRPreview() }})
		}
	}
	if m.detailOpen {
		actions = append(actions,
			paletteAction{"Find changed file", "Search paths and jump straight to a patch", "f", func(m *model) tea.Cmd { m.openFilePicker(); return nil }},
			paletteAction{"Jump to diff", "Start reading the first changed file", "d", func(m *model) tea.Cmd { m.jumpToReviewFile(0); return nil }},
			paletteAction{"Back to inbox", "Return to your selection", "esc", func(m *model) tea.Cmd { m.detailOpen = false; return m.schedulePRPreview() }},
		)
		if m.canUpdateBranch() {
			actions = append(actions, paletteAction{
				"Update pull request branch", "Merge the base branch into this behind branch", "u",
				func(m *model) tea.Cmd {
					updated, cmd := m.Update(keyForPalette("u"))
					*m = updated.(model)
					return cmd
				},
			})
		}
		if m.mergeDialog == noMergeDialog && !m.mergeInProgress && !m.branchUpdateInProgress && !m.branchUpdateChecking {
			actions = append(actions, paletteAction{
				"Merge pull request", "Choose a merge method and confirm", "m",
				func(m *model) tea.Cmd {
					updated, cmd := m.Update(keyForPalette("m"))
					*m = updated.(model)
					return cmd
				},
			})
		}
	}
	actions = append(actions,
		paletteAction{"My pull requests", "Show open PRs authored by you", "", func(m *model) tea.Cmd { return m.applyPRQuery(defaultPRQuery) }},
		paletteAction{"Needs my review", "Show PRs requesting your review", "", func(m *model) tea.Cmd { return m.applyPRQuery(reviewRequestedQuery) }},
		paletteAction{"All open pull requests", "Show open PRs across repositories", "", func(m *model) tea.Cmd { return m.applyPRQuery(allOpenQuery) }},
	)
	if query, ok := ownedRepositoriesPRQuery(m.viewer.Login); ok {
		login := m.viewer.Login
		actions = append(actions, paletteAction{
			"Open PRs in my repositories", "Show open PRs in repositories owned by @" + login, "",
			func(m *model) tea.Cmd { return m.applyPRQuery(query) },
		})
	}
	return append(actions,
		paletteAction{"Help", "Show keyboard shortcuts", "?", func(m *model) tea.Cmd { m.showHelp = true; m.helpScroll = 0; return nil }},
		paletteAction{"Quit", "Exit glance", "q", func(*model) tea.Cmd { return tea.Quit }},
	)
}

func keyForPalette(text string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)}
}

func (m model) matchingPaletteActions() []paletteAction {
	words := strings.Fields(strings.ToLower(m.paletteQuery))
	actions := make([]paletteAction, 0)
	for _, action := range m.paletteActions() {
		text := strings.ToLower(action.name + " " + action.hint)
		match := true
		for _, word := range words {
			if !strings.Contains(text, word) {
				match = false
				break
			}
		}
		if match {
			actions = append(actions, action)
		}
	}
	return actions
}

func (m model) updatePaletteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	actions := m.matchingPaletteActions()
	m.paletteSelection = min(max(0, m.paletteSelection), max(0, len(actions)-1))
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.paletteOpen = false
		m.paletteQuery = ""
		return m, m.schedulePRPreview()
	case "up", "ctrl+p", "shift+tab":
		m.paletteSelection = max(0, m.paletteSelection-1)
	case "down", "ctrl+n", "tab":
		m.paletteSelection = min(max(0, len(actions)-1), m.paletteSelection+1)
	case "home":
		m.paletteSelection = 0
	case "end":
		m.paletteSelection = max(0, len(actions)-1)
	case "enter":
		if m.paletteSelection < len(actions) {
			m.paletteOpen = false
			m.paletteQuery = ""
			cmd := actions[m.paletteSelection].run(&m)
			return m, cmd
		}
	case "backspace", "ctrl+h":
		m.paletteQuery = dropLastRune(m.paletteQuery)
		m.paletteSelection = 0
	case "ctrl+u":
		m.paletteQuery = ""
		m.paletteSelection = 0
	case " ":
		m.paletteQuery += " "
		m.paletteSelection = 0
	default:
		if msg.Type == tea.KeyRunes && !msg.Alt {
			m.paletteQuery += safeText(string(msg.Runes))
			m.paletteSelection = 0
		}
	}
	return m, nil
}

func (m model) paletteView(width int) []string {
	cardWidth := min(width, 72)
	inner := max(1, cardWidth-4)
	query := ": " + searchViewport(m.paletteQuery, len([]rune(m.paletteQuery)), 0, inner-2)
	if m.paletteQuery == "" {
		query = ": ▌" + mutedStyle.Render(clipLine("Type a command…", inner-3))
	}
	content := []string{accentStyle.Render("COMMAND PALETTE"), textStyle.Render(query)}
	if m.height >= 12 {
		content = append(content, "")
	}
	actions := m.matchingPaletteActions()
	selected := min(max(0, m.paletteSelection), max(0, len(actions)-1))
	if len(actions) == 0 {
		content = append(content, mutedStyle.Render(clipLine("No matching commands", inner)))
	} else {
		descriptionRows := 0
		if m.height >= 16 {
			descriptionRows = 2
		}
		visible := min(len(actions), max(1, m.height-7-len(content)-descriptionRows))
		start := min(max(0, selected-visible+1), len(actions)-visible)
		for i := start; i < start+visible; i++ {
			label := "  " + actions[i].name
			if i == selected {
				label = "▸ " + actions[i].name
			}
			row := align(clipLine(label, inner-lipgloss.Width(actions[i].shortcut)-1), actions[i].shortcut, inner)
			if i == selected {
				row = selectedStyle.Bold(true).Width(inner).Render(row)
			} else {
				row = textStyle.Render(row)
			}
			content = append(content, row)
		}
	}
	if m.height >= 16 {
		hint := "Try a shorter query · ctrl+u clears"
		if len(actions) > 0 {
			hint = actions[selected].hint
		}
		content = append(content, "", mutedStyle.Render(clipLine(hint, inner)))
	}
	card := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(themeAccentColor)).Padding(0, 1).Width(inner + 2).Render(strings.Join(content, "\n"))
	margin := strings.Repeat(" ", 2+max(0, (width-cardWidth)/2))
	lines := strings.Split(card, "\n")
	for i := range lines {
		lines[i] = margin + lines[i]
	}
	return lines
}
