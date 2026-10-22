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
