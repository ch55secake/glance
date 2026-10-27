package githubapi

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestViewerUsesGhAPI(t *testing.T) {
	runner := &stubRunner{stdout: []byte(`{"login":"octocat","name":"The Octocat"}`)}
	user, err := NewClientWithRunner(runner).Viewer(context.Background())
	if err != nil {
		t.Fatalf("Viewer() error = %v", err)
	}
	if user.Login != "octocat" || user.Name != "The Octocat" {
		t.Fatalf("Viewer() = %+v, want octocat user", user)
	}
	if want := [][]string{{"api", "user"}}; !reflect.DeepEqual(runner.calls, want) {
		t.Errorf("gh calls = %#v, want %#v", runner.calls, want)
	}
}

func TestListPullRequestsUsesGhPagination(t *testing.T) {
	runner := &stubRunner{stdout: []byte(`[[{"number":1,"title":"first"}],[{"number":2,"title":"second"}]]`)}
	pullRequests, err := NewClientWithRunner(runner).ListPullRequests(context.Background(), "octo", "repo")
	if err != nil {
		t.Fatalf("ListPullRequests() error = %v", err)
	}
	if len(pullRequests) != 2 || pullRequests[0].Number != 1 || pullRequests[1].Number != 2 {
		t.Fatalf("ListPullRequests() = %+v, want two pull requests in page order", pullRequests)
	}
	want := [][]string{{
		"api", "--paginate", "--slurp", "-X", "GET",
		"-F", "state=open", "-F", "per_page=100", "repos/octo/repo/pulls",
	}}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Errorf("gh calls = %#v, want %#v", runner.calls, want)
	}
}

func TestSearchPullRequestsUsesGitHubQueryAndRecoversRepository(t *testing.T) {
	runner := &stubRunner{stdout: []byte(`[{"items":[{"number":42,"title":"My PR","state":"open","draft":true,"repository_url":"https://api.github.com/repos/octo/project","pull_request":{"url":"https://api.github.com/repos/octo/project/pulls/42"}},{"number":4,"title":"Not a PR","repository_url":"https://api.github.com/repos/octo/project"}]},{"items":[{"number":7,"title":"Another PR","repository_url":"https://api.github.com/repos/team/docs","pull_request":{"url":"https://api.github.com/repos/team/docs/pulls/7"}}]}]`)}
	results, err := NewClientWithRunner(runner).SearchPullRequests(context.Background(), "is:open author:@me")
	if err != nil || len(results) != 2 || !results[0].Draft || results[0].Repository != "octo/project" || results[1].Repository != "team/docs" {
		t.Fatalf("SearchPullRequests() = %+v, %v", results, err)
	}
	want := [][]string{{"api", "--paginate", "--slurp", "-X", "GET", "-F", "q=is:pr is:open author:@me", "-F", "sort=updated", "-F", "order=desc", "-F", "per_page=100", "search/issues"}}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Errorf("gh calls = %#v, want %#v", runner.calls, want)
	}
}

func TestListPullRequestsRejectsInvalidPathSegments(t *testing.T) {
	runner := &stubRunner{}
	_, err := NewClientWithRunner(runner).ListPullRequests(context.Background(), "octo/other", "repo")
	if err == nil {
		t.Fatal("ListPullRequests() error = nil, want validation error")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("invalid repository triggered gh calls: %#v", runner.calls)
	}
}

func TestListPullRequestFilesUsesPagination(t *testing.T) {
	runner := &stubRunner{stdout: []byte(`[[{"filename":"README.md","status":"modified","additions":2,"deletions":1,"changes":3,"patch":"@@ -1 +1 @@\n-old\n+new"}],[{"filename":"cmd/app.go","previous_filename":"cmd/main.go","status":"renamed","patch":"@@ -1 +1 @@\n-old\n+new"}]]`)}
	files, err := NewClientWithRunner(runner).ListPullRequestFiles(context.Background(), "octo", "project", 42)
	if err != nil {
		t.Fatalf("ListPullRequestFiles() error = %v", err)
	}
	if len(files) != 2 || files[0].Filename != "README.md" || !strings.Contains(files[0].Patch, "+new") || files[1].PreviousFilename != "cmd/main.go" {
		t.Fatalf("ListPullRequestFiles() = %+v", files)
	}
	want := [][]string{{
		"api", "--paginate", "--slurp", "-X", "GET", "-F", "per_page=100",
		"repos/octo/project/pulls/42/files",
	}}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Errorf("gh calls = %#v, want %#v", runner.calls, want)
	}
}

func TestListPullRequestFilesRejectsInvalidArguments(t *testing.T) {
	runner := &stubRunner{}
	if _, err := NewClientWithRunner(runner).ListPullRequestFiles(context.Background(), "octo/project", "repo", 42); err == nil {
		t.Fatal("ListPullRequestFiles() error = nil, want validation error")
	}
	if _, err := NewClientWithRunner(runner).ListPullRequestFiles(context.Background(), "octo", "repo", 0); err == nil {
		t.Fatal("ListPullRequestFiles() error = nil for PR number 0")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("invalid file request invoked gh: %#v", runner.calls)
	}
}

func TestGetPullRequestDetailsLoadsReviewStatusChecksAndMergeMethods(t *testing.T) {
	runner := &routingRunner{responses: map[string][]byte{
		"repos/octo/project/pulls/42":                  []byte(`{"number":42,"title":"Useful change","state":"open","body":"Description","user":{"login":"octocat"},"head":{"ref":"feature","sha":"abc123"},"base":{"ref":"main","sha":"def456"},"requested_reviewers":[{"login":"hubot"}],"mergeable":true,"mergeable_state":"clean"}`),
		"repos/octo/project/pulls/42/reviews":          []byte(`[[{"user":{"login":"reviewer"},"state":"APPROVED","submitted_at":"2026-10-21T12:00:00Z"}]]`),
		"repos/octo/project/commits/abc123/check-runs": []byte(`[{"total_count":1,"check_runs":[{"name":"CI","status":"completed","conclusion":"success"}]}]`),
		"repos/octo/project":                           []byte(`{"allow_merge_commit":true,"allow_squash_merge":false,"allow_rebase_merge":true}`),
	}}
	details, err := NewClientWithRunner(runner).GetPullRequestDetails(context.Background(), "octo", "project", 42)
	if err != nil {
		t.Fatalf("GetPullRequestDetails() error = %v", err)
	}
	if details.Number != 42 || details.Repository != "octo/project" || details.Body != "Description" || details.Head.SHA != "abc123" {
		t.Fatalf("pull request details = %+v", details)
	}
	if len(details.RequestedReviewers) != 1 || len(details.Reviews) != 1 || details.Reviews[0].State != "APPROVED" {
		t.Errorf("review metadata = %+v", details)
	}
	if len(details.CheckRuns) != 1 || details.CheckRuns[0].Conclusion != "success" {
		t.Errorf("check runs = %+v", details.CheckRuns)
	}
	if !reflect.DeepEqual(details.AllowedMergeMethods, []MergeMethod{MergeCommit, MergeRebase}) {
		t.Errorf("allowed merge methods = %#v", details.AllowedMergeMethods)
	}
	if details.ReviewsError != "" || details.ChecksError != "" || details.MergeMethodsError != "" {
		t.Errorf("optional detail errors = reviews:%q checks:%q methods:%q", details.ReviewsError, details.ChecksError, details.MergeMethodsError)
	}
}

func TestGetPullRequestPreviewOmitsMergeSettingsRequest(t *testing.T) {
	runner := &routingRunner{responses: map[string][]byte{
		"repos/octo/project/pulls/42":                  []byte(`{"number":42,"body":"Review context","head":{"sha":"abc123"}}`),
		"repos/octo/project/pulls/42/reviews":          []byte(`[[{"state":"APPROVED"}]]`),
		"repos/octo/project/commits/abc123/check-runs": []byte(`[{"check_runs":[{"name":"CI","conclusion":"success"}]}]`),
	}}
	details, err := NewClientWithRunner(runner).GetPullRequestPreview(context.Background(), "octo", "project", 42)
	if err != nil || details.Number != 42 || details.Body != "Review context" || len(details.CheckRuns) != 1 || len(details.Reviews) != 1 {
		t.Fatalf("preview details=%+v error=%v", details, err)
	}
	for _, call := range runner.calls {
		if call[len(call)-1] == "repos/octo/project" {
			t.Fatalf("preview unexpectedly fetched merge settings: %#v", runner.calls)
		}
	}
}

func TestGetPullRequestDetailsKeepsMainDetailsWhenOptionalRequestsFail(t *testing.T) {
	runner := &routingRunner{
		responses: map[string][]byte{
			"repos/octo/project/pulls/42": []byte(`{"number":42,"state":"open","head":{"sha":"abc123"}}`),
		},
		stderrs: map[string][]byte{
			"repos/octo/project/pulls/42/reviews":          []byte("HTTP 403: Resource not accessible"),
			"repos/octo/project/commits/abc123/check-runs": []byte("HTTP 403: Resource not accessible"),
			"repos/octo/project":                           []byte("HTTP 403: Resource not accessible"),
		},
		errors: map[string]error{
			"repos/octo/project/pulls/42/reviews":          errors.New("exit status 1"),
			"repos/octo/project/commits/abc123/check-runs": errors.New("exit status 1"),
			"repos/octo/project":                           errors.New("exit status 1"),
		},
	}
	details, err := NewClientWithRunner(runner).GetPullRequestDetails(context.Background(), "octo", "project", 42)
	if err != nil || details.Number != 42 {
		t.Fatalf("GetPullRequestDetails() details=%+v error=%v; expected main details despite optional errors", details, err)
	}
	if details.ReviewsError == "" || details.ChecksError == "" || details.MergeMethodsError == "" {
		t.Errorf("optional failures were not retained: %+v", details)
	}
}

func TestGetPullRequestDetailsRejectsInvalidArguments(t *testing.T) {
	runner := &stubRunner{}
	for _, tc := range []struct {
		owner, repository string
		number            int
	}{{"octo/team", "project", 42}, {"octo", "project", 0}} {
		if _, err := NewClientWithRunner(runner).GetPullRequestDetails(context.Background(), tc.owner, tc.repository, tc.number); err == nil {
			t.Errorf("GetPullRequestDetails(%q, %q, %d) error = nil", tc.owner, tc.repository, tc.number)
		}
	}
	if len(runner.calls) != 0 {
		t.Fatalf("invalid arguments invoked gh: %#v", runner.calls)
	}
}

func TestMergePullRequestUsesExpectedHeadAndMethod(t *testing.T) {
	runner := &stubRunner{stdout: []byte(`{"sha":"merged123","merged":true,"message":"Pull Request successfully merged"}`)}
	result, err := NewClientWithRunner(runner).MergePullRequest(context.Background(), "octo", "project", 42, MergeSquash, "head123")
	if err != nil {
		t.Fatalf("MergePullRequest() error = %v", err)
	}
	if !result.Merged || result.SHA != "merged123" {
		t.Fatalf("MergePullRequest() = %+v", result)
	}
	want := [][]string{{"api", "-X", "PUT", "-F", "sha=head123", "-F", "merge_method=squash", "repos/octo/project/pulls/42/merge"}}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Errorf("gh calls = %#v, want %#v", runner.calls, want)
	}
}

func TestCreatePullRequestCommentPreservesMultilineBody(t *testing.T) {
	body := "Thanks for the update.\n\nCould you add a regression test?"
	runner := &stubRunner{stdout: []byte(`{"id":43,"body":"Thanks for the update.\n\nCould you add a regression test?","html_url":"https://github.com/octo/project/pull/42#issuecomment-43"}`)}
	comment, err := NewClientWithRunner(runner).CreatePullRequestComment(context.Background(), "octo", "project", 42, body)
	if err != nil {
		t.Fatalf("CreatePullRequestComment() error = %v", err)
	}
	if comment.ID != 43 || comment.Body != body || comment.HTMLURL == "" {
		t.Fatalf("CreatePullRequestComment() = %+v", comment)
	}
	want := [][]string{{"api", "-X", "POST", "-f", "body=" + body, "repos/octo/project/issues/42/comments"}}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Errorf("gh calls = %#v, want %#v", runner.calls, want)
	}
}

func TestCreatePullRequestCommentRejectsInvalidReferenceOrBlankBody(t *testing.T) {
	runner := &stubRunner{}
	client := NewClientWithRunner(runner)
	for _, tc := range []struct {
		owner, repository string
		number            int
		body              string
	}{
		{"owner/repo", "repo", 42, "comment"},
		{"owner", "repo", 0, "comment"},
		{"owner", "repo", 42, " \n\t "},
	} {
		if _, err := client.CreatePullRequestComment(context.Background(), tc.owner, tc.repository, tc.number, tc.body); err == nil {
			t.Errorf("CreatePullRequestComment(%q, %q, %d, %q) error = nil", tc.owner, tc.repository, tc.number, tc.body)
		}
	}
	if len(runner.calls) != 0 {
		t.Fatalf("invalid comment request invoked gh: %#v", runner.calls)
	}
}

func TestCreatePullRequestReviewCommentTargetsDiffLine(t *testing.T) {
	body := "Could this handle an empty value?"
	runner := &stubRunner{stdout: []byte(`{"id":47,"body":"Could this handle an empty value?","path":"cmd/app.go","line":18,"side":"RIGHT","commit_id":"head123","html_url":"https://github.com/octo/project/pull/42#discussion_r47"}`)}
	comment, err := NewClientWithRunner(runner).CreatePullRequestReviewComment(context.Background(), "octo", "project", 42, "head123", "cmd/app.go", 18, "RIGHT", body)
	if err != nil {
		t.Fatalf("CreatePullRequestReviewComment() error = %v", err)
	}
	if comment.ID != 47 || comment.Path != "cmd/app.go" || comment.Line != 18 || comment.Side != "RIGHT" || comment.CommitID != "head123" {
		t.Fatalf("CreatePullRequestReviewComment() = %+v", comment)
	}
	want := [][]string{{
		"api", "-X", "POST", "-f", "body=" + body, "-f", "commit_id=head123", "-f", "path=cmd/app.go",
		"-F", "line=18", "-f", "side=RIGHT", "repos/octo/project/pulls/42/comments",
	}}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Errorf("gh calls = %#v, want %#v", runner.calls, want)
	}
}

func TestCreatePullRequestReviewCommentRejectsInvalidTarget(t *testing.T) {
	runner := &stubRunner{}
	client := NewClientWithRunner(runner)
	for _, tc := range []struct {
		owner, repository     string
		number, line          int
		sha, path, side, body string
	}{
		{"octo/team", "project", 42, 18, "head", "app.go", "RIGHT", "comment"},
		{"octo", "project", 42, 0, "head", "app.go", "RIGHT", "comment"},
		{"octo", "project", 42, 18, "", "app.go", "RIGHT", "comment"},
		{"octo", "project", 42, 18, "head", "app.go", "CENTER", "comment"},
		{"octo", "project", 42, 18, "head", "app.go", "LEFT", "  "},
	} {
		if _, err := client.CreatePullRequestReviewComment(context.Background(), tc.owner, tc.repository, tc.number, tc.sha, tc.path, tc.line, tc.side, tc.body); err == nil {
			t.Errorf("CreatePullRequestReviewComment(%+v) error = nil", tc)
		}
	}
	if len(runner.calls) != 0 {
		t.Fatalf("invalid diff comment invoked gh: %#v", runner.calls)
	}
}

func TestMergePullRequestRequiresValidMethodAndHeadSHA(t *testing.T) {
	runner := &stubRunner{}
	client := NewClientWithRunner(runner)
	for _, tc := range []struct {
		method MergeMethod
		sha    string
	}{{MergeMethod("invalid"), "head"}, {MergeSquash, ""}} {
		if _, err := client.MergePullRequest(context.Background(), "octo", "project", 42, tc.method, tc.sha); err == nil {
			t.Errorf("MergePullRequest(%q, %q) error = nil", tc.method, tc.sha)
		}
	}
	if len(runner.calls) != 0 {
		t.Fatalf("invalid merge request invoked gh: %#v", runner.calls)
	}
}

func TestMergePullRequestExplainsGitHubFailures(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
		want   string
	}{
		{name: "stale head", stderr: "HTTP 409: Head branch was modified. Review and try the merge again.", want: "changed since it was loaded"},
		{name: "branch protection", stderr: "HTTP 405: Protected branch update failed; required status checks are pending", want: "branch protection is blocking"},
		{name: "conflict", stderr: "HTTP 405: Pull Request is not mergeable", want: "resolve conflicts"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runner := &stubRunner{stderr: []byte(tc.stderr), err: errors.New("exit status 1")}
			_, err := NewClientWithRunner(runner).MergePullRequest(context.Background(), "octo", "project", 42, MergeCommit, "head123")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("MergePullRequest() error = %v, want %q", err, tc.want)
			}
		})
	}
	refused := &stubRunner{stdout: []byte(`{"merged":false,"message":"Pull Request is not mergeable"}`)}
	if _, err := NewClientWithRunner(refused).MergePullRequest(context.Background(), "octo", "project", 42, MergeCommit, "head123"); err == nil || !strings.Contains(err.Error(), "not mergeable") {
		t.Fatalf("MergePullRequest() refusal error = %v, want GitHub response", err)
	}
}

func TestListRepositoriesUsesPaginationAndAllAffiliations(t *testing.T) {
	runner := &stubRunner{stdout: []byte(`[[{"name":"one","full_name":"octo/one","owner":{"login":"octo"}}],[{"name":"two","full_name":"octo/two","owner":{"login":"octo"}}]]`)}
	repositories, err := NewClientWithRunner(runner).ListRepositories(context.Background())
	if err != nil {
		t.Fatalf("ListRepositories() error = %v", err)
	}
	if len(repositories) != 2 || repositories[0].FullName != "octo/one" || repositories[1].Owner.Login != "octo" {
		t.Fatalf("ListRepositories() = %+v, want both paginated repositories", repositories)
	}
	want := [][]string{{
		"api", "--paginate", "--slurp", "-X", "GET",
		"-F", "affiliation=owner,collaborator,organization_member", "-F", "per_page=100", "user/repos",
	}}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Errorf("gh calls = %#v, want %#v", runner.calls, want)
	}
}

func TestListOpenPullRequestsAcrossRepositories(t *testing.T) {
	runner := &routingRunner{responses: map[string][]byte{
		"user/repos":           []byte(`[[{"name":"one","full_name":"octo/one","owner":{"login":"octo"}}],[{"name":"two","full_name":"org/two","owner":{"login":"org"}}]]`),
		"repos/octo/one/pulls": []byte(`[[{"number":4,"title":"older","updated_at":"2026-10-20T12:00:00Z","user":{"login":"octocat"}}]]`),
		"repos/org/two/pulls":  []byte(`[[{"number":8,"title":"newer","updated_at":"2026-10-21T12:00:00Z","user":{"login":"hubot"}}]]`),
	}}
	pullRequests, err := NewClientWithRunner(runner).ListOpenPullRequests(context.Background())
	if err != nil {
		t.Fatalf("ListOpenPullRequests() error = %v", err)
	}
	if len(pullRequests) != 2 {
		t.Fatalf("ListOpenPullRequests() returned %d items, want 2: %+v", len(pullRequests), pullRequests)
	}
	if pullRequests[0].Repository != "org/two" || pullRequests[0].Number != 8 || pullRequests[1].Repository != "octo/one" {
		t.Errorf("pull requests = %+v, want all repositories sorted by most recently updated", pullRequests)
	}

	runner.mu.Lock()
	defer runner.mu.Unlock()
	if len(runner.calls) != 3 {
		t.Fatalf("gh made %d calls, want repository listing plus two PR requests: %#v", len(runner.calls), runner.calls)
	}
	for _, call := range runner.calls[1:] {
		if len(call) < 2 || call[0] != "api" || !contains(call, "--paginate") {
			t.Errorf("pull request call = %#v, want paginated gh api request", call)
		}
	}
}

func TestListOpenPullRequestsWithNoRepositories(t *testing.T) {
	runner := &stubRunner{stdout: []byte(`[]`)}
	pullRequests, err := NewClientWithRunner(runner).ListOpenPullRequests(context.Background())
	if err != nil {
		t.Fatalf("ListOpenPullRequests() error = %v", err)
	}
	if pullRequests == nil || len(pullRequests) != 0 {
		t.Fatalf("ListOpenPullRequests() = %#v, want an empty list", pullRequests)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("gh calls = %#v, want only the repositories request", runner.calls)
	}
}

func TestListOpenPullRequestsReportsRepositoryError(t *testing.T) {
	runner := &routingRunner{
		responses: map[string][]byte{
			"user/repos": []byte(`[[{"name":"private","full_name":"octo/private","owner":{"login":"octo"}}]]`),
		},
		stderrs: map[string][]byte{"repos/octo/private/pulls": []byte("HTTP 403: Resource not accessible")},
		errors:  map[string]error{"repos/octo/private/pulls": errors.New("exit status 1")},
	}
	_, err := NewClientWithRunner(runner).ListOpenPullRequests(context.Background())
	if err == nil || !strings.Contains(err.Error(), "octo/private") {
		t.Fatalf("ListOpenPullRequests() error = %v, want repository context", err)
	}
}

func TestListOpenIssuesAcrossRepositoriesFiltersPullRequests(t *testing.T) {
	runner := &routingRunner{responses: map[string][]byte{
		"user/repos": []byte(`[[` +
			`{"name":"one","full_name":"octo/one","owner":{"login":"octo"},"has_issues":true},` +
			`{"name":"disabled","full_name":"octo/disabled","owner":{"login":"octo"},"has_issues":false}],` +
			`[{"name":"two","full_name":"org/two","owner":{"login":"org"},"has_issues":true}]]`),
		"repos/octo/one/issues": []byte(`[[` +
			`{"number":3,"title":"Older issue","state":"open","updated_at":"2026-10-20T12:00:00Z","user":{"login":"octocat"},"labels":[{"name":"bug","color":"ff0000"}],"comments":2},` +
			`{"number":4,"title":"A pull request","state":"open","pull_request":{"url":"https://api.github.com/repos/octo/one/pulls/4"}}]]`),
		"repos/org/two/issues": []byte(`[[{"number":9,"title":"Newer issue","state":"open","updated_at":"2026-10-21T12:00:00Z","user":{"login":"hubot"}}]]`),
	}}
	issues, err := NewClientWithRunner(runner).ListOpenIssues(context.Background())
	if err != nil {
		t.Fatalf("ListOpenIssues() error = %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("ListOpenIssues() returned %d items, want actual issues only: %+v", len(issues), issues)
	}
	if issues[0].Repository != "org/two" || issues[0].Number != 9 || issues[1].Repository != "octo/one" {
		t.Errorf("issues = %+v, want repository names and newest-first ordering", issues)
	}
	if issues[1].Comments != 2 || len(issues[1].Labels) != 1 || issues[1].Labels[0].Name != "bug" {
		t.Errorf("issue metadata was not decoded: %+v", issues[1])
	}

	runner.mu.Lock()
	defer runner.mu.Unlock()
	if len(runner.calls) != 3 {
		t.Fatalf("gh made %d calls, want repository listing and two issue requests", len(runner.calls))
	}
	for _, call := range runner.calls[1:] {
		if call[len(call)-1] == "repos/octo/disabled/issues" {
			t.Fatal("requested issues for a repository with issues disabled")
		}
		if !contains(call, "state=open") || !contains(call, "--paginate") {
			t.Errorf("issue call = %#v, want open state and pagination", call)
		}
	}
}

func TestListOpenIssuesWithNoIssueEnabledRepositories(t *testing.T) {
	runner := &stubRunner{stdout: []byte(`[[{"name":"wiki","full_name":"octo/wiki","owner":{"login":"octo"},"has_issues":false}]]`)}
	issues, err := NewClientWithRunner(runner).ListOpenIssues(context.Background())
	if err != nil {
		t.Fatalf("ListOpenIssues() error = %v", err)
	}
	if issues == nil || len(issues) != 0 {
		t.Fatalf("ListOpenIssues() = %#v, want empty slice", issues)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("gh calls = %#v, want only repository listing", runner.calls)
	}
}

func TestListOpenIssuesReportsRepositoryError(t *testing.T) {
	runner := &routingRunner{
		responses: map[string][]byte{
			"user/repos": []byte(`[[{"name":"private","full_name":"octo/private","owner":{"login":"octo"},"has_issues":true}]]`),
		},
		stderrs: map[string][]byte{"repos/octo/private/issues": []byte("HTTP 403: Resource not accessible")},
		errors:  map[string]error{"repos/octo/private/issues": errors.New("exit status 1")},
	}
	_, err := NewClientWithRunner(runner).ListOpenIssues(context.Background())
	if err == nil || !strings.Contains(err.Error(), "octo/private") {
		t.Fatalf("ListOpenIssues() error = %v, want repository context", err)
	}
}

func TestListNotificationsUsesPaginationAndKeepsUnreadMetadata(t *testing.T) {
	runner := &stubRunner{stdout: []byte(`[[` +
		`{"id":"1","unread":false,"reason":"mention","updated_at":"2026-10-20T12:00:00Z","last_read_at":"2026-10-20T12:30:00Z","subject":{"title":"Old thread","url":"https://api.github.com/repos/octo/one/issues/1","type":"Issue"},"repository":{"id":1,"name":"one","full_name":"octo/one","html_url":"https://github.com/octo/one","owner":{"login":"octo"}}}],` +
		`[{"id":"2","unread":true,"reason":"review_requested","updated_at":"2026-10-21T12:00:00Z","subject":{"title":"Review this","type":"PullRequest"},"repository":{"id":2,"name":"two","full_name":"org/two","owner":{"login":"org"}}}]]`)}
	notifications, err := NewClientWithRunner(runner).ListNotifications(context.Background())
	if err != nil {
		t.Fatalf("ListNotifications() error = %v", err)
	}
	if len(notifications) != 2 || notifications[0].ID != "2" || notifications[1].Unread {
		t.Fatalf("ListNotifications() = %+v, want pages flattened and newest first", notifications)
	}
	if notifications[0].Subject.Type != "PullRequest" || notifications[0].Reason != "review_requested" || notifications[0].Repository.FullName != "org/two" {
		t.Errorf("notification metadata was not decoded: %+v", notifications[0])
	}
	if notifications[1].LastReadAt == nil || notifications[1].Subject.Title != "Old thread" {
		t.Errorf("read notification metadata was not decoded: %+v", notifications[1])
	}
	want := [][]string{{
		"api", "--paginate", "--slurp", "-X", "GET",
		"-F", "all=true", "-F", "participating=false", "-F", "per_page=100", "notifications",
	}}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Errorf("gh calls = %#v, want %#v", runner.calls, want)
	}
}

func TestListNotificationsReportsCommandError(t *testing.T) {
	runner := &stubRunner{stderr: []byte("HTTP 403: Resource not accessible"), err: errors.New("exit status 1")}
	_, err := NewClientWithRunner(runner).ListNotifications(context.Background())
	if err == nil || !strings.Contains(err.Error(), "permissions granted to gh") {
		t.Fatalf("ListNotifications() error = %v, want actionable permission error", err)
	}
}

func TestGhAuthenticationErrorIsActionable(t *testing.T) {
	runner := &stubRunner{
		stderr: []byte("gh: not logged into any GitHub hosts. Run gh auth login."),
		err:    errors.New("exit status 1"),
	}
	_, err := NewClientWithRunner(runner).Viewer(context.Background())
	if err == nil || !strings.Contains(err.Error(), "gh auth login") {
		t.Fatalf("Viewer() error = %v, want gh auth login guidance", err)
	}
}

func TestGhRateLimitAndPermissionErrorsAreActionable(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
		want   string
	}{
		{name: "rate limit", stderr: "gh: API rate limit exceeded (HTTP 403)", want: "rate limit exceeded"},
		{name: "forbidden", stderr: "gh: Resource not accessible by token (HTTP 403)", want: "permissions granted to gh"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &stubRunner{stderr: []byte(tt.stderr), err: errors.New("exit status 1")}
			_, err := NewClientWithRunner(runner).Viewer(context.Background())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Viewer() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestMissingGhIsActionable(t *testing.T) {
	commandErr := (&CommandError{Err: exec.ErrNotFound}).Error()
	if !strings.Contains(commandErr, "install it") {
		t.Errorf("CommandError() = %q, want install guidance", commandErr)
	}
}

func TestCommandRunnerDisablesPrompts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX shell script")
	}

	binDir := t.TempDir()
	ghPath := filepath.Join(binDir, "gh")
	if err := os.WriteFile(ghPath, []byte("#!/bin/sh\nprintf '%s\\n' \"$GH_PROMPT_DISABLED\" \"$@\"\n"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	stdout, stderr, err := (commandRunner{}).Run(context.Background(), "api", "user")
	if err != nil {
		t.Fatalf("Run() error = %v (stderr %q)", err, stderr)
	}
	if got, want := string(stdout), "1\napi\nuser\n"; got != want {
		t.Fatalf("gh output = %q, want %q", got, want)
	}
}

type stubRunner struct {
	stdout []byte
	stderr []byte
	err    error
	calls  [][]string
}

func (r *stubRunner) Run(_ context.Context, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, append([]string(nil), args...))
	return r.stdout, r.stderr, r.err
}

type routingRunner struct {
	mu        sync.Mutex
	responses map[string][]byte
	stderrs   map[string][]byte
	errors    map[string]error
	calls     [][]string
}

func (r *routingRunner) Run(_ context.Context, args ...string) ([]byte, []byte, error) {
	endpoint := args[len(args)-1]
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, append([]string(nil), args...))
	return r.responses[endpoint], r.stderrs[endpoint], r.errors[endpoint]
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
