// Package worker provides a client to communicate with the Claude Worker container
// via HTTP API, allowing vibe-git to run Claude commands in an isolated environment.
package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client provides methods to interact with the Claude Worker container
type Client struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewClient creates a new Worker client
func NewClient(baseURL, token string) *Client {
	if baseURL == "" {
		baseURL = "http://localhost:3000"
	}
	return &Client{
		baseURL: baseURL,
		token:   token,
		client:  &http.Client{Timeout: 300 * time.Second},
	}
}

// ClaudeRunRequest represents a request to run Claude
type ClaudeRunRequest struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Timeout int      `json:"timeout"`
	Stdin   string   `json:"stdin,omitempty"`
}

// ClaudeRunResponse represents the response from running Claude
type ClaudeRunResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Duration string `json:"duration"`
}

// RunClaude executes a Claude command in the worker container
func (c *Client) RunClaude(ctx context.Context, command string, args []string, timeout int) (*ClaudeRunResponse, error) {
	reqBody := ClaudeRunRequest{
		Command: command,
		Args:    args,
		Timeout: timeout,
	}

	resp, err := c.doRequest(ctx, "POST", "/claude/run", reqBody)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("claude run failed: %s", string(body))
	}

	var result ClaudeRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &result, nil
}

// GitStatus returns the git status of the project
func (c *Client) GitStatus(ctx context.Context) (string, error) {
	resp, err := c.doRequest(ctx, "GET", "/git/status", nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Success bool   `json:"success"`
		Output  string `json:"output"`
		Error   string `json:"error,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if !result.Success {
		return "", fmt.Errorf("git status failed: %s", result.Error)
	}

	return result.Output, nil
}

// GitDiff returns the git diff
func (c *Client) GitDiff(ctx context.Context, cached bool, file string) (string, error) {
	url := "/git/diff"
	if cached {
		url += "?cached=true"
	}
	if file != "" {
		sep := "?"
		if cached {
			sep = "&"
		}
		url += sep + "file=" + file
	}

	resp, err := c.doRequest(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Success bool   `json:"success"`
		Output  string `json:"output"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.Output, nil
}

// FileRead reads a file from the project
func (c *Client) FileRead(ctx context.Context, path string) (string, error) {
	reqBody := map[string]string{"path": path}

	resp, err := c.doRequest(ctx, "POST", "/file/read", reqBody)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		Size    int    `json:"size"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.Content, nil
}

// FileWrite writes a file in the project
func (c *Client) FileWrite(ctx context.Context, path string, content string) error {
	reqBody := map[string]string{
		"path":    path,
		"content": content,
	}

	resp, err := c.doRequest(ctx, "POST", "/file/write", reqBody)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Success bool `json:"success"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if !result.Success {
		return fmt.Errorf("file write failed")
	}

	return nil
}

// FileList lists files in a directory
func (c *Client) FileList(ctx context.Context, dir string) ([]map[string]interface{}, error) {
	url := "/file/list"
	if dir != "" {
		url += "?dir=" + dir
	}

	resp, err := c.doRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Path  string                   `json:"path"`
		Files []map[string]interface{} `json:"files"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Files, nil
}

// ProjectInfo returns project information
func (c *Client) ProjectInfo(ctx context.Context) (map[string]interface{}, error) {
	resp, err := c.doRequest(ctx, "GET", "/project/info", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}

// Health checks if the worker is healthy
func (c *Client) Health(ctx context.Context) (map[string]interface{}, error) {
	resp, err := c.doRequest(ctx, "GET", "/health", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}

// doRequest performs an HTTP request
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("X-Worker-Auth", c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.client.Do(req)
}
