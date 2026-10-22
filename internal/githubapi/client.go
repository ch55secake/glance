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
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	pageSize                  = 100
	maxConcurrentRepoRequests = 5
)

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

// Repository is a repository visible to the authenticated user.
type Repository struct {
	Name      string `json:"name"`
	FullName  string `json:"full_name"`
	Owner     User   `json:"owner"`
	HasIssues bool   `json:"has_issues"`
}

// PullRequest contains the fields needed to display a pull request in glance.
type PullRequest struct {
	Number     int       `json:"number"`
	Title      string    `json:"title"`
	State      string    `json:"state"`
	Draft      bool      `json:"draft"`
	HTMLURL    string    `json:"html_url"`
	UpdatedAt  time.Time `json:"updated_at"`
	User       User      `json:"user"`
	Repository string    `json:"-"`
}

// Issue is an open GitHub issue. Pull requests are excluded from ListOpenIssues.
type Issue struct {
	Number     int       `json:"number"`
	Title      string    `json:"title"`
	State      string    `json:"state"`
	HTMLURL    string    `json:"html_url"`
	UpdatedAt  time.Time `json:"updated_at"`
	User       User      `json:"user"`
	Labels     []Label   `json:"labels"`
	Comments   int       `json:"comments"`
	Repository string    `json:"-"`
}

// Label is a label attached to an issue.
type Label struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Notification is a GitHub notification thread for an issue or pull request.
type Notification struct {
	ID         string                 `json:"id"`
	Unread     bool                   `json:"unread"`
	Reason     string                 `json:"reason"`
	UpdatedAt  time.Time              `json:"updated_at"`
	LastReadAt *time.Time             `json:"last_read_at"`
	Subject    NotificationSubject    `json:"subject"`
	Repository NotificationRepository `json:"repository"`
}

// NotificationSubject contains the notification's subject metadata.
type NotificationSubject struct {
	Title            string `json:"title"`
	URL              string `json:"url"`
	LatestCommentURL string `json:"latest_comment_url"`
	Type             string `json:"type"`
}

// NotificationRepository identifies the repository a notification belongs to.
type NotificationRepository struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	HTMLURL  string `json:"html_url"`
	Owner    User   `json:"owner"`
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

// ListRepositories returns all repositories visible to the authenticated user.
func (c *Client) ListRepositories(ctx context.Context) ([]Repository, error) {
	stdout, err := c.runPaginated(ctx, "user/repos",
		"affiliation=owner,collaborator,organization_member",
		fmt.Sprintf("per_page=%d", pageSize),
	)
	if err != nil {
		return nil, err
	}
	return decodePaginated[Repository](stdout, "repositories")
}

// ListOpenPullRequests returns open pull requests across all visible repositories.
func (c *Client) ListOpenPullRequests(ctx context.Context) ([]PullRequest, error) {
	repositories, err := c.ListRepositories(ctx)
	if err != nil {
		return nil, err
	}
	pullRequests, err := collectRepositoryItems(ctx, repositories, "pull requests",
		func(ctx context.Context, repository Repository) ([]PullRequest, error) {
			return c.ListPullRequests(ctx, repository.Owner.Login, repository.Name)
		},
		func(pr *PullRequest, repository string) { pr.Repository = repository },
	)
	if err != nil {
		return nil, err
	}

	sort.Slice(pullRequests, func(i, j int) bool {
		left, right := pullRequests[i], pullRequests[j]
		if !left.UpdatedAt.Equal(right.UpdatedAt) {
			return left.UpdatedAt.After(right.UpdatedAt)
		}
		if left.Repository != right.Repository {
			return left.Repository < right.Repository
		}
		return left.Number > right.Number
	})
	return pullRequests, nil
}

// ListOpenIssues returns open issues across all repositories visible to the user.
// Pull requests are excluded because GitHub's issues endpoint includes them.
func (c *Client) ListOpenIssues(ctx context.Context) ([]Issue, error) {
	repositories, err := c.ListRepositories(ctx)
	if err != nil {
		return nil, err
	}
	repositoriesWithIssues := make([]Repository, 0, len(repositories))
	for _, repository := range repositories {
		if repository.HasIssues {
			repositoriesWithIssues = append(repositoriesWithIssues, repository)
		}
	}
	issues, err := collectRepositoryItems(ctx, repositoriesWithIssues, "issues",
		func(ctx context.Context, repository Repository) ([]Issue, error) {
			return c.listOpenIssuesForRepository(ctx, repository)
		},
		func(issue *Issue, repository string) { issue.Repository = repository },
	)
	if err != nil {
		return nil, err
	}
	sort.Slice(issues, func(i, j int) bool {
		left, right := issues[i], issues[j]
		if !left.UpdatedAt.Equal(right.UpdatedAt) {
			return left.UpdatedAt.After(right.UpdatedAt)
		}
		if left.Repository != right.Repository {
			return left.Repository < right.Repository
		}
		return left.Number > right.Number
	})
	return issues, nil
}

// ListNotifications returns all notification threads, including read threads.
// Unread status is retained so callers can choose their own inbox view.
func (c *Client) ListNotifications(ctx context.Context) ([]Notification, error) {
	stdout, err := c.runPaginated(ctx, "notifications",
		"all=true",
		"participating=false",
		fmt.Sprintf("per_page=%d", pageSize),
	)
	if err != nil {
		return nil, err
	}
	notifications, err := decodePaginated[Notification](stdout, "notifications")
	if err != nil {
		return nil, err
	}
	sort.Slice(notifications, func(i, j int) bool {
		return notifications[i].UpdatedAt.After(notifications[j].UpdatedAt)
	})
	return notifications, nil
}

type issueAPIResponse struct {
	Issue
	PullRequest json.RawMessage `json:"pull_request"`
}

func (c *Client) listOpenIssuesForRepository(ctx context.Context, repository Repository) ([]Issue, error) {
	if !validPathSegment(repository.Owner.Login) || !validPathSegment(repository.Name) {
		return nil, errors.New("GitHub owner and repository must be non-empty path segments")
	}
	endpoint := "repos/" + url.PathEscape(repository.Owner.Login) + "/" + url.PathEscape(repository.Name) + "/issues"
	stdout, err := c.runPaginated(ctx, endpoint, "state=open", fmt.Sprintf("per_page=%d", pageSize))
	if err != nil {
		return nil, err
	}
	var pages [][]issueAPIResponse
	if err := json.Unmarshal(stdout, &pages); err != nil {
		return nil, fmt.Errorf("decode gh api issues response: %w", err)
	}
	issues := make([]Issue, 0)
	for _, page := range pages {
		for _, item := range page {
			if len(item.PullRequest) != 0 && !bytes.Equal(bytes.TrimSpace(item.PullRequest), []byte("null")) {
				continue
			}
			issues = append(issues, item.Issue)
		}
	}
	return issues, nil
}

func collectRepositoryItems[T any](
	ctx context.Context,
	repositories []Repository,
	itemName string,
	fetch func(context.Context, Repository) ([]T, error),
	setRepository func(*T, string),
) ([]T, error) {
	if len(repositories) == 0 {
		return []T{}, nil
	}

	type result struct {
		items []T
		err   error
	}
	workerCount := min(maxConcurrentRepoRequests, len(repositories))
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan Repository, len(repositories))
	results := make(chan result, len(repositories))
	for _, repository := range repositories {
		jobs <- repository
	}
	close(jobs)

	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for repository := range jobs {
				items, err := fetch(requestCtx, repository)
				if err != nil {
					results <- result{err: fmt.Errorf("list %s for %s: %w", itemName, repository.FullName, err)}
					cancel()
					return
				}
				for i := range items {
					setRepository(&items[i], repository.FullName)
				}
				results <- result{items: items}
			}
		}()
	}
	go func() {
		workers.Wait()
		close(results)
	}()

	allItems := make([]T, 0)
	var firstErr error
	for result := range results {
		if result.err != nil {
			if firstErr == nil {
				firstErr = result.err
			}
			continue
		}
		allItems = append(allItems, result.items...)
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return allItems, nil
}

// ListPullRequests returns all open pull requests for a repository.
func (c *Client) ListPullRequests(ctx context.Context, owner, repository string) ([]PullRequest, error) {
	if !validPathSegment(owner) || !validPathSegment(repository) {
		return nil, errors.New("GitHub owner and repository must be non-empty path segments")
	}

	endpoint := "repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repository) + "/pulls"
	stdout, err := c.runPaginated(ctx, endpoint, "state=open", fmt.Sprintf("per_page=%d", pageSize))
	if err != nil {
		return nil, err
	}
	return decodePaginated[PullRequest](stdout, "pull requests")
}

func (c *Client) runAPI(ctx context.Context, endpoint string) ([]byte, error) {
	stdout, stderr, err := c.runner.Run(ctx, "api", endpoint)
	if err != nil {
		return nil, newCommandError(stderr, err)
	}
	return stdout, nil
}

func (c *Client) runPaginated(ctx context.Context, endpoint string, fields ...string) ([]byte, error) {
	args := []string{"api", "--paginate", "--slurp", "-X", "GET"}
	for _, field := range fields {
		args = append(args, "-F", field)
	}
	args = append(args, endpoint)
	stdout, stderr, err := c.runner.Run(ctx, args...)
	if err != nil {
		return nil, newCommandError(stderr, err)
	}
	return stdout, nil
}

func decodePaginated[T any](data []byte, resource string) ([]T, error) {
	var pages [][]T
	if err := json.Unmarshal(data, &pages); err != nil {
		return nil, fmt.Errorf("decode gh api %s response: %w", resource, err)
	}
	items := make([]T, 0)
	for _, page := range pages {
		items = append(items, page...)
	}
	return items, nil
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
