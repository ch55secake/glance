package main

import (
	"context"
	"time"

	"glance/internal/githubapi"

	tea "github.com/charmbracelet/bubbletea"
)

const previewCacheLimit = 12

type previewReadyMsg struct {
	key   string
	epoch int
}

type previewLoadedMsg struct {
	key     string
	epoch   int
	details githubapi.PullRequestDetails
	err     error
}

func (m model) schedulePRPreview() tea.Cmd {
	if m.loading || m.prLoading || m.apiError != "" || m.width < 80 || m.height < 20 || m.activeSection != 0 || m.detailOpen || m.showHelp || m.searching {
		return nil
	}
	key := m.selectedKey()
	if key == "" || len(m.previewPending) >= 2 || m.previewPending[key] || m.previewErrors[key] != "" {
		return nil
	}
	if _, ok := m.previewCache[key]; ok {
		return nil
	}
	epoch := m.previewEpoch
	return tea.Tick(450*time.Millisecond, func(time.Time) tea.Msg {
		return previewReadyMsg{key: key, epoch: epoch}
	})
}

func (m model) loadPRPreview(key string, epoch int) tea.Cmd {
	client := m.client
	owner, repository, number, err := parsePullRequestKey(key)
	return func() tea.Msg {
		if err != nil {
			return previewLoadedMsg{key: key, epoch: epoch, err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), dashboardLoadTimeout)
		defer cancel()
		details, err := client.GetPullRequestPreview(ctx, owner, repository, number)
		return previewLoadedMsg{key: key, epoch: epoch, details: details, err: err}
	}
}

func (m *model) storePRPreview(key string, details githubapi.PullRequestDetails) {
	if details.Number == 0 {
		return
	}
	if m.previewCache == nil {
		m.previewCache = make(map[string]githubapi.PullRequestDetails)
	}
	if _, ok := m.previewCache[key]; !ok {
		if len(m.previewOrder) == previewCacheLimit {
			delete(m.previewCache, m.previewOrder[0])
			m.previewOrder = m.previewOrder[1:]
		}
		m.previewOrder = append(m.previewOrder, key)
	}
	m.previewCache[key] = details
}

func (m *model) clearPRPreviews() {
	m.previewEpoch++
	m.previewCache = nil
	m.previewErrors = nil
	m.previewPending = nil
	m.previewOrder = nil
}
