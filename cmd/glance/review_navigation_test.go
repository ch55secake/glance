package main

import (
	"strings"
	"testing"

	"glance/internal/githubapi"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func reviewNavigationModel() model {
	m := initialModel()
	m.loading, m.detailOpen, m.diffLoaded = false, true, true
	m.width, m.height = 120, 28
	m.detailKey = "team/glance#42"
	m.prDetails = githubapi.PullRequestDetails{
		PullRequest: githubapi.PullRequest{Number: 42, Repository: "team/glance", Title: "Make code review feel effortless", State: "open", User: githubapi.User{Login: "octocat"}},
		Body:        "## A better review flow\n\nJump to the file you need and keep track of what you have read.",
		Mergeable:   new(true), CheckRuns: []githubapi.CheckRun{{Name: "build / test", Conclusion: "success"}},
	}
	m.diffFiles = []githubapi.PullRequestFile{
		{Filename: "cmd/glance/model.go", Status: "modified", Additions: 31, Deletions: 4, Patch: "@@ -1 +1 @@\n" + strings.Repeat("+func update() {}\n", 40)},
		{Filename: "cmd/glance/変更_test.go", PreviousFilename: "cmd/old_test.go", Status: "renamed", Additions: 12, Patch: "@@ -0,0 +1 @@\n+func TestReview() {}"},
		{Filename: "assets/screenshot.png", Status: "added"},
	}
	m.cacheDetailLines()
	return m
}

func TestFilePickerFindsRenamesAndJumpsIntoContinuousDiff(t *testing.T) {
	m := reviewNavigationModel()
	m.detailScroll = 3
	m.sidebarFocus = true
	m = applyKeys(m, key("f"), key("old_test"))
	if !m.filePickerOpen || !strings.Contains(m.View(), "変更_test.go") {
		t.Fatal("picker did not search a renamed file's previous path")
	}
	m = applyKeys(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.filePickerOpen || m.sidebarFocus || m.currentReviewFile() != 1 || !strings.Contains(m.View(), "TestReview") {
		t.Fatalf("picker failed to jump into the selected file:\n%s", m.View())
	}
	m = applyKeys(m, key("]"))
	if m.currentReviewFile() != 2 || !strings.Contains(m.View(), "GitHub did not provide a patch") {
		t.Fatalf("last, patchless file cannot be reached:\n%s", m.View())
	}
	m = applyKeys(m, key("["), key("["))
	if m.currentReviewFile() != 0 {
		t.Fatal("previous-file navigation skipped a file")
	}
	m = applyKeys(m, tea.KeyMsg{Type: tea.KeyDown})
	if !strings.Contains(m.reviewLocation(116), "cmd/glance/model.go") {
		t.Fatal("file context disappeared when its heading scrolled away")
	}
	position := m.detailScroll
	m = applyKeys(m, key("f"), key("not-a-file"), tea.KeyMsg{Type: tea.KeyEnter})
	if !m.filePickerOpen || m.detailScroll != position || !strings.Contains(m.View(), "No files match") {
		t.Fatal("an empty search changed the review position")
	}
	m = applyKeys(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.detailScroll != position || m.filePickerOpen {
		t.Fatal("canceling file search lost the review position")
	}
}

func TestFileNavigationKeepsAnchorsThroughResizeAndLateDetails(t *testing.T) {
	m := reviewNavigationModel()
	m = applyKeys(m, key("d"), key("]"))
	for _, width := range []int{80, 30, 160} {
		updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
		m = updated.(model)
		if m.currentReviewFile() != 1 || !strings.Contains(m.View(), "変更_test.go") {
			t.Fatalf("resize to %d moved away from the chosen file:\n%s", width, m.View())
		}
	}
	details := m.prDetails
	details.Body = strings.Repeat("More description arrived.\n\n", 40)
	updated, _ := m.Update(pullRequestDetailsLoadedMsg{key: m.detailKey, details: details})
	m = updated.(model)
	if m.currentReviewFile() != 1 || !strings.Contains(m.View(), "TestReview") {
		t.Fatal("late description shifted the review to a different file")
	}
	// A subsequent data refresh should keep navigation tied to the new rendered rows.
	files := append([]githubapi.PullRequestFile(nil), m.diffFiles...)
	files[0].Patch += strings.Repeat("+extra context\n", 40)
	updated, _ = m.Update(pullRequestFilesLoadedMsg{key: m.detailKey, files: files})
	m = applyKeys(updated.(model), key("d"), key("]"))
	if m.currentReviewFile() != 1 || !strings.Contains(m.View(), "TestReview") {
		t.Fatal("refreshed patch length invalidated file navigation")
	}
}

func TestViewedFilesAreLocalVersionAwareAndDoNotRerenderDiff(t *testing.T) {
	m := reviewNavigationModel()
	cached := &m.detailLines[0]
	m = applyKeys(m, key("d"), key("v"), key("]"), key("v"))
	if m.reviewedFileCount() != 2 || &m.detailLines[0] != cached || !strings.Contains(m.reviewLocation(116), "2/3 viewed") {
		t.Fatal("viewed progress was lost or re-rendered the diff")
	}
	files := append([]githubapi.PullRequestFile(nil), m.diffFiles...)
	files[1].Patch += "\n+new change"
	updated, _ := m.Update(pullRequestFilesLoadedMsg{key: m.detailKey, files: files})
	m = updated.(model)
	if m.reviewedFileCount() != 1 || !m.fileReviewed(0) || m.fileReviewed(1) {
		t.Fatal("refresh retained viewed state for changed content or lost unchanged content")
	}
	if _, stale := m.reviewedFiles[files[1].Filename]; stale {
		t.Fatal("changed file retained a stale viewed snapshot")
	}
	m.pullRequests = []githubapi.PullRequest{{Repository: "team/other", Number: 12}}
	m.openSelectedPullRequest()
	if len(m.reviewedFiles) != 0 || m.filePickerOpen {
		t.Fatal("opening a different PR retained local review progress")
	}
}

func TestFilePickerRespectsMergeFocusAndAsyncLoading(t *testing.T) {
	m := reviewNavigationModel()
	m.mergeDialog = chooseMergeMethod
	m.prDetails.AllowedMergeMethods = []githubapi.MergeMethod{githubapi.MergeCommit}
	m = applyKeys(m, key("f"), key("]"), key("v"))
	if m.filePickerOpen || m.detailScroll != 0 || m.reviewedFileCount() != 0 {
		t.Fatal("file shortcuts took focus from the merge dialog")
	}
	m.mergeDialog = noMergeDialog
	m.diffFiles, m.detailLines = nil, nil
	m.diffLoading, m.diffLoaded = true, false
	m = applyKeys(m, key("f"))
	if !strings.Contains(m.View(), "Loading changed files") {
		t.Fatal("picker did not explain that files are loading")
	}
	updated, _ := m.Update(pullRequestFilesLoadedMsg{key: m.detailKey, files: reviewNavigationModel().diffFiles})
	m = applyKeys(updated.(model), tea.KeyMsg{Type: tea.KeyEnd}, tea.KeyMsg{Type: tea.KeyEnter})
	if m.currentReviewFile() != 2 || m.filePickerOpen {
		t.Fatal("files arriving while the picker is open could not be selected")
	}
}

func TestFilePickerAndJumpLayoutsAcrossTerminalSizes(t *testing.T) {
	previous := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
	for _, profile := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		lipgloss.SetColorProfile(profile)
		for _, size := range []struct{ width, height int }{{30, 10}, {30, 16}, {80, 24}, {120, 28}, {160, 40}} {
			m := reviewNavigationModel()
			updated, _ := m.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})
			m = applyKeys(updated.(model), key("f"), tea.KeyMsg{Type: tea.KeyEnd})
			for _, picker := range []bool{true, false} {
				if !picker {
					m = applyKeys(m, tea.KeyMsg{Type: tea.KeyEnter})
				}
				view := m.View()
				lines := strings.Split(view, "\n")
				if len(lines) != m.height || !strings.Contains(ansi.Strip(view), "screenshot.png") {
					t.Fatalf("%v picker=%v lost final file or fixed height:\n%s", size, picker, view)
				}
				for _, line := range lines {
					if lipgloss.Width(line) > m.width {
						t.Fatalf("%v overflows: %q", size, line)
					}
				}
				if profile == termenv.Ascii && strings.Contains(view, "\x1b[") {
					t.Fatal("monochrome file navigation contains ANSI colors")
				}
			}
		}
	}
}
