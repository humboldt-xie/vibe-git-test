package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"vibe-git/internal/claude"
	"vibe-git/internal/config"
	"vibe-git/internal/ctxloader"
	"vibe-git/internal/git"
	"vibe-git/internal/github"
)

var (
	githubToken    string
	claudeAPIKey   string
	repoOwner      string
	repoName       string
	baseBranch     string
	model          string
	autoMerge      bool
	closeIssue     bool
	waitForChecks  bool
	mergeTimeout   time.Duration
)

func init() {
	// Default values from environment
	githubToken = os.Getenv("GITHUB_TOKEN")
	claudeAPIKey = os.Getenv("ANTHROPIC_API_KEY")

	// Load defaults from ~/.claude/settings.json if env not set
	if claudeAPIKey == "" {
		cfg := config.LoadFromClaudeSettings()
		claudeAPIKey = cfg.AnthropicAPIKey
		if cfg.AnthropicBaseURL != "" {
			os.Setenv("ANTHROPIC_BASE_URL", cfg.AnthropicBaseURL)
		}
	}

	// Default poll interval from environment
	if envPollInterval := os.Getenv("VIBE_GIT_POLL_INTERVAL"); envPollInterval != "" {
		if d, err := time.ParseDuration(envPollInterval); err == nil {
			pollInterval = d
		}
	}
}

// Execute runs the CLI
func Execute() error {
	// Define flags
	flag.StringVar(&githubToken, "github-token", githubToken, "GitHub personal access token")
	flag.StringVar(&claudeAPIKey, "claude-api-key", claudeAPIKey, "Anthropic API key")
	flag.StringVar(&repoOwner, "owner", "", "GitHub repository owner")
	flag.StringVar(&repoName, "repo", "", "GitHub repository name")
	flag.StringVar(&baseBranch, "base", "main", "Base branch")
	flag.StringVar(&model, "model", "claude-3-5-sonnet-latest", "Claude model")

	// Watch mode flags
	flag.StringVar(&watchMode, "watch-mode", "webhook", "Watch mode: webhook or poll")
	flag.IntVar(&webhookPort, "webhook-port", 8080, "Webhook server port")
	pollIntervalStr := flag.String("poll-interval", pollInterval.String(), "Poll interval (e.g., 1m, 5m, 1h)")

	// Auto-merge flags
	flag.BoolVar(&autoMerge, "auto-merge", false, "Automatically merge PR after creation")
	flag.BoolVar(&closeIssue, "close-issue", false, "Close issue after merging PR")
	flag.BoolVar(&waitForChecks, "wait-for-checks", true, "Wait for CI checks before merging")
	mergeTimeoutStr := flag.String("merge-timeout", "10m", "Timeout for waiting to merge")

	flag.Parse()

	// Parse poll interval
	var err error
	pollInterval, err = time.ParseDuration(*pollIntervalStr)
	if err != nil {
		return fmt.Errorf("invalid poll interval: %w", err)
	}

	// Parse merge timeout
	mergeTimeout, err = time.ParseDuration(*mergeTimeoutStr)
	if err != nil {
		return fmt.Errorf("invalid merge timeout: %w", err)
	}

	if flag.NArg() < 1 {
		printUsage()
		return fmt.Errorf("no command specified")
	}

	command := flag.Arg(0)

	switch command {
	case "issue":
		if flag.NArg() < 2 {
			printUsage()
			return fmt.Errorf("issue number required")
		}
		return runIssue(flag.Arg(1))
	case "watch":
		return runWatch()
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command: %s", command)
	}
}

func printUsage() {
	fmt.Println(`vibe-git - Autonomous development using Claude

Usage:
  vibe-git issue <issue-numbers> [flags]
  vibe-git watch [flags]

Commands:
  issue    Process GitHub issues and create PRs with Claude-generated code
  watch    Automatically watch for new issues and process them

Flags:`)
	flag.PrintDefaults()
	fmt.Println(`
Examples:
  # Process specific issues
  vibe-git issue 42 --owner myorg --repo myproject
  vibe-git issue "1,2,3" --owner myorg --repo myproject

  # Auto-merge PR and close issue after processing
  vibe-git issue 42 --owner myorg --repo myproject --auto-merge --close-issue

  # Watch mode - Webhook (real-time)
  vibe-git watch --owner myorg --repo myproject --watch-mode webhook --webhook-port 8080

  # Watch mode - Poll (check every 5 minutes)
  vibe-git watch --owner myorg --repo myproject --watch-mode poll --poll-interval 5m

  # Watch with auto-merge (CI must pass first)
  vibe-git watch --owner myorg --repo myproject --auto-merge --close-issue

Environment Variables:
  GITHUB_TOKEN           GitHub personal access token
  ANTHROPIC_API_KEY      Anthropic API key
  VIBE_GIT_POLL_INTERVAL Default poll interval (e.g., 1m, 5m, 1h)`)
}

func runIssue(issueArg string) error {
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

	// Parse issue numbers
	issueNums, err := parseIssueNumbers(issueArg)
	if err != nil {
		return err
	}

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nReceived interrupt, shutting down...")
		cancel()
	}()

	// Initialize clients
	githubClient := github.NewClient(githubToken, repoOwner, repoName)
	claudeClient := claude.NewClient(claudeAPIKey, os.Getenv("ANTHROPIC_BASE_URL"), model)
	gitClient := git.NewClient(repoOwner, repoName, githubToken)

	// Process each issue
	for _, issueNum := range issueNums {
		if err := processIssue(ctx, githubClient, claudeClient, gitClient, issueNum); err != nil {
			fmt.Fprintf(os.Stderr, "Error processing issue #%d: %v\n", issueNum, err)
			continue
		}
	}

	return nil
}

func parseIssueNumbers(arg string) ([]int, error) {
	var numbers []int
	parts := strings.Split(arg, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Handle range like "1-5"
		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				return nil, fmt.Errorf("invalid range: %s", part)
			}
			start, err := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
			if err != nil {
				return nil, fmt.Errorf("invalid issue number: %s", rangeParts[0])
			}
			end, err := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid issue number: %s", rangeParts[1])
			}
			for i := start; i <= end; i++ {
				numbers = append(numbers, i)
			}
		} else {
			num, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid issue number: %s", part)
			}
			numbers = append(numbers, num)
		}
	}

	if len(numbers) == 0 {
		return nil, fmt.Errorf("no valid issue numbers provided")
	}

	return numbers, nil
}

func processIssue(ctx context.Context, gh *github.Client, cl *claude.Client, git *git.Client, issueNum int) error {
	fmt.Printf("\n=== Processing Issue #%d ===\n", issueNum)

	// Fetch issue details
	issue, err := gh.GetIssue(ctx, issueNum)
	if err != nil {
		return fmt.Errorf("fetching issue: %w", err)
	}

	fmt.Printf("Title: %s\n", issue.Title)
	fmt.Printf("URL: %s\n", issue.URL)

	// Extract @file references from issue
	refs := ctxloader.ExtractFileReferences(issue.Title + "\n" + issue.Body)
	if len(refs) > 0 {
		fmt.Printf("Found @references: %v\n", refs)
	}

	// Load referenced files
	referencedFiles := ctxloader.LoadReferencedFiles(refs, ".")
	for _, f := range referencedFiles {
		if f.Found {
			fmt.Printf("✓ Loaded referenced file: %s\n", f.Path)
		} else {
			fmt.Printf("⚠ File not found: %s\n", f.Path)
		}
	}

	// Create branch
	branchName := fmt.Sprintf("vibe-git/issue-%d", issueNum)
	fmt.Printf("Creating branch: %s\n", branchName)

	if err := git.CreateBranch(ctx, baseBranch, branchName); err != nil {
		return fmt.Errorf("creating branch: %w", err)
	}

	// Generate code with Claude, passing referenced files
	fmt.Println("Generating code with Claude...")
	changes, err := cl.GenerateCode(ctx, issue.Title, issue.Body, referencedFiles)
	if err != nil {
		return fmt.Errorf("generating code: %w", err)
	}

	// Apply changes
	fmt.Printf("Applying %d file changes...\n", len(changes))
	if err := git.ApplyChanges(changes); err != nil {
		return fmt.Errorf("applying changes: %w", err)
	}

	// Commit changes
	commitMsg := fmt.Sprintf("Fix issue #%d: %s\n\n%s", issueNum, issue.Title, issue.URL)
	if err := git.Commit(commitMsg); err != nil {
		return fmt.Errorf("committing changes: %w", err)
	}

	// Push branch
	fmt.Printf("Pushing branch %s...\n", branchName)
	if err := git.PushBranch(ctx, branchName); err != nil {
		return fmt.Errorf("pushing branch: %w", err)
	}

	// Create PR
	prTitle := fmt.Sprintf("Fix #%d: %s", issueNum, issue.Title)
	prBody := fmt.Sprintf("Closes #%d\n\n%s", issueNum, issue.URL)

	prNumber, prURL, err := gh.CreatePullRequestWithNumber(ctx, baseBranch, branchName, prTitle, prBody)
	if err != nil {
		return fmt.Errorf("creating PR: %w", err)
	}

	fmt.Printf("✓ Created PR: %s\n", prURL)

	// Close issue if enabled (PR description has "Closes #X" which auto-closes on merge,
	// but we also support explicit closing)
	if closeIssue {
		fmt.Println("  Closing issue...")
		if err := gh.CloseIssue(ctx, issueNum); err != nil {
			fmt.Fprintf(os.Stderr, "  ⚠ Failed to close issue: %v\n", err)
		} else {
			fmt.Println("  ✓ Issue closed")
		}
	}

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
		mergeMsg := fmt.Sprintf("Auto-merged by vibe-git\n\nFixes #%d", issueNum)

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
	}

	return nil
}

// isConflictError checks if the error is due to merge conflicts
func isConflictError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "conflict") ||
		strings.Contains(errStr, "not mergeable")
}
