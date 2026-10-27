package main

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const branchUpdateCheckDelay = 2 * time.Second

func isBehindBase(state string) bool {
	return strings.EqualFold(strings.TrimSpace(state), "behind")
}

func (m model) canUpdateBranch() bool {
	return m.detailOpen && m.prDetails.State == "open" && !m.prDetails.Merged &&
		isBehindBase(m.prDetails.MergeableState) && m.prDetails.Head.SHA != "" &&
		m.detailError == "" && !m.detailLoading && !m.diffLoading &&
		!m.branchUpdateInProgress && !m.branchUpdateChecking
}

func branchUpdateStatus(state string) string {
	switch state {
	case "dirty":
		return "Branch is up to date, but has merge conflicts."
	case "blocked":
		return "Branch is up to date; repository rules block merging."
	case "unstable":
		return "Branch is up to date, but required checks are failing."
	case "draft":
		return "Branch is up to date; this pull request is a draft."
	case "has_hooks":
		return "Branch is up to date; GitHub is waiting for checks."
	default:
		return "Branch is up to date with its base."
	}
}

func branchUpdatePrompt(width int) string {
	if width < 50 {
		return "Behind base · press u"
	}
	return "Branch is behind its base · press u to update"
}

func scheduleBranchUpdateCheck(requestID int, key string) tea.Cmd {
	return tea.Tick(branchUpdateCheckDelay, func(time.Time) tea.Msg {
		return branchUpdateCheckMsg{requestID: requestID, key: key}
	})
}
