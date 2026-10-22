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
