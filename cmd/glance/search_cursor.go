package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *model) insertSearchText(text string) {
	runes := []rune(m.searchDraft)
	cursor := min(max(0, m.searchCursor), len(runes))
	m.searchDraft = string(runes[:cursor]) + text + string(runes[cursor:])
	m.searchCursor = cursor + len([]rune(text))
}

func (m *model) deleteSearchRune(backward bool) {
	runes := []rune(m.searchDraft)
	cursor := min(max(0, m.searchCursor), len(runes))
	if backward {
		if cursor == 0 {
			return
		}
		if cursor == len(runes) {
			m.searchDraft = dropLastRune(m.searchDraft)
			m.searchCursor = cursor - 1
			return
		}
		m.searchDraft = string(runes[:cursor-1]) + string(runes[cursor:])
		m.searchCursor = cursor - 1
		return
	}
	if cursor < len(runes) {
		m.searchDraft = string(runes[:cursor]) + string(runes[cursor+1:])
	}
}

func (m model) searchFieldWidth() int {
	return max(1, min(max(1, m.width-4), 68)-7)
}

func searchViewportStart(text string, cursor, offset, width int) int {
	runes := []rune(text)
	cursor = min(max(0, cursor), len(runes))
	offset = min(max(0, offset), cursor)
	for offset < cursor {
		prefix := 0
		if offset > 0 {
			prefix = 1 // leading ellipsis
		}
		if lipgloss.Width(string(runes[offset:cursor]))+prefix+1 <= width {
			break // leave room for the cursor
		}
		offset++
	}
	return offset
}

func (m *model) keepSearchCursorVisible() {
	m.searchOffset = searchViewportStart(m.searchDraft, m.searchCursor, m.searchOffset, m.searchFieldWidth())
}

func searchViewport(text string, cursor, offset, width int) string {
	runes := []rune(text)
	cursor = min(max(0, cursor), len(runes))
	offset = searchViewportStart(text, cursor, offset, width)
	var out strings.Builder
	if offset > 0 {
		out.WriteRune('…')
	}
	out.WriteString(string(runes[offset:cursor]))
	out.WriteRune('▌')
	remaining := width - lipgloss.Width(out.String())
	for i := cursor; i < len(runes); i++ {
		charWidth := lipgloss.Width(string(runes[i]))
		if charWidth > remaining {
			if remaining >= 1 {
				out.WriteRune('…')
			}
			break
		}
		if i < len(runes)-1 && charWidth == remaining {
			out.WriteRune('…')
			break
		}
		out.WriteRune(runes[i])
		remaining -= charWidth
	}
	return out.String()
}
