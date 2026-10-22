// Package githubapi provides non-interactive GitHub API access through gh.
package githubapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

const pageSize = 100

// Runner executes a non-interactive gh command and returns its output.
type Runner interface {
	Run(ctx context.Context, args ...string) (stdout, stderr []byte, err error)
}

// Client uses `gh api` to access GitHub with the user's existing gh authentication.
type Client struct {
	runner Runner
}

// User is the authenticated GitHub account.
type User struct {
	Login string `json:"login"`
	Name  string `json:"name"`
}

// PullRequest contains the fields needed to display a pull request in glance.
type PullRequest struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	State     string    `json:"state"`
	Draft     bool      `json:"draft"`
	HTMLURL   string    `json:"html_url"`
	UpdatedAt time.Time `json:"updated_at"`
	User      User      `json:"user"`
}

// CommandError describes a failure while invoking gh api.
type CommandError struct {
	Stderr string
	Err    error
}

func (e *CommandError) Error() string {
	detail := strings.TrimSpace(e.Stderr)
	lowerDetail := strings.ToLower(detail)

	switch {
	case errors.Is(e.Err, exec.ErrNotFound):
		return "GitHub CLI (gh) was not found; install it and ensure it is on PATH."
	case isAuthenticationError(lowerDetail):
		return "GitHub CLI is not authenticated; run `gh auth login` before starting glance."
	case errors.Is(e.Err, context.DeadlineExceeded):
		return "GitHub API request timed out; try again."
	case errors.Is(e.Err, context.Canceled):
		return "GitHub API request canceled."
	case strings.Contains(lowerDetail, "rate limit"):
		return "GitHub API rate limit exceeded; try again later."
	case strings.Contains(lowerDetail, "http 403") || strings.Contains(lowerDetail, "http 404"):
		return "GitHub denied access; check repository access and the permissions granted to gh."
	case detail != "":
		return fmt.Sprintf("gh api failed: %s", detail)
	case e.Err != nil:
		return fmt.Sprintf("gh api failed: %v", e.Err)
	default:
		return "gh api failed."
	}
}

func (e *CommandError) Unwrap() error {
	return e.Err
}

// NewClient creates a client backed by the local GitHub CLI.
func NewClient() *Client {
	return NewClientWithRunner(commandRunner{})
}

// NewClientWithRunner creates a client with an injectable command runner.
func NewClientWithRunner(runner Runner) *Client {
	if runner == nil {
		runner = commandRunner{}
	}
	return &Client{runner: runner}
}

func (c *Client) Viewer(ctx context.Context) (User, error) {
	data, err := c.runAPI(ctx, "user")
	if err != nil {
		return User{}, err
	}

	var user User
	if err := json.Unmarshal(data, &user); err != nil {
		return User{}, fmt.Errorf("decode gh api user response: %w", err)
	}
	return user, nil
}

// ListPullRequests returns all open pull requests for a repository.
func (c *Client) ListPullRequests(ctx context.Context, owner, repository string) ([]PullRequest, error) {
	if !validPathSegment(owner) || !validPathSegment(repository) {
		return nil, errors.New("GitHub owner and repository must be non-empty path segments")
	}

	endpoint := "repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repository) + "/pulls"
	stdout, stderr, err := c.runner.Run(
		ctx,
		"api",
		"--paginate",
		"--slurp",
		"-X", "GET",
		"-F", "state=open",
		"-F", fmt.Sprintf("per_page=%d", pageSize),
		endpoint,
	)
	if err != nil {
		return nil, newCommandError(stderr, err)
	}

	var pages [][]PullRequest
	if err := json.Unmarshal(stdout, &pages); err != nil {
		return nil, fmt.Errorf("decode gh api pull requests response: %w", err)
	}

	pullRequests := make([]PullRequest, 0)
	for _, page := range pages {
		pullRequests = append(pullRequests, page...)
	}
	return pullRequests, nil
}

func (c *Client) runAPI(ctx context.Context, endpoint string) ([]byte, error) {
	stdout, stderr, err := c.runner.Run(ctx, "api", endpoint)
	if err != nil {
		return nil, newCommandError(stderr, err)
	}
	return stdout, nil
}

func newCommandError(stderr []byte, err error) *CommandError {
	return &CommandError{Stderr: string(stderr), Err: err}
}

func isAuthenticationError(message string) bool {
	return strings.Contains(message, "not logged in") ||
		strings.Contains(message, "not logged into any github hosts") ||
		strings.Contains(message, "authentication required") ||
		strings.Contains(message, "bad credentials") ||
		strings.Contains(message, "http 401")
}

func validPathSegment(value string) bool {
	return value != "" && value != "." && value != ".." && !strings.ContainsAny(value, "/\\")
}

type commandRunner struct{}

func (commandRunner) Run(ctx context.Context, args ...string) ([]byte, []byte, error) {
	command := exec.CommandContext(ctx, "gh", args...)
	command.Env = setEnvironmentVariable(os.Environ(), "GH_PROMPT_DISABLED", "1")

	var stderr bytes.Buffer
	command.Stderr = &stderr
	stdout, err := command.Output()
	return stdout, stderr.Bytes(), err
}

func setEnvironmentVariable(environment []string, name, value string) []string {
	prefix := name + "="
	for i, entry := range environment {
		if strings.HasPrefix(entry, prefix) {
			environment[i] = prefix + value
			return environment
		}
	}
	return append(environment, prefix+value)
}
