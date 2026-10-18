package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNavigation(t *testing.T) {
	tests := []struct {
		name  string
		start int
		key   tea.KeyType
		want  int
	}{
		{name: "next section", start: 0, key: tea.KeyRight, want: 1},
		{name: "previous section", start: 1, key: tea.KeyLeft, want: 0},
		{name: "wrap forward", start: len(sections) - 1, key: tea.KeyRight, want: 0},
		{name: "wrap backward", start: 0, key: tea.KeyLeft, want: len(sections) - 1},
		{name: "tab advances", start: 0, key: tea.KeyTab, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := initialModel()
			m.activeSection = tt.start
			updated, cmd := m.Update(tea.KeyMsg{Type: tt.key})
			if cmd != nil {
				t.Fatal("navigation unexpectedly returned a command")
			}
			if got := updated.(model).activeSection; got != tt.want {
				t.Fatalf("active section = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestQuitKey(t *testing.T) {
	_, cmd := initialModel().Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("quit key returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("quit key did not return a quit message")
	}
}

func TestViewShowsDashboardAndPlaceholder(t *testing.T) {
	view := initialModel().View()
	for _, want := range []string{"glance", "Overview", "Pull requests", "Welcome to glance", "GitHub data integration is not implemented yet."} {
		if !strings.Contains(view, want) {
			t.Errorf("view %q does not contain %q", view, want)
		}
	}
}
