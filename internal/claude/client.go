package claude

import (
	"bytes"
	stdctx "context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"vibe-git/internal/ctxloader"
)

const anthropicAPIURL = "https://api.anthropic.com/v1/messages"

// Client wraps the Anthropic API
type Client struct {
	apiKey string
	model  string
	http   *http.Client
}

// FileChange represents a file modification
type FileChange struct {
	Path      string `json:"path"`
	Operation string `json:"operation"` // "create", "modify", "delete"
	Content   string `json:"content"`
}

// NewClient creates a new Claude client
func NewClient(apiKey, model string) *Client {
	return &Client{
		apiKey: apiKey,
		model:  model,
		http:   &http.Client{},
	}
}

// GenerateCode generates code changes based on the issue
func (c *Client) GenerateCode(ctx stdctx.Context, issueTitle, issueBody string, referencedFiles []*ctxloader.FileReference) ([]FileChange, error) {
	// Build prompt with context
	prompt, err := c.buildPrompt(issueTitle, issueBody, referencedFiles)
	if err != nil {
		return nil, fmt.Errorf("building prompt: %w", err)
	}

	requestBody := map[string]interface{}{
		"model":      c.model,
		"max_tokens": 4096,
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": prompt,
			},
		},
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", anthropicAPIURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", c.apiKey)
	req.Header.Set("Anthropic-Version", "2023-06-01")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling Claude API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	// Extract text content
	var responseText string
	for _, block := range result.Content {
		if block.Type == "text" {
			responseText += block.Text
		}
	}

	// Parse JSON changes
	changes, err := parseChangesFromResponse(responseText)
	if err != nil {
		return nil, fmt.Errorf("parsing changes: %w", err)
	}

	return changes, nil
}

// buildPrompt builds the complete prompt with issue and context
func (c *Client) buildPrompt(issueTitle, issueBody string, referencedFiles []*ctxloader.FileReference) (string, error) {
	var sb strings.Builder

	sb.WriteString("You are an expert software developer. Given a GitHub issue, analyze the codebase and implement the necessary changes.\n\n")

	// Issue information
	sb.WriteString("## Issue Title\n")
	sb.WriteString(issueTitle)
	sb.WriteString("\n\n")

	sb.WriteString("## Issue Description\n")
	sb.WriteString(issueBody)
	sb.WriteString("\n\n")

	// Referenced files (from @mentions)
	if len(referencedFiles) > 0 {
		sb.WriteString(ctxloader.BuildReferencedFilesSection(referencedFiles))
	}

	// Full codebase context
	sb.WriteString("## Current Codebase\n\n")

	// Build exclude list from referenced files
	excludeFiles := make([]string, 0)
	for _, f := range referencedFiles {
		if f.Found {
			excludeFiles = append(excludeFiles, f.Path)
		}
	}

	codebase, err := ctxloader.BuildCodebaseSection(".", excludeFiles)
	if err != nil {
		return "", err
	}
	sb.WriteString(codebase)

	sb.WriteString("\n\n")
	sb.WriteString("Please analyze this issue and provide the necessary code changes.")
	sb.WriteString(" Pay special attention to the referenced files mentioned with @ in the issue.\n\n")
	sb.WriteString("Return your response as a JSON array of file changes:\n\n")
	sb.WriteString("[\n")
	sb.WriteString("  {\n")
	sb.WriteString("    \"path\": \"relative/path/to/file.go\",\n")
	sb.WriteString("    \"operation\": \"create|modify|delete\",\n")
	sb.WriteString("    \"content\": \"full content of the file\"\n")
	sb.WriteString("  }\n")
	sb.WriteString("]\n\n")
	sb.WriteString("Guidelines:\n")
	sb.WriteString("- Only modify files that need to change\n")
	sb.WriteString("- Provide complete file content, not diffs\n")
	sb.WriteString("- Follow existing code patterns and style\n")
	sb.WriteString("- Include all necessary imports\n")
	sb.WriteString("- Write tests if the issue involves new functionality\n")
	sb.WriteString("- Ensure code compiles and is syntactically correct\n")
	if len(referencedFiles) > 0 {
		sb.WriteString("- The @referenced files are particularly relevant to this issue\n")
	}
	sb.WriteString("\nRespond ONLY with the JSON array, no other text.")

	return sb.String(), nil
}

// ResolveConflict resolves a git merge conflict using Claude
func (c *Client) ResolveConflict(ctx stdctx.Context, filePath string, conflictContent string, issueTitle string) (string, error) {
	prompt := "You are an expert software developer. Resolve the following git merge conflict.\n\n" +
		"## Context\n" +
		"This conflict occurred while implementing: " + issueTitle + "\n\n" +
		"## Conflicted File: " + filePath + "\n" +
		"```\n" + conflictContent + "\n```\n\n" +
		"The conflict markers show:\n" +
		"- `<<<<<<< HEAD` - Current branch changes\n" +
		"- `=======` - Separator\n" +
		"- `>>>>>>> branch-name` - Incoming changes from base branch\n\n" +
		"Please resolve this conflict by:\n" +
		"1. Keeping the best parts of both versions\n" +
		"2. Ensuring the code is syntactically correct\n" +
		"3. Maintaining consistency with the original issue's intent\n" +
		"4. Removing all conflict markers\n\n" +
		"Respond ONLY with the resolved file content, no explanations or markdown formatting."

	requestBody := map[string]interface{}{
		"model":      c.model,
		"max_tokens": 4096,
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": prompt,
			},
		},
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", anthropicAPIURL, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", c.apiKey)
	req.Header.Set("Anthropic-Version", "2023-06-01")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling Claude API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}

	// Extract text content
	var resolvedContent string
	for _, block := range result.Content {
		if block.Type == "text" {
			resolvedContent += block.Text
		}
	}

	// Clean up the response - remove markdown code blocks if present
	resolvedContent = strings.TrimSpace(resolvedContent)
	if strings.HasPrefix(resolvedContent, "```") {
		lines := strings.Split(resolvedContent, "\n")
		if len(lines) > 2 {
			// Remove first line (```language) and last line (```)
			resolvedContent = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	return resolvedContent, nil
}

// parseChangesFromResponse extracts the JSON array from Claude's response
func parseChangesFromResponse(response string) ([]FileChange, error) {
	// Extract JSON code block if present
	start := strings.Index(response, "[")
	end := strings.LastIndex(response, "]")

	if start == -1 || end == -1 || end <= start {
		return nil, fmt.Errorf("no JSON array found in response")
	}

	jsonStr := response[start : end+1]

	var changes []FileChange
	if err := json.Unmarshal([]byte(jsonStr), &changes); err != nil {
		return nil, fmt.Errorf("unmarshaling JSON: %w", err)
	}

	return changes, nil
}
