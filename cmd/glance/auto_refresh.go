package main

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const autoRefreshInterval = 5 * time.Minute

type autoRefreshMsg struct{}

func scheduleAutoRefresh() tea.Cmd {
	return tea.Tick(autoRefreshInterval, func(time.Time) tea.Msg { return autoRefreshMsg{} })
}
