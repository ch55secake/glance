package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestFilterCursorEditsWithoutChangingSections(t *testing.T) {
	m := initialModel()
	m.loading = false
	updated, _ := m.Update(key("/"))
	m = updated.(model)
	if m.searchCursor != len([]rune(defaultPRQuery)) || !m.searching {
		t.Fatalf("filter cursor did not start at query end: %+v", m)
	}
	for _, msg := range []tea.KeyMsg{{Type: tea.KeyHome}, key("界"), {Type: tea.KeyRight}, {Type: tea.KeyDelete}} {
		updated, _ = m.Update(msg)
		m = updated.(model)
	}
	if m.searchDraft != "界i:pr is:open author:@me" || m.searchCursor != 2 || m.activeSection != 0 {
		t.Fatalf("cursor editing changed query or section: %+v", m)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = updated.(model)
	if m.searchDraft != "界:pr is:open author:@me" || m.searchCursor != 1 {
		t.Fatalf("backspace did not remove rune before cursor: %+v", m)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = updated.(model)
	if m.searchCursor != len([]rune(m.searchDraft)) {
		t.Fatal("End did not move to query end")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	if m.searching || m.query != defaultPRQuery || m.searchCursor != 0 {
		t.Fatalf("cancel did not restore applied filter: %+v", m)
	}
}

func TestLongFilterScrollsAroundCursor(t *testing.T) {
	m := initialModel()
	m.width, m.height, m.loading = 30, 16, false
	updated, _ := m.Update(key("/"))
	m = updated.(model)
	end := strings.Join(m.githubFilterBox(m.width-4), "\n")
	if !strings.Contains(end, "author:@me") || !strings.Contains(end, "▌") || !strings.Contains(end, "…") {
		t.Fatalf("long query end or cursor hidden: %s", end)
	}
	for range len([]rune(m.searchDraft)) {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
		m = updated.(model)
	}
	start := strings.Join(m.githubFilterBox(m.width-4), "\n")
	if m.searchCursor != 0 || !strings.Contains(start, "▌is:pr") || !strings.Contains(start, "…") || m.activeSection != 0 {
		t.Fatalf("left did not scroll to query start: %s", start)
	}
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(model)
	for _, line := range m.githubFilterBox(m.width - 4) {
		if lipgloss.Width(line) > m.width-4 {
			t.Errorf("filter overflows after resize: %q", line)
		}
	}
}

func TestFilterInsertsAndDeletesAtUnicodeCursor(t *testing.T) {
	m := initialModel()
	m.loading, m.searching = false, true
	m.searchDraft = "a界c"
	m.searchCursor = 2
	updated, _ := m.Update(key("🙂"))
	m = updated.(model)
	if m.searchDraft != "a界🙂c" || m.searchCursor != 3 {
		t.Fatalf("insert split a Unicode rune: %+v", m)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = updated.(model)
	if m.searchDraft != "a界c" || m.searchCursor != 2 {
		t.Fatalf("backspace split a Unicode rune: %+v", m)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDelete})
	m = updated.(model)
	if m.searchDraft != "a界" || m.searchCursor != 2 {
		t.Fatalf("Delete removed the wrong rune: %+v", m)
	}
}

func TestFilterWordEditingAndWiderViewport(t *testing.T) {
	m := initialModel()
	m.width, m.loading = 30, false
	m.query = "is:pr repo:team/変更 author:@me"
	m = applyKeys(m, key("/"), tea.KeyMsg{Type: tea.KeyCtrlLeft})
	if string([]rune(m.searchDraft)[m.searchCursor:]) != "author:@me" {
		t.Fatal("word movement did not reach the start of the last qualifier")
	}
	m = applyKeys(m, tea.KeyMsg{Type: tea.KeyCtrlW})
	if m.searchDraft != "is:pr author:@me" || m.query != "is:pr repo:team/変更 author:@me" {
		t.Fatalf("word deletion damaged the query or applied it early: %q", m.searchDraft)
	}
	m = applyKeys(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}, Alt: true}, key(" "), key("draft:false"))
	if m.searchDraft != "is:pr author:@me draft:false" {
		t.Fatalf("alt+f inserted literal text or moved to the wrong boundary: %q", m.searchDraft)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m = updated.(model)
	box := strings.Join(m.githubFilterBox(m.width-4), "\n")
	if m.searchOffset != 0 || !strings.Contains(box, m.searchDraft+"▌") {
		t.Fatalf("widening the field kept short text hidden: %s", box)
	}
}
