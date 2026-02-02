package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const githubAPIURL = "https://api.github.com"

// Client wraps the GitHub API
type Client struct {
	token  string
	owner  string
	repo   string
	http   *http.Client
}

// Issue represents a GitHub issue
type Issue struct {
	Number  int
	Title   string
	Body    string
	URL     string
	State   string
	Labels  []string
}

// NewClient creates a new GitHub client
func NewClient(token, owner, repo string) *Client {
	return &Client{
		token: token,
		owner: owner,
		repo:  repo,
		http:  &http.Client{},
	}
}

// GetIssue fetches a single issue by number
func (c *Client) GetIssue(ctx context.Context, number int) (*Issue, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d", githubAPIURL, c.owner, c.repo, number)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching issue %d: %w", number, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		Body    string `json:"body"`
		HTMLURL string `json:"html_url"`
		State   string `json:"state"`
		Labels  []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	labels := make([]string, len(result.Labels))
	for i, label := range result.Labels {
		labels[i] = label.Name
	}

	return &Issue{
		Number: result.Number,
		Title:  result.Title,
		Body:   result.Body,
		URL:    result.HTMLURL,
		State:  result.State,
		Labels: labels,
	}, nil
}

// CreatePullRequest creates a new pull request
func (c *Client) CreatePullRequest(ctx context.Context, base, head, title, body string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls", githubAPIURL, c.owner, c.repo)

	requestBody := map[string]interface{}{
		"title": title,
		"head":  head,
		"base":  base,
		"body":  body,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("creating PR: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		HTMLURL string `json:"html_url"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	return result.HTMLURL, nil
}

// CreatePullRequestWithNumber creates a new pull request and returns PR number and URL
func (c *Client) CreatePullRequestWithNumber(ctx context.Context, base, head, title, body string) (int, string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls", githubAPIURL, c.owner, c.repo)

	requestBody := map[string]interface{}{
		"title": title,
		"head":  head,
		"base":  base,
		"body":  body,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return 0, "", fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return 0, "", err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("creating PR: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return 0, "", fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		HTMLURL string `json:"html_url"`
		Number  int    `json:"number"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, "", fmt.Errorf("decoding response: %w", err)
	}

	return result.Number, result.HTMLURL, nil
}

// MergePullRequest merges a pull request
func (c *Client) MergePullRequest(ctx context.Context, prNumber int, commitTitle, commitMessage string) error {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/merge", githubAPIURL, c.owner, c.repo, prNumber)

	requestBody := map[string]interface{}{
		"commit_title":   commitTitle,
		"commit_message": commitMessage,
		"merge_method":   "squash",
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("merging PR: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// CloseIssue closes an issue
func (c *Client) CloseIssue(ctx context.Context, issueNumber int) error {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d", githubAPIURL, c.owner, c.repo, issueNumber)

	requestBody := map[string]interface{}{
		"state": "closed",
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PATCH", url, bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("closing issue: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// WaitForMergeable waits for PR to be mergeable
func (c *Client) WaitForMergeable(ctx context.Context, prNumber int, timeout time.Duration) error {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d", githubAPIURL, c.owner, c.repo, prNumber)

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for PR to be mergeable")
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				return err
			}

			req.Header.Set("Authorization", "Bearer "+c.token)
			req.Header.Set("Accept", "application/vnd.github+json")

			resp, err := c.http.Do(req)
			if err != nil {
				resp.Body.Close()
				continue
			}

			var result struct {
				Mergeable *bool  `json:"mergeable"`
				State     string `json:"state"`
			}

			json.NewDecoder(resp.Body).Decode(&result)
			resp.Body.Close()

			if result.State == "closed" {
				return fmt.Errorf("PR was closed")
			}

			if result.Mergeable != nil && *result.Mergeable {
				return nil
			}
		}
	}
}

// GetDefaultBranch returns the default branch for the repository
func (c *Client) GetDefaultBranch(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s", githubAPIURL, c.owner, c.repo)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching repo: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		DefaultBranch string `json:"default_branch"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	return result.DefaultBranch, nil
}

// ListRecentIssues lists issues created after the given time
func (c *Client) ListRecentIssues(ctx context.Context, since time.Time) ([]*Issue, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/issues?state=open&sort=created&direction=desc&since=%s",
		githubAPIURL, c.owner, c.repo, since.Format(time.RFC3339))

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching issues: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	var results []struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		Body    string `json:"body"`
		HTMLURL string `json:"html_url"`
		State   string `json:"state"`
		Labels  []struct {
			Name string `json:"name"`
		} `json:"labels"`
		PullRequest *struct{} `json:"pull_request"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	var issues []*Issue
	for _, r := range results {
		// Skip pull requests (GitHub API returns them as issues too)
		if r.PullRequest != nil {
			continue
		}

		labels := make([]string, len(r.Labels))
		for i, label := range r.Labels {
			labels[i] = label.Name
		}

		issues = append(issues, &Issue{
			Number: r.Number,
			Title:  r.Title,
			Body:   r.Body,
			URL:    r.HTMLURL,
			State:  r.State,
			Labels: labels,
		})
	}

	return issues, nil
}
