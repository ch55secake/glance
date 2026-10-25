package main

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestThemeSwitcherLivesInPaletteAndPersistsSelection(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	previousTheme := applyTheme(defaultThemeID)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previousProfile)
		applyTheme(previousTheme)
	})
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	m := initialModel()
	m.width, m.height, m.loading = 120, 24, false
	m = applyKeys(m, key(":"), key("theme"), tea.KeyMsg{Type: tea.KeyEnter})
	if !m.themePickerOpen || m.paletteOpen || !strings.Contains(m.View(), "COLOR THEMES") {
		t.Fatal("palette did not open the theme picker")
	}
	m = applyKeys(m, tea.KeyMsg{Type: tea.KeyDown}, tea.KeyMsg{Type: tea.KeyEnter})
	if m.theme != "nord" || m.themePickerOpen || m.themeConfigError != "" {
		t.Fatalf("theme choice did not apply cleanly: %+v", m)
	}
	if !strings.Contains(m.View(), "38;2;136;192;208") {
		t.Fatal("theme selection did not recolor the workspace")
	}
	saved, err := loadThemePreference()
	if err != nil || saved != "nord" {
		t.Fatalf("theme selection was not persisted: %q, %v", saved, err)
	}
	restarted := initialModel()
	restarted.theme = applyTheme(saved)
	restarted.width, restarted.height = 120, 24
	if restarted.theme != "nord" || themeAccentColor != "#B48EAD" || !strings.Contains(restarted.View(), "glance") {
		t.Fatal("saved theme was not restored for a new session")
	}
}

func TestEveryThemeProvidesCompleteStyles(t *testing.T) {
	previous := applyTheme(defaultThemeID)
	t.Cleanup(func() { applyTheme(previous) })
	for _, candidate := range themes {
		if got := applyTheme(candidate.id); got != candidate.id || themeAccentColor != candidate.accent ||
			syntaxKeyword != candidate.keyword || syntaxString != candidate.codeString || syntaxNumber != candidate.number ||
			syntaxComment != candidate.comment {
			t.Errorf("theme %q did not apply its full palette", candidate.id)
		}
		markdown := markdownThemeStyles()
		if markdown.Heading.Color == nil || *markdown.Heading.Color != candidate.accent ||
			markdown.CodeBlock.Chroma == nil || markdown.CodeBlock.Chroma.Keyword.Color == nil ||
			*markdown.CodeBlock.Chroma.Keyword.Color != candidate.keyword {
			t.Errorf("theme %q did not update Markdown colors", candidate.id)
		}
	}
}

func TestSwitchingThemeRecolorsAndRebuildsAnOpenReview(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	previousTheme := applyTheme(defaultThemeID)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previousProfile)
		applyTheme(previousTheme)
	})
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := reviewNavigationModel()
	oldLines := &m.detailLines[0]
	m = applyKeys(m, key(":"), key("theme"), tea.KeyMsg{Type: tea.KeyEnter}, tea.KeyMsg{Type: tea.KeyDown}, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.detailOpen || m.theme != "nord" || &m.detailLines[0] == oldLines {
		t.Fatal("theme change did not rebuild the open review")
	}
	if !strings.Contains(strings.Join(m.detailLines, "\n"), "38;2;179;142;173") {
		t.Fatal("review diff did not adopt Nord syntax and hunk colors")
	}
}

func TestThemePickerCanCancelAndKeepsThemeUnchanged(t *testing.T) {
	previous := applyTheme(defaultThemeID)
	t.Cleanup(func() { applyTheme(previous) })
	m := initialModel()
	m.width, m.height, m.loading = 100, 24, false
	m = applyKeys(m, key("t"), tea.KeyMsg{Type: tea.KeyDown}, tea.KeyMsg{Type: tea.KeyEsc})
	if m.themePickerOpen || m.theme != defaultThemeID {
		t.Fatal("cancel changed or left the theme picker open")
	}
}

func TestThemeSaveFailureIsVisibleWithoutBlockingTheSession(t *testing.T) {
	previous := applyTheme(defaultThemeID)
	t.Cleanup(func() { applyTheme(previous) })
	root := t.TempDir()
	file := root + "/not-a-directory"
	if err := os.WriteFile(file, []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", file)
	m := initialModel()
	m.width, m.height = 100, 24
	m = applyKeys(m, key(":"), key("theme"), tea.KeyMsg{Type: tea.KeyEnter}, tea.KeyMsg{Type: tea.KeyDown}, tea.KeyMsg{Type: tea.KeyEnter})
	if m.theme != "nord" || m.themeConfigError == "" || !strings.Contains(m.header(96), "CONFIG ERROR") {
		t.Fatal("failed persistence blocked the theme or hid its warning")
	}
}

func TestThemePickerLayoutFitsAndShowsSelectedTheme(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	previousTheme := applyTheme(defaultThemeID)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previousProfile)
		applyTheme(previousTheme)
	})
	for _, profile := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		lipgloss.SetColorProfile(profile)
		for _, size := range []struct{ width, height int }{{30, 10}, {45, 16}, {80, 18}, {120, 24}} {
			m := initialModel()
			m.width, m.height, m.loading, m.themePickerOpen = size.width, size.height, false, true
			m.themePickerSelection = len(themes) - 1
			view := m.View()
			if !strings.Contains(view, "Solarized") || len(strings.Split(view, "\n")) > size.height {
				t.Errorf("%v did not reveal the selected theme:\n%s", size, view)
			}
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > size.width {
					t.Errorf("%v theme picker overflows: %q", size, line)
				}
			}
			if profile == termenv.Ascii && strings.Contains(view, "\x1b[") {
				t.Errorf("%v monochrome picker emitted ANSI colors", size)
			}
		}
	}
}
