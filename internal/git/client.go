package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"vibe-git/internal/claude"
)

// Client handles git operations
type Client struct {
	owner string
	repo  string
	token string
	dir   string
}

// NewClient creates a new git client
func NewClient(owner, repo, token string) *Client {
	return &Client{
		owner: owner,
		repo:  repo,
		token: token,
		dir:   ".",
	}
}

// SetDir sets the working directory
func (c *Client) SetDir(dir string) {
	c.dir = dir
}

// CreateBranch creates a new branch from the base branch
func (c *Client) CreateBranch(ctx context.Context, baseBranch, newBranch string) error {
	// Fetch latest changes
	if err := c.run("fetch", "origin"); err != nil {
		return fmt.Errorf("fetching: %w", err)
	}

	// Checkout base branch
	if err := c.run("checkout", baseBranch); err != nil {
		return fmt.Errorf("checking out base branch: %w", err)
	}

	// Pull latest changes
	if err := c.run("pull", "origin", baseBranch); err != nil {
		return fmt.Errorf("pulling base branch: %w", err)
	}

	// Create and checkout new branch
	if err := c.run("checkout", "-b", newBranch); err != nil {
		return fmt.Errorf("creating branch: %w", err)
	}

	return nil
}

// ApplyChanges applies file changes to the repository
func (c *Client) ApplyChanges(changes []claude.FileChange) error {
	for _, change := range changes {
		fullPath := filepath.Join(c.dir, change.Path)

		switch change.Operation {
		case "create", "modify":
			// Ensure directory exists
			dir := filepath.Dir(fullPath)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("creating directory %s: %w", dir, err)
			}

			// Write file
			if err := os.WriteFile(fullPath, []byte(change.Content), 0644); err != nil {
				return fmt.Errorf("writing file %s: %w", change.Path, err)
			}

		case "delete":
			if err := os.Remove(fullPath); err != nil {
				return fmt.Errorf("deleting file %s: %w", change.Path, err)
			}

		default:
			return fmt.Errorf("unknown operation: %s", change.Operation)
		}

		// Stage the file
		if err := c.run("add", change.Path); err != nil {
			return fmt.Errorf("staging file %s: %w", change.Path, err)
		}
	}

	return nil
}

// Commit creates a commit with the staged changes
func (c *Client) Commit(message string) error {
	// Check if there are changes to commit
	status, err := c.runOutput("status", "--porcelain")
	if err != nil {
		return fmt.Errorf("checking status: %w", err)
	}

	if strings.TrimSpace(status) == "" {
		return fmt.Errorf("no changes to commit")
	}

	// Configure git user if not set
	if err := c.configureGitUser(); err != nil {
		return err
	}

	// Commit
	if err := c.run("commit", "-m", message); err != nil {
		return fmt.Errorf("committing: %w", err)
	}

	return nil
}

// PushBranch pushes the current branch to origin
func (c *Client) PushBranch(ctx context.Context, branch string) error {
	// Set up remote URL with token for authentication
	remoteURL := fmt.Sprintf("https://%s@github.com/%s/%s.git", c.token, c.owner, c.repo)

	// Configure remote
	if err := c.run("remote", "set-url", "origin", remoteURL); err != nil {
		return fmt.Errorf("setting remote: %w", err)
	}

	// Push branch
	if err := c.run("push", "-u", "origin", branch); err != nil {
		return fmt.Errorf("pushing: %w", err)
	}

	return nil
}

// HasConflicts checks if the current branch has merge conflicts with base
func (c *Client) HasConflicts(ctx context.Context, baseBranch string) (bool, error) {
	// Fetch latest
	if err := c.run("fetch", "origin"); err != nil {
		return false, fmt.Errorf("fetching: %w", err)
	}

	// Try a test merge to detect conflicts
	if err := c.run("merge", "--no-commit", "--no-ff", "origin/"+baseBranch); err != nil {
		// Check if it's due to conflicts
		status, _ := c.runOutput("status", "--porcelain")
		if strings.Contains(status, "UU") || strings.Contains(status, "AA") ||
			strings.Contains(status, "DD") || strings.Contains(status, "AU") ||
			strings.Contains(status, "UA") || strings.Contains(status, "DU") ||
			strings.Contains(status, "UD") {
			// Abort the merge attempt
			c.run("merge", "--abort")
			return true, nil
		}
	}

	// Abort the test merge
	c.run("merge", "--abort")
	return false, nil
}

// ResolveConflicts pulls latest base branch and resolves conflicts
func (c *Client) ResolveConflicts(ctx context.Context, baseBranch string, issueTitle string, resolveFn ConflictResolver) error {
	fmt.Println("  Detected merge conflicts, attempting to resolve...")

	// Fetch latest
	if err := c.run("fetch", "origin"); err != nil {
		return fmt.Errorf("fetching: %w", err)
	}

	// Attempt to merge base branch
	if err := c.run("merge", "origin/"+baseBranch); err != nil {
		// Check if there are actual conflicts
		conflictFiles, err := c.getConflictFiles()
		if err != nil {
			return fmt.Errorf("getting conflict files: %w", err)
		}

		if len(conflictFiles) == 0 {
			// No conflicts, merge succeeded or other error
			return nil
		}

		fmt.Printf("  Found %d conflicted file(s): %v\n", len(conflictFiles), conflictFiles)

		// Resolve each conflicted file
		for _, file := range conflictFiles {
			if err := c.resolveFileConflict(file, issueTitle, resolveFn); err != nil {
				return fmt.Errorf("resolving conflict in %s: %w", file, err)
			}
		}

		// Complete the merge
		if err := c.Commit("Resolve merge conflicts\n\n" + issueTitle); err != nil {
			return fmt.Errorf("committing resolved conflicts: %w", err)
		}

		fmt.Println("  ✓ Conflicts resolved and committed")
	}

	return nil
}

// getConflictFiles returns list of files with merge conflicts
func (c *Client) getConflictFiles() ([]string, error) {
	status, err := c.runOutput("status", "--porcelain")
	if err != nil {
		return nil, err
	}

	var files []string
	lines := strings.Split(status, "\n")
	for _, line := range lines {
		if len(line) < 3 {
			continue
		}
		// Check for conflict markers in status
		// UU = both modified, AA = both added, etc.
		code := line[:2]
		if code == "UU" || code == "AA" || code == "DD" ||
			code == "AU" || code == "UA" || code == "DU" || code == "UD" {
			files = append(files, strings.TrimSpace(line[3:]))
		}
	}

	return files, nil
}

// ConflictResolver is a function that resolves a conflict given the conflicted content
type ConflictResolver func(filePath string, conflictContent string, issueTitle string) (string, error)

// resolveFileConflict resolves a single file conflict
func (c *Client) resolveFileConflict(file string, issueTitle string, resolveFn ConflictResolver) error {
	// Read the conflicted file
	content, err := os.ReadFile(filepath.Join(c.dir, file))
	if err != nil {
		return fmt.Errorf("reading conflicted file: %w", err)
	}

	// Use the resolver function to resolve
	resolved, err := resolveFn(file, string(content), issueTitle)
	if err != nil {
		return fmt.Errorf("conflict resolution failed: %w", err)
	}

	// Write resolved content
	if err := os.WriteFile(filepath.Join(c.dir, file), []byte(resolved), 0644); err != nil {
		return fmt.Errorf("writing resolved file: %w", err)
	}

	// Stage the resolved file
	if err := c.run("add", file); err != nil {
		return fmt.Errorf("staging resolved file: %w", err)
	}

	fmt.Printf("    ✓ Resolved: %s\n", file)
	return nil
}

// ForcePushWithLease pushes with force-with-lease (safer force push)
func (c *Client) ForcePushWithLease(ctx context.Context, branch string) error {
	remoteURL := fmt.Sprintf("https://%s@github.com/%s/%s.git", c.token, c.owner, c.repo)

	if err := c.run("remote", "set-url", "origin", remoteURL); err != nil {
		return fmt.Errorf("setting remote: %w", err)
	}

	if err := c.run("push", "--force-with-lease", "-u", "origin", branch); err != nil {
		return fmt.Errorf("force pushing: %w", err)
	}

	return nil
}

// configureGitUser sets up git user config for commits
func (c *Client) configureGitUser() error {
	// Check if user.name is set
	name, _ := c.runOutput("config", "user.name")
	if strings.TrimSpace(name) == "" {
		if err := c.run("config", "user.name", "Vibe Git"); err != nil {
			return fmt.Errorf("setting git user.name: %w", err)
		}
	}

	// Check if user.email is set
	email, _ := c.runOutput("config", "user.email")
	if strings.TrimSpace(email) == "" {
		if err := c.run("config", "user.email", "vibe-git@localhost"); err != nil {
			return fmt.Errorf("setting git user.email: %w", err)
		}
	}

	return nil
}

// run executes a git command
func (c *Client) run(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = c.dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runOutput executes a git command and returns the output
func (c *Client) runOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = c.dir
	output, err := cmd.Output()
	return string(output), err
}
