package githubapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNewClientRequiresToken(t *testing.T) {
	t.Setenv(TokenEnvVar, " \t")

	if _, err := NewClientFromEnv(); !errors.Is(err, ErrMissingToken) {
		t.Fatalf("NewClientFromEnv() error = %v, want %v", err, ErrMissingToken)
	}
}

func TestNewClientUsesGitHubAPIByDefault(t *testing.T) {
	client, err := NewClient("test-token")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if client.baseURL != apiBaseURL {
		t.Errorf("base URL = %q, want %q", client.baseURL, apiBaseURL)
	}
	if client.httpClient.Timeout != requestTimeout {
		t.Errorf("HTTP timeout = %s, want %s", client.httpClient.Timeout, requestTimeout)
	}
}

func TestViewerUsesBearerTokenAndDecodesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			t.Errorf("path = %q, want /user", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want Bearer test-token", got)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("Accept = %q, want GitHub JSON media type", got)
		}
		if got := r.Header.Get("X-GitHub-Api-Version"); got != apiVersion {
			t.Errorf("API version = %q, want %q", got, apiVersion)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(User{Login: "octocat", Name: "The Octocat"})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	user, err := client.Viewer(context.Background())
	if err != nil {
		t.Fatalf("Viewer() error = %v", err)
	}
	if user.Login != "octocat" || user.Name != "The Octocat" {
		t.Fatalf("Viewer() = %+v, want octocat user", user)
	}
}

func TestViewerUnauthorizedErrorDoesNotLeakToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprintln(w, `{"message":"Bad credentials"}`)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	client.token = "sensitive-token"
	_, err := client.Viewer(context.Background())

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Viewer() error = %T %v, want *APIError", err, err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", apiErr.StatusCode, http.StatusUnauthorized)
	}
	if !strings.Contains(err.Error(), TokenEnvVar) {
		t.Errorf("error %q does not explain how to fix authentication", err)
	}
	if strings.Contains(err.Error(), client.token) {
		t.Errorf("error exposes token: %q", err)
	}
}

func TestRateLimitErrorIncludesRetryInformation(t *testing.T) {
	resetAt := time.Now().Add(time.Minute).Unix()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetAt, 10))
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprintln(w, `{"message":"API rate limit exceeded"}`)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	_, err := client.Viewer(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Viewer() error = %T %v, want *APIError", err, err)
	}
	if !apiErr.RateLimited || apiErr.RateLimitReset.IsZero() {
		t.Fatalf("rate limit metadata = %+v, want rate-limited with reset time", apiErr)
	}
	if !strings.Contains(err.Error(), "rate limit exceeded") {
		t.Errorf("error %q does not explain rate limiting", err)
	}
}

func TestListPullRequestsPaginates(t *testing.T) {
	var requestedPages []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/octo/repo/pulls" {
			t.Errorf("path = %q, want /repos/octo/repo/pulls", r.URL.Path)
		}
		if r.URL.Query().Get("per_page") != strconv.Itoa(pageSize) {
			t.Errorf("per_page = %q, want %d", r.URL.Query().Get("per_page"), pageSize)
		}
		if r.URL.Query().Get("state") != "open" {
			t.Errorf("state = %q, want open", r.URL.Query().Get("state"))
		}

		page := r.URL.Query().Get("page")
		requestedPages = append(requestedPages, page)
		w.Header().Set("Content-Type", "application/json")
		switch page {
		case "1":
			firstPage := make([]PullRequest, pageSize)
			for i := range firstPage {
				firstPage[i].Number = i + 1
			}
			_ = json.NewEncoder(w).Encode(firstPage)
		case "2":
			_ = json.NewEncoder(w).Encode([]PullRequest{{Number: 101}})
		default:
			t.Errorf("unexpected page request: %s", page)
			_ = json.NewEncoder(w).Encode([]PullRequest{})
		}
	}))
	defer server.Close()

	client := newTestClient(t, server)
	pullRequests, err := client.ListPullRequests(context.Background(), "octo", "repo")
	if err != nil {
		t.Fatalf("ListPullRequests() error = %v", err)
	}
	if len(pullRequests) != 101 {
		t.Fatalf("ListPullRequests() returned %d items, want 101", len(pullRequests))
	}
	if pullRequests[100].Number != 101 {
		t.Errorf("last pull request number = %d, want 101", pullRequests[100].Number)
	}
	if got := strings.Join(requestedPages, ","); got != "1,2" {
		t.Errorf("requested pages = %q, want 1,2", got)
	}
}

func TestForbiddenErrorIsNotMisreportedAsRateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprintln(w, `{"message":"Resource not accessible by token"}`)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	_, err := client.Viewer(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Viewer() error = %T %v, want *APIError", err, err)
	}
	if apiErr.RateLimited {
		t.Fatalf("API error incorrectly classified as a rate limit: %+v", apiErr)
	}
	if !strings.Contains(err.Error(), "token permissions") {
		t.Errorf("error %q does not explain permission failure", err)
	}
}

func newTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client, err := newClient("test-token", server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClient() error = %v", err)
	}
	return client
}
