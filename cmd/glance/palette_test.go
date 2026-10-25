package main

import (
	"strings"
	"testing"

	"glance/internal/githubapi"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestCommandPaletteFiltersAndRunsActions(t *testing.T) {
	m := initialModel()
	m.loading = false
	m.pullRequests = []githubapi.PullRequest{{Number: 42, Repository: "team/repo", Title: "PR"}}
	updated, cmd := m.Update(key(":"))
	m = updated.(model)
	if cmd != nil || !m.paletteOpen || !strings.Contains(m.View(), "COMMAND PALETTE") {
		t.Fatalf("colon did not open palette: %+v", m)
	}
	for _, char := range "issues" {
		updated, _ = m.Update(key(string(char)))
		m = updated.(model)
	}
	if len(m.matchingPaletteActions()) != 1 || m.matchingPaletteActions()[0].name != "Issues" {
		t.Fatalf("palette search does not match issues: %q", m.paletteQuery)
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if cmd != nil || m.paletteOpen || m.activeSection != 1 {
		t.Fatalf("palette did not switch inbox: %+v", m)
	}
	updated, _ = m.Update(key(":"))
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.(model).activeSection != 1 || updated.(model).paletteOpen {
		t.Fatal("canceling palette changed inbox")
	}
}

func TestCommandPalettePreservesReviewAndFilterModes(t *testing.T) {
	m := initialModel()
	m.loading = false
	m.pullRequests = []githubapi.PullRequest{{Number: 42, Repository: "team/repo"}}
	updated, _ := m.Update(key(":"))
	m = updated.(model)
	for _, char := range "review" {
		updated, _ = m.Update(key(string(char)))
		m = updated.(model)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if cmd == nil || !m.detailOpen || m.paletteOpen || m.detailKey != "team/repo#42" {
		t.Fatalf("palette review did not open selected PR: %+v", m)
	}
	updated, _ = m.Update(key(":"))
	m = updated.(model)
	if !m.paletteOpen || !strings.Contains(m.View(), "COMMAND PALETTE") {
		t.Fatal("palette unavailable from review")
	}
	for _, char := range "filter" {
		updated, _ = m.Update(key(string(char)))
		m = updated.(model)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.detailOpen || !m.searching || m.searchDraft != defaultPRQuery || m.paletteOpen {
		t.Fatalf("filter action did not open PR query editor: %+v", m)
	}
	updated, _ = m.Update(key(":"))
	if updated.(model).paletteOpen || updated.(model).searchDraft != defaultPRQuery+":" {
		t.Fatal("colon inside filter editor unexpectedly opened palette")
	}
}

func TestCommandPaletteLayoutAndMonochrome(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
	for _, size := range []struct{ width, height int }{{30, 16}, {45, 18}, {80, 24}, {120, 36}} {
		m := initialModel()
		m.width, m.height, m.paletteOpen = size.width, size.height, true
		view := m.View()
		if strings.Contains(view, "\x1b[") || !strings.Contains(view, "COMMAND PALETTE") {
			t.Errorf("palette lost plain-text rendering at %dx%d: %s", size.width, size.height, view)
		}
		if lines := strings.Split(view, "\n"); len(lines) > size.height {
			t.Errorf("palette uses %d rows at height %d", len(lines), size.height)
		}
		for _, line := range strings.Split(view, "\n") {
			if lipgloss.Width(line) > size.width {
				t.Errorf("palette overflows %d columns: %q", size.width, line)
			}
		}
	}
}
