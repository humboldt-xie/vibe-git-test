package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"vibe-git/internal/claude"
	"vibe-git/internal/ctxloader"
	"vibe-git/internal/git"
	"vibe-git/internal/github"
)

var (
	watchMode   string // "webhook" or "poll"
	webhookPort int
	pollInterval time.Duration
	lastChecked time.Time
)

func init() {
	// Will be set by flags in Execute
}

// runWatch starts watching for new issues
func runWatch() error {
	// Validate flags
	if githubToken == "" {
		return fmt.Errorf("GitHub token required (use --github-token or GITHUB_TOKEN env)")
	}
	if claudeAPIKey == "" {
		return fmt.Errorf("Claude API key required (use --claude-api-key or ANTHROPIC_API_KEY env)")
	}
	if repoOwner == "" || repoName == "" {
		return fmt.Errorf("repository owner and name required (use --owner and --repo)")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nShutting down...")
		cancel()
	}()

	// Initialize clients
	githubClient := github.NewClient(githubToken, repoOwner, repoName)
	claudeClient := claude.NewClient(claudeAPIKey, model)
	gitClient := git.NewClient(repoOwner, repoName, githubToken)

	switch watchMode {
	case "webhook":
		return runWebhookServer(ctx, githubClient, claudeClient, gitClient)
	case "poll":
		return runPollMode(ctx, githubClient, claudeClient, gitClient)
	default:
		return fmt.Errorf("unknown watch mode: %s (use 'webhook' or 'poll')", watchMode)
	}
}

// ========== Webhook Mode ==========

type webhookPayload struct {
	Action string `json:"action"`
	Issue  struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Body   string `json:"body"`
		State  string `json:"state"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
		HTMLURL string `json:"html_url"`
	} `json:"issue"`
}

func runWebhookServer(ctx context.Context, gh *github.Client, cl *claude.Client, git *git.Client) error {
	http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var payload webhookPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Only process opened issues
		if payload.Action != "opened" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Skip if issue is closed
		if payload.Issue.State != "open" {
			w.WriteHeader(http.StatusOK)
			return
		}

		fmt.Printf("\n📥 New issue received: #%d - %s\n", payload.Issue.Number, payload.Issue.Title)

		// Process in background
		go func() {
			issue := &github.Issue{
				Number: payload.Issue.Number,
				Title:  payload.Issue.Title,
				Body:   payload.Issue.Body,
				URL:    payload.Issue.HTMLURL,
				State:  payload.Issue.State,
			}
			for _, l := range payload.Issue.Labels {
				issue.Labels = append(issue.Labels, l.Name)
			}

			if err := processIssueWithClients(gh, cl, git, issue); err != nil {
				fmt.Fprintf(os.Stderr, "Error processing issue #%d: %v\n", payload.Issue.Number, err)
			}
		}()

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", webhookPort),
		Handler: nil,
	}

	fmt.Printf("🚀 Webhook server starting on port %d\n", webhookPort)
	fmt.Printf("📋 Configure GitHub webhook to: http://your-server:%d/webhook\n", webhookPort)
	fmt.Println("✓ Waiting for new issues...")

	// Start server in goroutine
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	return server.Shutdown(shutdownCtx)
}

// ========== Poll Mode ==========

func runPollMode(ctx context.Context, gh *github.Client, cl *claude.Client, git *git.Client) error {
	fmt.Printf("🔄 Poll mode started (interval: %v)\n", pollInterval)
	fmt.Println("✓ Checking for new issues...")

	// Load last checked time from file if exists
	loadLastCheckedTime()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	// Check immediately on start
	checkAndProcessIssues(gh, cl, git)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			checkAndProcessIssues(gh, cl, git)
		}
	}
}

func checkAndProcessIssues(gh *github.Client, cl *claude.Client, git *git.Client) {
	fmt.Printf("\n[%s] Checking for new issues...\n", time.Now().Format("2006-01-02 15:04:05"))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get recent issues
	issues, err := gh.ListRecentIssues(ctx, lastChecked)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching issues: %v\n", err)
		return
	}

	if len(issues) == 0 {
		fmt.Println("  No new issues found")
		return
	}

	fmt.Printf("  Found %d new issue(s)\n", len(issues))

	for _, issue := range issues {
		if issue.State != "open" {
			continue
		}

		fmt.Printf("\n📥 Processing issue #%d: %s\n", issue.Number, issue.Title)

		if err := processIssueWithClients(gh, cl, git, issue); err != nil {
			fmt.Fprintf(os.Stderr, "Error processing issue #%d: %v\n", issue.Number, err)
			continue
		}
	}

	// Update last checked time
	lastChecked = time.Now()
	saveLastCheckedTime()
}

// ========== Shared Processing ==========

func processIssueWithClients(gh *github.Client, cl *claude.Client, git *git.Client, issue *github.Issue) error {
	// Extract @file references from issue
	refs := ctxloader.ExtractFileReferences(issue.Title + "\n" + issue.Body)
	if len(refs) > 0 {
		fmt.Printf("  Found @references: %v\n", refs)
	}

	// Load referenced files
	referencedFiles := ctxloader.LoadReferencedFiles(refs, ".")
	for _, f := range referencedFiles {
		if f.Found {
			fmt.Printf("  ✓ Loaded referenced file: %s\n", f.Path)
		} else {
			fmt.Printf("  ⚠ File not found: %s\n", f.Path)
		}
	}

	// Create branch
	branchName := fmt.Sprintf("vibe-git/issue-%d", issue.Number)
	fmt.Printf("  Creating branch: %s\n", branchName)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := git.CreateBranch(ctx, baseBranch, branchName); err != nil {
		return fmt.Errorf("creating branch: %w", err)
	}

	// Generate code with Claude, passing referenced files
	fmt.Println("  Generating code with Claude...")
	changes, err := cl.GenerateCode(ctx, issue.Title, issue.Body, referencedFiles)
	if err != nil {
		return fmt.Errorf("generating code: %w", err)
	}

	// Apply changes
	fmt.Printf("  Applying %d file changes...\n", len(changes))
	if err := git.ApplyChanges(changes); err != nil {
		return fmt.Errorf("applying changes: %w", err)
	}

	// Commit changes
	commitMsg := fmt.Sprintf("Fix issue #%d: %s\n\n%s", issue.Number, issue.Title, issue.URL)
	if err := git.Commit(commitMsg); err != nil {
		return fmt.Errorf("committing changes: %w", err)
	}

	// Push branch
	fmt.Printf("  Pushing branch...\n")
	if err := git.PushBranch(ctx, branchName); err != nil {
		return fmt.Errorf("pushing branch: %w", err)
	}

	// Create PR
	prTitle := fmt.Sprintf("Fix #%d: %s", issue.Number, issue.Title)
	prBody := fmt.Sprintf("Closes #%d\n\n%s", issue.Number, issue.URL)

	prNumber, prURL, err := gh.CreatePullRequestWithNumber(ctx, baseBranch, branchName, prTitle, prBody)
	if err != nil {
		return fmt.Errorf("creating PR: %w", err)
	}

	fmt.Printf("  ✓ Created PR: %s\n", prURL)

	// Auto-merge if enabled
	if autoMerge {
		if waitForChecks {
			fmt.Printf("  Waiting for CI checks to pass (timeout: %v)...\n", mergeTimeout)
			if err := gh.WaitForMergeable(ctx, prNumber, mergeTimeout); err != nil {
				fmt.Fprintf(os.Stderr, "  ⚠ Failed to wait for checks: %v\n", err)
				fmt.Println("  You can merge manually later")
				return nil
			}
		}

		fmt.Println("  Merging PR...")
		mergeTitle := fmt.Sprintf("Merge: %s", prTitle)
		mergeMsg := fmt.Sprintf("Auto-merged by vibe-git\n\nFixes #%d", issue.Number)

		if err := gh.MergePullRequest(ctx, prNumber, mergeTitle, mergeMsg); err != nil {
			// Check if it's a conflict
			if isConflictError(err) {
				fmt.Println("  ⚠ Merge conflict detected, attempting to resolve...")

				// Resolve conflicts
				if err := git.ResolveConflicts(ctx, baseBranch, issue.Title, func(filePath, conflictContent, issueTitle string) (string, error) {
					return cl.ResolveConflict(ctx, filePath, conflictContent, issueTitle)
				}); err != nil {
					fmt.Fprintf(os.Stderr, "  ⚠ Failed to resolve conflicts: %v\n", err)
					fmt.Println("  You need to resolve conflicts manually")
					return nil
				}

				// Push resolved changes
				fmt.Println("  Pushing resolved changes...")
				if err := git.ForcePushWithLease(ctx, branchName); err != nil {
					fmt.Fprintf(os.Stderr, "  ⚠ Failed to push resolved changes: %v\n", err)
					return nil
				}

				// Wait a moment for GitHub to update PR status
				time.Sleep(2 * time.Second)

				// Retry merge
				fmt.Println("  Retrying merge after conflict resolution...")
				if err := gh.MergePullRequest(ctx, prNumber, mergeTitle, mergeMsg); err != nil {
					fmt.Fprintf(os.Stderr, "  ⚠ Failed to merge PR after conflict resolution: %v\n", err)
					fmt.Println("  You can merge manually later")
					return nil
				}
			} else {
				fmt.Fprintf(os.Stderr, "  ⚠ Failed to merge PR: %v\n", err)
				fmt.Println("  You can merge manually later")
				return nil
			}
		}
		fmt.Println("  ✓ PR merged successfully")

		// Close issue if enabled
		if closeIssue {
			fmt.Println("  Closing issue...")
			if err := gh.CloseIssue(ctx, issue.Number); err != nil {
				fmt.Fprintf(os.Stderr, "  ⚠ Failed to close issue: %v\n", err)
			} else {
				fmt.Println("  ✓ Issue closed")
			}
		}
	}

	return nil
}

// ========== State Persistence ==========

const stateFile = ".vibe-git-state"

func loadLastCheckedTime() {
	data, err := os.ReadFile(stateFile)
	if err != nil {
		lastChecked = time.Now().Add(-24 * time.Hour) // Default to 24 hours ago
		return
	}

	ts, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		lastChecked = time.Now().Add(-24 * time.Hour)
		return
	}

	lastChecked = time.Unix(ts, 0)
}

func saveLastCheckedTime() {
	ts := strconv.FormatInt(lastChecked.Unix(), 10)
	os.WriteFile(stateFile, []byte(ts), 0644)
}
