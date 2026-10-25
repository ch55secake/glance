package main

import (
	"crypto/sha256"
	"fmt"
	"io"
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

type reviewedFileSnapshot struct {
	previousFilename string
	status           string
	additions        int
	deletions        int
	changes          int
	patch            [sha256.Size]byte
}

func snapshotReviewFile(file githubapi.PullRequestFile) reviewedFileSnapshot {
	hash := sha256.New()
	_, _ = io.Copy(hash, strings.NewReader(file.Patch))
	var patch [sha256.Size]byte
	copy(patch[:], hash.Sum(nil))
	return reviewedFileSnapshot{
		previousFilename: file.PreviousFilename,
		status:           file.Status,
		additions:        file.Additions,
		deletions:        file.Deletions,
		changes:          file.Changes,
		patch:            patch,
	}
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

func (m model) reviewPosition() (string, int) {
	index := m.currentReviewFile()
	if index < 0 {
		return "", 0
	}
	return m.diffFiles[index].Filename, m.detailScroll - m.reviewFiles[index].row
}

func (m *model) restoreReviewPosition(path string, offset int) {
	for _, anchor := range m.reviewFiles {
		if anchor.index >= 0 && anchor.index < len(m.diffFiles) && m.diffFiles[anchor.index].Filename == path {
			m.detailScroll = min(anchor.row+offset, anchor.end-1)
			return
		}
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
	return ok && index < len(m.diffFileSnapshots) && previous == m.diffFileSnapshots[index]
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
	viewed := make(map[string]reviewedFileSnapshot)
	snapshots := make([]reviewedFileSnapshot, len(files))
	for i, file := range files {
		snapshot := snapshotReviewFile(file)
		snapshots[i] = snapshot
		if previous, ok := m.reviewedFiles[file.Filename]; ok && previous == snapshot {
			viewed[file.Filename] = snapshot
		}
	}
	m.reviewedFiles = viewed
	m.diffFileSnapshots = snapshots
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
		m.reviewedFiles = make(map[string]reviewedFileSnapshot)
	}
	if index >= len(m.diffFileSnapshots) {
		m.diffFileSnapshots = make([]reviewedFileSnapshot, len(m.diffFiles))
		for i, current := range m.diffFiles {
			m.diffFileSnapshots[i] = snapshotReviewFile(current)
		}
	}
	m.reviewedFiles[file.Filename] = m.diffFileSnapshots[index]
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

func (m model) filePickerSelectedPath() string {
	files := m.matchingFiles()
	if m.filePickerSelection < 0 || m.filePickerSelection >= len(files) {
		return ""
	}
	return m.diffFiles[files[m.filePickerSelection]].Filename
}

func (m *model) restoreFilePickerSelection(path string) {
	if path == "" {
		return
	}
	for i, index := range m.matchingFiles() {
		if m.diffFiles[index].Filename == path {
			m.filePickerSelection = i
			return
		}
	}
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
