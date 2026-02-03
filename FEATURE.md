# Vibe-Git Feature Document

## Project Overview

Vibe-Git is an AI-powered autonomous development tool that automatically processes GitHub issues, generates code changes using Claude AI, and creates pull requests. It bridges the gap between requirements and implementation through intelligent automation.

## Core Features

### 1. Issue Processing

#### 1.1 Single Issue Processing
- Fetch GitHub issue details by issue number
- Parse issue title and description
- Support multiple issues (comma-separated: "1,2,3")
- Support issue ranges (e.g., "1-5" processes issues 1 through 5)

#### 1.2 @File References
- Extract @filename references from issue text
- Supported formats: `@filename`, `@path/to/file`, `@"file with spaces"`
- Automatically load referenced file contents and pass to Claude
- Highlight referenced files in the prompt

### 2. AI Code Generation

#### 2.1 Code Change Generation
- Call Claude API to analyze issues and codebase
- Generate JSON-formatted file change lists
- Support three operation types: create, modify, delete
- Return complete file content (not diffs)

#### 2.2 Prompt Building
- Build prompts containing issue information
- Automatically collect codebase context
- Special highlighting of @referenced files
- Provide code generation guidelines and format requirements

#### 2.3 Conflict Resolution
- Automatically detect merge conflicts
- Use Claude AI to analyze conflict content
- Intelligently merge the best parts from both sides
- Remove all conflict markers

### 3. Git Operations

#### 3.1 Branch Management
- Create new branches from base branch
- Branch naming format: `vibe-git/issue-{number}`
- Automatically fetch latest code

#### 3.2 Change Application
- Create/modify/delete files based on AI-generated changes
- Automatically create necessary directory structures
- Stage all changed files

#### 3.3 Commit and Push
- Auto-generate commit messages (include issue number and title)
- Configure Git user info if not set
- Push to remote repository

#### 3.4 Conflict Handling
- Detect merge conflicts with base branch
- Get list of conflicted files
- Call AI to resolve conflicts in each file
- Use force-with-lease for safe force push

### 4. GitHub Integration

#### 4.1 Issue Fetching
- Get issue details via GitHub API
- Extract title, description, state, labels

#### 4.2 Pull Request Creation
- Auto-create PR linked to issue
- PR title format: `Fix #{number}: {issue_title}`
- PR description includes `Closes #{number}` for auto-closing

#### 4.3 Auto-Merge
- Wait for CI checks to pass before auto-merging
- Configurable merge timeout (default 10 minutes)
- Use squash merge strategy
- Auto-close original issue after merge (when --close-issue flag is set)

#### 4.4 Conflict Detection and Retry
- Detect if PR is mergeable
- Auto-trigger conflict resolution on conflicts
- Retry merge after resolution

### 5. Watch Mode

#### 5.1 Webhook Mode
- Start HTTP server to listen for GitHub webhooks
- Default port 8080 (configurable)
- Handle `issues.opened` events
- Process new issues asynchronously in background

#### 5.2 Poll Mode
- Periodically check for new issues
- Default interval 5 minutes (configurable)
- Use `.vibe-git-state` file to track last check time
- Check from 24 hours ago on first run

#### 5.3 Health Check
- Provide `/health` endpoint in webhook mode
- Return service status information

### 6. HTTP Request Tool

#### 6.1 Basic Request Features
- Support GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS methods
- Custom request headers
- Query parameter support
- Request body support (string or file)

#### 6.2 Response Handling
- Display response status code
- Support JSON formatted output
- Optional inclusion of response headers
- Support saving to file

### 7. Configuration Management

#### 7.1 Environment Variables
- `GITHUB_TOKEN` - GitHub personal access token
- `ANTHROPIC_API_KEY` - Anthropic API key
- `ANTHROPIC_BASE_URL` - Custom Claude API base URL
- `VIBE_GIT_POLL_INTERVAL` - Default poll interval

#### 7.2 Configuration File Support
- Load config from `~/.claude/settings.json`
- Support `ANTHROPIC_AUTH_TOKEN` and `ANTHROPIC_API_KEY`
- Support custom base URL

#### 7.3 Command Line Flags
- `--github-token` - GitHub token
- `--claude-api-key` - Claude API key
- `--owner` - Repository owner
- `--repo` - Repository name
- `--base` - Base branch (default: main)
- `--model` - Claude model (default: claude-3-5-sonnet-latest)
- `--auto-merge` - Auto-merge PR
- `--close-issue` - Close issue after merge
- `--wait-for-checks` - Wait for CI checks (default: true)
- `--merge-timeout` - Merge timeout (default: 10m)
- `--watch-mode` - Watch mode (webhook/poll)
- `--webhook-port` - Webhook port (default: 8080)
- `--poll-interval` - Poll interval (default: 5m)

## Non-Functional Requirements

### 1. Performance
- Issue processing timeout: 5 minutes
- HTTP request timeout: 30 seconds
- Codebase file size limit: 100KB (skip oversized files)
- Poll API timeout: 30 seconds

### 2. Security
- API keys not committed to Git
- Use `.env` file for sensitive information
- API keys isolated in Gateway container in Docker mode
- Path traversal protection for file operations
- Use force-with-lease for safe force push

### 3. Code Quality
- Follow Go 1.21+ standards
- Use standard library with minimal external dependencies
- Comprehensive error handling with meaningful messages
- Clear logging output showing processing progress

### 4. Compatibility
- Support GitHub personal access tokens (repo scope)
- Support Anthropic Claude API
- Compatible with standard Git repositories
- Support Docker deployment

## Deployment Methods

### 1. Local Deployment
- Compile to single binary
- Dependencies: Go 1.21+, Git, GitHub Token, Claude API Key

### 2. Docker Deployment
- Gateway container: Protects Anthropic API key, provides proxy service
- Worker container: Isolated execution of Claude commands and Git operations
- Token authentication for inter-service communication

## Project Structure

```
vibe-git/
├── main.go                      # Entry point
├── cmd/                         # CLI commands
│   ├── root.go                 # Main command handling
│   ├── watch.go                # Watch mode
│   └── request.go              # HTTP request utility
├── internal/                    # Internal packages
│   ├── claude/client.go        # Claude API client
│   ├── ctxloader/loader.go     # @file reference handling
│   ├── git/client.go           # Git operations
│   ├── github/client.go        # GitHub API client
│   ├── config/loader.go        # Configuration loading
│   └── worker/client.go        # Docker Worker client
└── docker/                      # Docker deployment files
```

## Use Cases

1. **Automated Bug Fixes** - Developer creates issue describing bug, system auto-generates fix PR
2. **Feature Development** - Auto-generate implementation code from detailed feature description issues
3. **Code Refactoring** - Describe refactoring needs, AI auto-executes and creates PR
4. **Continuous Monitoring** - Webhook or poll mode auto-processes newly submitted issues
5. **Batch Processing** - Process multiple related issues or historical issues at once
