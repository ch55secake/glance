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
	Merged     bool      `json:"merged"`
	HTMLURL    string    `json:"html_url"`
	UpdatedAt  time.Time `json:"updated_at"`
	User       User      `json:"user"`
	Repository string    `json:"-"`
}

// PullRequestBranch is a base or head branch on a pull request.
type PullRequestBranch struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}

// PullRequestReview is a review submitted for a pull request.
type PullRequestReview struct {
	User        User      `json:"user"`
	State       string    `json:"state"`
	SubmittedAt time.Time `json:"submitted_at"`
}

// CheckRun is a GitHub Actions or other check attached to the PR's head commit.
type CheckRun struct {
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion"`
	HTMLURL     string     `json:"html_url"`
	CompletedAt *time.Time `json:"completed_at"`
}

// PullRequestDetails contains the fields needed to review and merge a PR.
type PullRequestDetails struct {
	PullRequest
	Body                string              `json:"body"`
	Base                PullRequestBranch   `json:"base"`
	Head                PullRequestBranch   `json:"head"`
	RequestedReviewers  []User              `json:"requested_reviewers"`
	Mergeable           *bool               `json:"mergeable"`
	MergeableState      string              `json:"mergeable_state"`
	AllowedMergeMethods []MergeMethod       `json:"-"`
	Reviews             []PullRequestReview `json:"-"`
	CheckRuns           []CheckRun          `json:"-"`
	ReviewsError        string              `json:"-"`
	ChecksError         string              `json:"-"`
	MergeMethodsError   string              `json:"-"`
}

// PullRequestFile describes a file changed by a pull request, including the
// unified patch when GitHub makes one available.
type PullRequestFile struct {
	Filename         string `json:"filename"`
	PreviousFilename string `json:"previous_filename"`
	Status           string `json:"status"`
	Additions        int    `json:"additions"`
	Deletions        int    `json:"deletions"`
	Changes          int    `json:"changes"`
	Patch            string `json:"patch"`
}

// MergeMethod is a supported GitHub pull request merge method.
type MergeMethod string

const (
	MergeCommit MergeMethod = "merge"
	MergeSquash MergeMethod = "squash"
	MergeRebase MergeMethod = "rebase"
)

// MergeResult is GitHub's response after a successful merge.
type MergeResult struct {
	SHA     string `json:"sha"`
	Merged  bool   `json:"merged"`
	Message string `json:"message"`
}

type repositoryMergeSettings struct {
	AllowMergeCommit bool `json:"allow_merge_commit"`
	AllowSquashMerge bool `json:"allow_squash_merge"`
	AllowRebaseMerge bool `json:"allow_rebase_merge"`
}

// Issue is an open GitHub issue. Pull requests are excluded from ListOpenIssues.
type Issue struct {
	Number     int       `json:"number"`
	Title      string    `json:"title"`
	Body       string    `json:"body"`
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

// SearchPullRequests runs GitHub's issue-search query for PRs. GitHub returns
// PRs as issue results, so their repository is recovered from repository_url.
func (c *Client) SearchPullRequests(ctx context.Context, query string) ([]PullRequest, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("GitHub pull request filter must not be empty")
	}
	if !strings.Contains(strings.ToLower(query), "is:pr") && !strings.Contains(strings.ToLower(query), "type:pr") {
		query = "is:pr " + query
	}
	stdout, err := c.runPaginated(ctx, "search/issues", "q="+query, "sort=updated", "order=desc", fmt.Sprintf("per_page=%d", pageSize))
	if err != nil {
		return nil, err
	}
	var pages []struct {
		Items []struct {
			PullRequest
			RepositoryURL string          `json:"repository_url"`
			PRReference   json.RawMessage `json:"pull_request"`
		} `json:"items"`
	}
	if err := json.Unmarshal(stdout, &pages); err != nil {
		return nil, fmt.Errorf("decode gh api pull request search response: %w", err)
	}
	results := make([]PullRequest, 0)
	for _, page := range pages {
		for _, item := range page.Items {
			if len(item.PRReference) == 0 || bytes.Equal(bytes.TrimSpace(item.PRReference), []byte("null")) {
				continue
			}
			_, path, ok := strings.Cut(item.RepositoryURL, "/repos/")
			if !ok || len(strings.Split(path, "/")) != 2 {
				return nil, fmt.Errorf("invalid repository URL in pull request search result")
			}
			item.Repository = path
			results = append(results, item.PullRequest)
		}
	}
	return results, nil
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

// ListPullRequestFiles returns the changed files and patches for a pull request.
func (c *Client) ListPullRequestFiles(ctx context.Context, owner, repository string, number int) ([]PullRequestFile, error) {
	if !validPathSegment(owner) || !validPathSegment(repository) || number < 1 {
		return nil, errors.New("GitHub owner, repository, and pull request number must be valid")
	}
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d/files", url.PathEscape(owner), url.PathEscape(repository), number)
	stdout, err := c.runPaginated(ctx, endpoint, fmt.Sprintf("per_page=%d", pageSize))
	if err != nil {
		return nil, err
	}
	return decodePaginated[PullRequestFile](stdout, "pull request files")
}

// GetPullRequestDetails returns review, branch, and check information for a PR.
// Detail, review, and check requests are kept separate so a missing check or
// review permission doesn't prevent the main PR content from being shown.
func (c *Client) GetPullRequestDetails(ctx context.Context, owner, repository string, number int) (PullRequestDetails, error) {
	return c.getPullRequestDetails(ctx, owner, repository, number, true)
}

// GetPullRequestPreview skips merge settings, which are only needed in the full review.
func (c *Client) GetPullRequestPreview(ctx context.Context, owner, repository string, number int) (PullRequestDetails, error) {
	return c.getPullRequestDetails(ctx, owner, repository, number, false)
}

func (c *Client) getPullRequestDetails(ctx context.Context, owner, repository string, number int, mergeSettings bool) (PullRequestDetails, error) {
	if !validPathSegment(owner) || !validPathSegment(repository) || number < 1 {
		return PullRequestDetails{}, errors.New("GitHub owner, repository, and pull request number must be valid")
	}
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d", url.PathEscape(owner), url.PathEscape(repository), number)
	data, err := c.runAPI(ctx, endpoint)
	if err != nil {
		return PullRequestDetails{}, err
	}
	var details PullRequestDetails
	if err := json.Unmarshal(data, &details); err != nil {
		return PullRequestDetails{}, fmt.Errorf("decode gh api pull request details response: %w", err)
	}
	details.Repository = owner + "/" + repository

	var requests sync.WaitGroup
	requests.Add(2)
	go func() {
		defer requests.Done()
		reviews, reviewsErr := c.listPullRequestReviews(ctx, owner, repository, number)
		details.Reviews = reviews
		if reviewsErr != nil {
			details.ReviewsError = reviewsErr.Error()
		}
	}()
	go func() {
		defer requests.Done()
		if details.Head.SHA == "" {
			details.ChecksError = "GitHub did not return the pull request head SHA."
			return
		}
		checkRuns, checksErr := c.listCheckRuns(ctx, owner, repository, details.Head.SHA)
		details.CheckRuns = checkRuns
		if checksErr != nil {
			details.ChecksError = checksErr.Error()
		}
	}()
	if mergeSettings {
		requests.Add(1)
		go func() {
			defer requests.Done()
			settings, settingsErr := c.getRepositoryMergeSettings(ctx, owner, repository)
			if settingsErr != nil {
				details.MergeMethodsError = settingsErr.Error()
				return
			}
			if settings.AllowMergeCommit {
				details.AllowedMergeMethods = append(details.AllowedMergeMethods, MergeCommit)
			}
			if settings.AllowSquashMerge {
				details.AllowedMergeMethods = append(details.AllowedMergeMethods, MergeSquash)
			}
			if settings.AllowRebaseMerge {
				details.AllowedMergeMethods = append(details.AllowedMergeMethods, MergeRebase)
			}
		}()
	}
	requests.Wait()
	sort.Slice(details.Reviews, func(i, j int) bool {
		return details.Reviews[i].SubmittedAt.After(details.Reviews[j].SubmittedAt)
	})
	return details, nil
}

func (c *Client) getRepositoryMergeSettings(ctx context.Context, owner, repository string) (repositoryMergeSettings, error) {
	endpoint := fmt.Sprintf("repos/%s/%s", url.PathEscape(owner), url.PathEscape(repository))
	data, err := c.runAPI(ctx, endpoint)
	if err != nil {
		return repositoryMergeSettings{}, err
	}
	var settings repositoryMergeSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return repositoryMergeSettings{}, fmt.Errorf("decode gh api repository merge settings response: %w", err)
	}
	return settings, nil
}

// MergePullRequest merges a pull request if its head still matches expectedSHA.
func (c *Client) MergePullRequest(ctx context.Context, owner, repository string, number int, method MergeMethod, expectedSHA string) (MergeResult, error) {
	if !validPathSegment(owner) || !validPathSegment(repository) || number < 1 {
		return MergeResult{}, errors.New("GitHub owner, repository, and pull request number must be valid")
	}
	if strings.TrimSpace(expectedSHA) == "" {
		return MergeResult{}, errors.New("a current pull request head SHA is required to merge safely")
	}
	switch method {
	case MergeCommit, MergeSquash, MergeRebase:
	default:
		return MergeResult{}, fmt.Errorf("unsupported pull request merge method %q", method)
	}
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d/merge", url.PathEscape(owner), url.PathEscape(repository), number)
	stdout, stderr, err := c.runner.Run(ctx,
		"api", "-X", "PUT",
		"-F", "sha="+expectedSHA,
		"-F", "merge_method="+string(method),
		endpoint,
	)
	if err != nil {
		return MergeResult{}, fmt.Errorf("merge pull request: %w", newMergeCommandError(stderr, err))
	}
	var result MergeResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		return MergeResult{}, fmt.Errorf("decode gh api merge response: %w", err)
	}
	if !result.Merged {
		message := strings.TrimSpace(result.Message)
		if message == "" {
			message = "GitHub did not merge the pull request. Check required reviews, status checks, branch protection, and merge conflicts."
		}
		return result, errors.New(message)
	}
	return result, nil
}

func newMergeCommandError(stderr []byte, err error) error {
	detail := strings.TrimSpace(string(stderr))
	lower := strings.ToLower(detail)
	switch {
	case strings.Contains(lower, "head branch was modified"), strings.Contains(lower, "head sha"),
		strings.Contains(lower, "sha does not match"), strings.Contains(lower, "sha was updated"):
		return fmt.Errorf("the pull request changed since it was loaded; refresh the PR and try again")
	case strings.Contains(lower, "not mergeable"), strings.Contains(lower, "merge conflict"), strings.Contains(lower, "conflict"):
		return fmt.Errorf("GitHub cannot merge this pull request yet; resolve conflicts and check its mergeability")
	case strings.Contains(lower, "required status checks"), strings.Contains(lower, "required review"),
		strings.Contains(lower, "protected branch"), strings.Contains(lower, "branch protection"):
		return fmt.Errorf("branch protection is blocking this merge; satisfy required reviews and status checks")
	default:
		return newCommandError(stderr, err)
	}
}

func (c *Client) listPullRequestReviews(ctx context.Context, owner, repository string, number int) ([]PullRequestReview, error) {
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d/reviews", url.PathEscape(owner), url.PathEscape(repository), number)
	stdout, err := c.runPaginated(ctx, endpoint, fmt.Sprintf("per_page=%d", pageSize))
	if err != nil {
		return nil, err
	}
	return decodePaginated[PullRequestReview](stdout, "pull request reviews")
}

func (c *Client) listCheckRuns(ctx context.Context, owner, repository, sha string) ([]CheckRun, error) {
	endpoint := "repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repository) + "/commits/" + url.PathEscape(sha) + "/check-runs"
	stdout, err := c.runPaginated(ctx, endpoint, fmt.Sprintf("per_page=%d", pageSize))
	if err != nil {
		return nil, err
	}
	var pages []struct {
		CheckRuns []CheckRun `json:"check_runs"`
	}
	if err := json.Unmarshal(stdout, &pages); err != nil {
		return nil, fmt.Errorf("decode gh api check runs response: %w", err)
	}
	checkRuns := make([]CheckRun, 0)
	for _, page := range pages {
		checkRuns = append(checkRuns, page.CheckRuns...)
	}
	return checkRuns, nil
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
