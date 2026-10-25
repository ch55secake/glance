package main

import (
	"fmt"
	"strings"

	"glance/internal/githubapi"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type reviewFileAnchor struct {
	index int
	row   int
	end   int
}

type reviewDocument struct {
	lines []string
	files []reviewFileAnchor
}

func appendReviewDiff(intro []string, diff reviewDocument) reviewDocument {
	for i := range diff.files {
		diff.files[i].row += len(intro)
		diff.files[i].end += len(intro)
	}
	diff.lines = append(intro, diff.lines...)
	return diff
}

func (m model) currentReviewFile() int {
	for i := len(m.reviewFiles) - 1; i >= 0; i-- {
		file := m.reviewFiles[i]
		if m.detailScroll >= file.row {
			if i == len(m.reviewFiles)-1 && m.detailScroll >= file.end {
				return -1
			}
			return file.index
		}
	}
	return -1
}

func (m model) reviewPosition() (int, int) {
	index := m.currentReviewFile()
	if index < 0 {
		return -1, 0
	}
	return index, m.detailScroll - m.reviewFiles[index].row
}

func (m *model) restoreReviewPosition(index, offset int) {
	if index >= 0 && index < len(m.reviewFiles) {
		file := m.reviewFiles[index]
		m.detailScroll = min(file.row+offset, file.end-1)
	}
}

func (m *model) jumpToReviewFile(index int) {
	m.cacheDetailLines()
	for _, file := range m.reviewFiles {
		if file.index == index {
			m.detailScroll = file.row
			m.sidebarFocus = false
			return
		}
	}
}

func (m *model) stepReviewFile(direction int) {
	if len(m.reviewFiles) == 0 {
		return
	}
	current := m.currentReviewFile()
	if current < 0 && m.detailScroll >= m.reviewFiles[len(m.reviewFiles)-1].end {
		current = len(m.reviewFiles)
	}
	m.jumpToReviewFile(min(max(0, current+direction), len(m.reviewFiles)-1))
}

func (m model) fileReviewed(index int) bool {
	if index < 0 || index >= len(m.diffFiles) {
		return false
	}
	file := m.diffFiles[index]
	previous, ok := m.reviewedFiles[file.Filename]
	return ok && previous == file
}

func (m model) reviewedFileCount() int {
	count := 0
	for i := range m.diffFiles {
		if m.fileReviewed(i) {
			count++
		}
	}
	return count
}

func (m *model) retainViewedFiles(files []githubapi.PullRequestFile) {
	if len(m.reviewedFiles) == 0 {
		return
	}
	viewed := make(map[string]githubapi.PullRequestFile)
	for _, file := range files {
		if previous, ok := m.reviewedFiles[file.Filename]; ok && previous == file {
			viewed[file.Filename] = file
		}
	}
	m.reviewedFiles = viewed
}

func (m *model) toggleReviewedFile() {
	index := m.currentReviewFile()
	if index < 0 || index >= len(m.diffFiles) {
		return
	}
	file := m.diffFiles[index]
	if m.fileReviewed(index) {
		delete(m.reviewedFiles, file.Filename)
		return
	}
	if m.reviewedFiles == nil {
		m.reviewedFiles = make(map[string]githubapi.PullRequestFile)
	}
	m.reviewedFiles[file.Filename] = file
}

func (m model) reviewLocation(width int) string {
	progress := fmt.Sprintf("%d/%d viewed · f files", m.reviewedFileCount(), len(m.diffFiles))
	available := width
	if width >= 55 {
		available -= lipgloss.Width(progress) + 2
	}
	label := "▤ Description"
	if index := m.currentReviewFile(); index >= 0 && index < len(m.diffFiles) {
		marker := "□ "
		if m.fileReviewed(index) {
			marker = "✓ "
		}
		label = marker + clipFilePath(safeText(m.diffFiles[index].Filename), available-2)
	} else if len(m.reviewFiles) > 0 && m.detailScroll >= m.reviewFiles[len(m.reviewFiles)-1].end {
		label = "◉ Checks & review activity"
	}
	if width < 55 {
		return "  " + fileLineStyle.Width(width).Render(clipLine(label, width))
	}
	return "  " + fileLineStyle.Width(width).Render(align(clipLine(label, available), progress, width))
}

func (m *model) openFilePicker() {
	m.cacheDetailLines()
	m.filePickerOpen = true
	m.filePickerQuery = ""
	m.filePickerSelection = max(0, m.currentReviewFile())
}

func (m model) matchingFiles() []int {
	words := strings.Fields(strings.ToLower(m.filePickerQuery))
	var matches []int
	for i, file := range m.diffFiles {
		text := strings.ToLower(file.Filename + " " + file.PreviousFilename)
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

func (m model) updateFilePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	files := m.matchingFiles()
	m.filePickerSelection = min(max(0, m.filePickerSelection), max(0, len(files)-1))
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.filePickerOpen = false
	case "up", "ctrl+p", "shift+tab":
		m.filePickerSelection = max(0, m.filePickerSelection-1)
	case "down", "ctrl+n", "tab":
		m.filePickerSelection = min(max(0, len(files)-1), m.filePickerSelection+1)
	case "home":
		m.filePickerSelection = 0
	case "end":
		m.filePickerSelection = max(0, len(files)-1)
	case "enter":
		if len(files) > 0 {
			m.jumpToReviewFile(files[m.filePickerSelection])
			m.filePickerOpen = false
		}
	case "backspace", "ctrl+h":
		m.filePickerQuery = dropLastRune(m.filePickerQuery)
		m.filePickerSelection = 0
	case "ctrl+u":
		m.filePickerQuery = ""
		m.filePickerSelection = 0
	case " ":
		m.filePickerQuery += " "
		m.filePickerSelection = 0
	default:
		if msg.Type == tea.KeyRunes && !msg.Alt {
			m.filePickerQuery += safeText(string(msg.Runes))
			m.filePickerSelection = 0
		}
	}
	return m, nil
}

func (m model) filePickerView(width int) []string {
	cardWidth := min(width, 100)
	inner := cardWidth - 4
	height := m.height - 5
	input := "⌕ " + searchViewport(m.filePickerQuery, len([]rune(m.filePickerQuery)), 0, inner-2)
	if m.filePickerQuery == "" {
		input = "⌕ ▌" + mutedStyle.Render(clipLine("Find a changed file…", inner-3))
	}
	content := []string{textStyle.Render(input)}
	if m.height >= 16 {
		content = append(content, "")
	}
	files := m.matchingFiles()
	selected := min(max(0, m.filePickerSelection), max(0, len(files)-1))
	if len(files) == 0 {
		message := "No files match this path"
		switch {
		case m.diffLoading:
			message = "Loading changed files…"
		case m.diffError != "":
			message = "Diff unavailable · esc then r to retry"
		case len(m.diffFiles) == 0:
			message = "No changed files"
		}
		content = append(content, mutedStyle.Render(clipLine(message, inner)))
	} else {
		rowHeight := 1
		if m.height >= 20 {
			rowHeight = 2
		}
		visible := min(len(files), max(1, (height-2-len(content))/rowHeight))
		start := min(max(0, selected-visible/2), len(files)-visible)
		for row := start; row < start+visible; row++ {
			index := files[row]
			file := m.diffFiles[index]
			marker := "  □ "
			if m.fileReviewed(index) {
				marker = "  ✓ "
			}
			style := textStyle
			if row == selected {
				marker = "▸" + marker[1:]
				style = selectedStyle.Bold(true)
			}
			content = append(content, style.Width(inner).Render(marker+clipFilePath(safeText(file.Filename), inner-4)))
			if rowHeight == 2 {
				metadata := fmt.Sprintf("%s · +%d −%d", safeText(file.Status), file.Additions, file.Deletions)
				if file.PreviousFilename != "" {
					metadata += " · from " + safeText(file.PreviousFilename)
				}
				content = append(content, mutedStyle.Render("    "+clipLine(metadata, inner-4)))
			}
		}
	}
	status := fmt.Sprintf("%d matches · %d/%d viewed", len(files), m.reviewedFileCount(), len(m.diffFiles))
	lines := panel(cardWidth, min(height, max(5, len(content)+2)), "CHANGED FILES", status, content, true)
	margin := strings.Repeat(" ", 2+(width-cardWidth)/2)
	for i := range lines {
		lines[i] = margin + lines[i]
	}
	return lines
}

// Retain the basename when paths are too long for the picker.
func clipFilePath(path string, width int) string {
	if lipgloss.Width(path) <= width {
		return path
	}
	runes := []rune(path)
	for len(runes) > 0 && lipgloss.Width(string(runes)) > width-1 {
		runes = runes[1:]
	}
	return "…" + string(runes)
}
