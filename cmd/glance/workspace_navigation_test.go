package main

import (
	"fmt"
	"strings"
	"testing"

	"glance/internal/githubapi"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func populatedWorkspace() model {
	m := initialModel()
	m.width, m.height, m.loading = 100, 24, false
	m.viewer.Login = "octocat"
	body := "First paragraph.\n\n" + strings.Repeat("Context worth reading before making a decision.\n", 70) + "Final paragraph."
	for i := range 20 {
		m.pullRequests = append(m.pullRequests, githubapi.PullRequest{Repository: "team/repo", Number: i + 1, Title: fmt.Sprintf("Pull request %d", i+1)})
		m.issues = append(m.issues, githubapi.Issue{Repository: "team/repo", Number: i + 1, Title: fmt.Sprintf("Issue %d", i+1), Body: body})
		m.notifications = append(m.notifications, githubapi.Notification{ID: fmt.Sprint(i), Subject: githubapi.NotificationSubject{Title: fmt.Sprintf("Notification %d", i+1)}})
		m.storePRPreview(fmt.Sprintf("team/repo#%d", i+1), githubapi.PullRequestDetails{
			PullRequest: m.pullRequests[i], Body: body,
			CheckRuns: []githubapi.CheckRun{{Name: "Build", Conclusion: "success"}},
			Reviews:   []githubapi.PullRequestReview{{User: githubapi.User{Login: "last-reviewer"}, State: "APPROVED"}},
		})
	}
	// Choose a cached item, even after the bounded preview cache has evicted older ones.
	m.selectedPR = 10
	return m
}

func applyKeys(m model, keys ...tea.KeyMsg) model {
	for _, key := range keys {
		updated, _ := m.Update(key)
		m = updated.(model)
	}
	return m
}

func TestInboxPreviewCanReachFullContentWithoutMovingSelection(t *testing.T) {
	for _, section := range []int{0, 1} {
		m := populatedWorkspace()
		m.activeSection = section
		selected := m.inboxSelectionKey()
		m = applyKeys(m, tea.KeyMsg{Type: tea.KeyTab}, tea.KeyMsg{Type: tea.KeyPgDown})
		if !m.inboxPreviewFocus || m.inboxPreviewScroll == 0 || m.inboxSelectionKey() != selected {
			t.Fatalf("section %d: paging preview changed selection or failed to scroll", section)
		}
		m = applyKeys(m, tea.KeyMsg{Type: tea.KeyEnd})
		view := m.View()
		if !strings.Contains(view, "Final paragraph.") || section == 0 && !strings.Contains(view, "last-reviewer") {
			t.Fatalf("section %d: content after 2000 characters is unreachable:\n%s", section, view)
		}
		position := m.inboxPreviewScroll
		m = applyKeys(m, key(":"), tea.KeyMsg{Type: tea.KeyEsc}, key("?"), tea.KeyMsg{Type: tea.KeyEsc})
		if !m.inboxPreviewFocus || m.inboxPreviewScroll != position || m.inboxSelectionKey() != selected {
			t.Fatal("closing a palette or help lost preview focus or position")
		}
		m = applyKeys(m, tea.KeyMsg{Type: tea.KeyEsc}, tea.KeyMsg{Type: tea.KeyDown})
		if m.inboxPreviewFocus || m.inboxPreviewScroll != 0 || m.inboxSelectionKey() == selected {
			t.Fatal("returning to the list and choosing another item did not reset the preview")
		}
	}
}

func TestInboxPagingAndDirectSectionShortcuts(t *testing.T) {
	m := populatedWorkspace()
	start := m.selectedPR
	m = applyKeys(m, tea.KeyMsg{Type: tea.KeyPgDown})
	if m.selectedPR <= start+1 || !strings.Contains(m.View(), fmt.Sprintf("SELECTED  %d / 20", m.selectedPR+1)) {
		t.Fatal("page down did not move the list and keep its selection visible")
	}
	m = applyKeys(m, key("2"), tea.KeyMsg{Type: tea.KeyEnd}, tea.KeyMsg{Type: tea.KeyTab}, tea.KeyMsg{Type: tea.KeyEnd})
	if m.activeSection != 1 || m.selectedIssue != 19 || m.inboxPreviewScroll == 0 {
		t.Fatal("direct section navigation or issue preview scrolling failed")
	}
	m = applyKeys(m, key("3"))
	if m.activeSection != 2 || m.inboxPreviewFocus || m.inboxPreviewScroll != 0 {
		t.Fatal("switching sections carried preview focus into another inbox")
	}
	m = applyKeys(m, key("1"))
	if m.activeSection != 0 || m.selectedPR <= start {
		t.Fatal("switching sections lost the pull request selection")
	}
}

func TestInboxPreviewRefreshAndResizeKeepValidPosition(t *testing.T) {
	m := populatedWorkspace()
	m = applyKeys(m, tea.KeyMsg{Type: tea.KeyTab}, tea.KeyMsg{Type: tea.KeyEnd})
	position := m.inboxPreviewScroll
	selected := m.pullRequests[m.selectedPR]
	updated, _ := m.Update(dashboardLoadedMsg{viewer: m.viewer, pullRequests: []githubapi.PullRequest{selected}})
	m = updated.(model)
	if m.selectedPR != 0 || m.inboxPreviewScroll != position || !m.inboxPreviewFocus {
		t.Fatal("reordering the same selected item lost preview position")
	}
	updated, _ = m.Update(previewLoadedMsg{key: m.selectedKey(), details: githubapi.PullRequestDetails{PullRequest: selected, Body: "Shorter context"}})
	m = updated.(model)
	if m.inboxPreviewScroll != 0 || !strings.Contains(m.View(), "Shorter context") {
		t.Fatal("shorter refreshed content left the preview scrolled into empty space")
	}
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	m = updated.(model)
	if m.inboxPreviewFocus || strings.Contains(m.View(), "SELECTED") {
		t.Fatal("narrow resize left focus on a hidden preview")
	}
	m = applyKeys(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.activeSection != 0 || m.inboxPreviewFocus {
		t.Fatal("tab changed sections or focused a nonexistent pane")
	}
}

func TestShortWorkspaceLoadsContextWhenPreviewIsFocused(t *testing.T) {
	m := populatedWorkspace()
	m.height = 16
	m.clearPRPreviews()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(model)
	if cmd == nil || !m.inboxPreviewFocus {
		t.Fatal("short preview cannot load its context when focused")
	}
	updated, cmd = m.Update(previewReadyMsg{key: m.selectedKey(), epoch: m.previewEpoch})
	if cmd == nil || !updated.(model).previewPending[m.selectedKey()] {
		t.Fatal("settled short preview did not start loading")
	}
}

func TestReviewHelpScrollsAndRestoresReviewPosition(t *testing.T) {
	m := populatedWorkspace()
	m.detailOpen, m.detailScroll, m.sidebarFocus = true, 3, true
	m = applyKeys(m, key("?"))
	if !m.showHelp || !strings.Contains(m.View(), "KEYBOARD SHORTCUTS") {
		t.Fatal("help shortcut is unavailable from PR review")
	}
	m.width, m.height = 30, 16
	m = applyKeys(m, tea.KeyMsg{Type: tea.KeyEnd})
	if !strings.Contains(m.View(), "Return to the inbox") || m.helpScroll == 0 {
		t.Fatalf("last shortcut is inaccessible in narrow help:\n%s", m.View())
	}
	m = applyKeys(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.showHelp || !m.detailOpen || m.detailScroll != 3 || !m.sidebarFocus {
		t.Fatal("help changed the review's position or focus")
	}
	m.mergeDialog = chooseMergeMethod
	m.prDetails.AllowedMergeMethods = []githubapi.MergeMethod{githubapi.MergeSquash}
	m = applyKeys(m, key("?"), key(":"), key("2"))
	if m.showHelp || m.paletteOpen || !m.detailOpen || m.mergeDialog != chooseMergeMethod {
		t.Fatal("navigation shortcuts took focus away from the merge chooser")
	}
}

func TestWorkspaceModesStayBoundedAndKeepControls(t *testing.T) {
	previous := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
	for _, profile := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		lipgloss.SetColorProfile(profile)
		for _, size := range []struct{ width, height int }{{30, 10}, {45, 12}, {79, 16}, {80, 16}, {100, 24}, {160, 42}} {
			for _, mode := range []string{"preview", "palette", "empty-palette", "help", "filter"} {
				m := populatedWorkspace()
				m.width, m.height = size.width, size.height
				switch mode {
				case "preview":
					m = applyKeys(m, tea.KeyMsg{Type: tea.KeyTab}, tea.KeyMsg{Type: tea.KeyEnd})
				case "palette", "empty-palette":
					m = applyKeys(m, key(":"), tea.KeyMsg{Type: tea.KeyEnd})
					if mode == "empty-palette" {
						m = applyKeys(m, key(strings.Repeat("変更", 60)))
					}
				case "help":
					m = applyKeys(m, key("?"), tea.KeyMsg{Type: tea.KeyEnd})
				case "filter":
					m = applyKeys(m, key("/"), key(strings.Repeat("変更", 60)))
				}
				view := m.View()
				lines := strings.Split(view, "\n")
				if len(lines) != m.height || lines[len(lines)-1] != m.footer(m.width-4) {
					t.Fatalf("%v %s lost pinned footer:\n%s", size, mode, view)
				}
				for _, line := range lines {
					if lipgloss.Width(line) > m.width {
						t.Fatalf("%v %s overflows: %q", size, mode, line)
					}
				}
				plain := ansi.Strip(view)
				if profile == termenv.Ascii && strings.Contains(view, "\x1b[") {
					t.Fatal("monochrome view contains color escapes")
				}
				if mode == "palette" && !strings.Contains(plain, "▸ Quit") {
					t.Fatalf("%v lost selected command:\n%s", size, plain)
				}
				if (mode == "empty-palette" || mode == "filter") && !strings.Contains(plain, "▌") {
					t.Fatalf("%v %s hid the input cursor:\n%s", size, mode, plain)
				}
				if strings.Contains(ansi.Strip(lines[len(lines)-1]), "…") {
					t.Fatal("footer contains an incomplete shortcut")
				}
			}
		}
	}
}
