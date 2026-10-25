package main

import "fmt"

func (m *model) switchSection(section int) {
	m.detailOpen = false
	if m.activeSection != section {
		m.inboxPreviewFocus = false
		m.inboxPreviewScroll = 0
	}
	m.activeSection = section
}

func (m *model) beginSearch() {
	m.switchSection(0)
	m.inboxPreviewFocus = false
	m.searching = true
	m.searchDraft = m.query
	m.searchCursor = len([]rune(m.searchDraft))
	m.searchOffset = 0
	m.keepSearchCursorVisible()
}

func (m model) inboxSelectionKey() string {
	switch m.activeSection {
	case 0:
		return "pr:" + m.selectedKey()
	case 1:
		return "issue:" + m.selectedIssueKey()
	case 2:
		return "notification:" + m.selectedNotificationID()
	default:
		return ""
	}
}

func (m model) hasInboxPreview() bool {
	if m.width < 80 || m.height < 12 || m.visibleItemTotal() == 0 {
		return false
	}
	switch m.activeSection {
	case 0:
		return m.apiError == ""
	case 1:
		return m.issueError == ""
	case 2:
		return m.notificationError == ""
	default:
		return false
	}
}

func (m model) workspaceHeight() int {
	headingRows := 2
	if m.activeSection == 0 {
		headingRows = 3
	}
	return max(1, m.height-3-headingRows-2)
}

func workspaceWidths(width int) (rail, preview int) {
	rail = min(38, max(26, width/3))
	return rail, max(1, width-rail-4)
}

func (m model) inboxPageSize() int {
	if m.hasInboxPreview() {
		return max(1, (m.workspaceHeight()-2)/2)
	}
	return m.visibleListCount()
}

func (m model) inboxPreviewHeight() int {
	return max(1, m.workspaceHeight()-3) // heading, breathing room, position cue
}

func (m model) inboxPreviewLines(width int) []string {
	switch m.activeSection {
	case 0:
		if m.selectedPR >= 0 && m.selectedPR < len(m.pullRequests) {
			return m.pullRequestPreview(m.pullRequests[m.selectedPR], width)
		}
	case 1:
		if m.selectedIssue >= 0 && m.selectedIssue < len(m.issues) {
			return issuePreview(m.issues[m.selectedIssue], width)
		}
	case 2:
		if m.selectedNotification >= 0 && m.selectedNotification < len(m.notifications) {
			return notificationPreview(m.notifications[m.selectedNotification], width)
		}
	}
	return nil
}

func (m model) maxInboxPreviewScroll() int {
	_, width := workspaceWidths(max(1, m.width-4))
	return max(0, len(m.inboxPreviewLines(width))-m.inboxPreviewHeight())
}

func (m *model) scrollInboxPreview(key string) bool {
	switch key {
	case "up", "k":
		m.inboxPreviewScroll = max(0, m.inboxPreviewScroll-1)
	case "down", "j":
		m.inboxPreviewScroll = min(m.maxInboxPreviewScroll(), m.inboxPreviewScroll+1)
	case "pgup", "ctrl+u":
		m.inboxPreviewScroll = max(0, m.inboxPreviewScroll-m.inboxPreviewHeight())
	case "pgdown", "ctrl+d":
		m.inboxPreviewScroll = min(m.maxInboxPreviewScroll(), m.inboxPreviewScroll+m.inboxPreviewHeight())
	case "home":
		m.inboxPreviewScroll = 0
	case "end":
		m.inboxPreviewScroll = m.maxInboxPreviewScroll()
	case "esc":
		m.inboxPreviewFocus = false
	default:
		return false
	}
	return true
}

func scrollPosition(start, visible, total int) string {
	if total <= visible {
		return "All visible"
	}
	return fmt.Sprintf("%d–%d / %d", start+1, min(start+visible, total), total)
}
