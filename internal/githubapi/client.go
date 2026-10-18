// Package githubapi provides the GitHub API client used by glance.
package githubapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// TokenEnvVar is the environment variable used to supply the GitHub token.
	TokenEnvVar       = "GLANCE_GITHUB_TOKEN"
	apiBaseURL        = "https://api.github.com"
	apiVersion        = "2022-11-28"
	pageSize          = 100
	requestTimeout    = 15 * time.Second
	maxResponseLength = 4 << 20
	maxErrorLength    = 64 << 10
)

// ErrMissingToken indicates that no GitHub token was configured.
var ErrMissingToken = errors.New("set GLANCE_GITHUB_TOKEN to connect to GitHub")

// Client makes authenticated requests to the GitHub REST API.
type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
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

// APIError describes an unsuccessful response from the GitHub API.
type APIError struct {
	StatusCode     int
	Message        string
	RateLimited    bool
	RetryAfter     time.Duration
	RateLimitReset time.Time
}

func (e *APIError) Error() string {
	switch {
	case e.StatusCode == http.StatusUnauthorized:
		return "GitHub authentication failed; check that GLANCE_GITHUB_TOKEN is valid and has access."
	case e.RateLimited:
		if e.RetryAfter > 0 {
			return fmt.Sprintf("GitHub API rate limit exceeded; retry in %s.", e.RetryAfter.Round(time.Second))
		}
		if !e.RateLimitReset.IsZero() {
			return fmt.Sprintf("GitHub API rate limit exceeded; retry after %s.", e.RateLimitReset.Format(time.RFC3339))
		}
		return "GitHub API rate limit exceeded; try again later."
	case e.StatusCode == http.StatusForbidden:
		return "GitHub denied access; check token permissions and repository access."
	case e.StatusCode == http.StatusNotFound:
		return "GitHub resource was not found or is not accessible to this token."
	case e.Message != "":
		return fmt.Sprintf("GitHub API request failed (HTTP %d): %s", e.StatusCode, e.Message)
	default:
		return fmt.Sprintf("GitHub API request failed (HTTP %d).", e.StatusCode)
	}
}

func NewClient(token string) (*Client, error) {
	return newClient(token, &http.Client{Timeout: requestTimeout}, apiBaseURL)
}

func NewClientFromEnv() (*Client, error) {
	return NewClient(os.Getenv(TokenEnvVar))
}

func newClient(token string, httpClient *http.Client, baseURL string) (*Client, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrMissingToken
	}
	if httpClient == nil {
		return nil, errors.New("GitHub API HTTP client must not be nil")
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("invalid GitHub API base URL: %w", err)
	}

	return &Client{token: token, baseURL: strings.TrimRight(baseURL, "/"), httpClient: httpClient}, nil
}

func (c *Client) Viewer(ctx context.Context) (User, error) {
	var user User
	err := c.getJSON(ctx, "/user", nil, &user)
	return user, err
}

// ListPullRequests returns all open pull requests for a repository.
func (c *Client) ListPullRequests(ctx context.Context, owner, repository string) ([]PullRequest, error) {
	if !validPathSegment(owner) || !validPathSegment(repository) {
		return nil, errors.New("GitHub owner and repository must be non-empty path segments")
	}

	endpoint := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repository) + "/pulls"
	pullRequests := make([]PullRequest, 0)
	for page := 1; ; page++ {
		query := url.Values{
			"page":     {strconv.Itoa(page)},
			"per_page": {strconv.Itoa(pageSize)},
			"state":    {"open"},
		}
		var currentPage []PullRequest
		if err := c.getJSON(ctx, endpoint, query, &currentPage); err != nil {
			return nil, err
		}
		pullRequests = append(pullRequests, currentPage...)
		if len(currentPage) < pageSize {
			return pullRequests, nil
		}
	}
}

func (c *Client) getJSON(ctx context.Context, endpoint string, query url.Values, result any) error {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Errorf("parse GitHub API base URL: %w", err)
	}
	base.Path = strings.TrimRight(base.Path, "/") + endpoint
	base.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return fmt.Errorf("create GitHub API request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("X-GitHub-Api-Version", apiVersion)

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("request GitHub API: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return decodeAPIError(response)
	}

	decoder := json.NewDecoder(io.LimitReader(response.Body, maxResponseLength))
	if err := decoder.Decode(result); err != nil {
		return fmt.Errorf("decode GitHub API response: %w", err)
	}
	return nil
}

func validPathSegment(value string) bool {
	return value != "" && value != "." && value != ".." && !strings.ContainsAny(value, "/\\")
}

func decodeAPIError(response *http.Response) error {
	var body struct {
		Message string `json:"message"`
	}
	_ = json.NewDecoder(io.LimitReader(response.Body, maxErrorLength)).Decode(&body)

	retryAfter := parseRetryAfter(response.Header.Get("Retry-After"))
	resetUnix, _ := strconv.ParseInt(response.Header.Get("X-RateLimit-Reset"), 10, 64)
	rateLimited := response.StatusCode == http.StatusTooManyRequests ||
		(response.StatusCode == http.StatusForbidden && (response.Header.Get("X-RateLimit-Remaining") == "0" || retryAfter > 0))

	apiError := &APIError{
		StatusCode:  response.StatusCode,
		Message:     body.Message,
		RateLimited: rateLimited,
		RetryAfter:  retryAfter,
	}
	if resetUnix > 0 {
		apiError.RateLimitReset = time.Unix(resetUnix, 0)
	}
	return apiError
}

func parseRetryAfter(value string) time.Duration {
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if retryAt, err := http.ParseTime(value); err == nil {
		if delay := time.Until(retryAt); delay > 0 {
			return delay
		}
	}
	return 0
}
