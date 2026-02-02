# vibe-git

Autonomous development using Claude. Fetches GitHub issues, generates code with Claude AI, and creates pull requests.

## Features

- Fetch GitHub issues by number
- **Auto-watch for new issues** (webhook or poll mode)
- **@file references** - Reference specific files in issues with `@filename`
- Generate code changes using Claude AI
- Create branches, commit changes, and open PRs
- **Auto-merge PRs** after CI checks pass
- **Auto-close issues** after merging
- **Auto-resolve merge conflicts** using Claude AI
- Support for processing multiple issues at once
- **Docker deployment** with isolated key management

## Quick Start

### Option 1: Native Deployment

```bash
# Build
go build -o vibe-git .

# Run
export GITHUB_TOKEN="ghp_your_token"
export ANTHROPIC_API_KEY="sk-ant-api_your_key"

./vibe-git issue 42 --owner myorg --repo myproject
```

### Option 2: Docker Deployment (Recommended)

```bash
# 1. Configure
cp .env.example .env
# Edit .env with your API keys

# 2. Start services
make docker-up

# 3. Run
make docker-issue ISSUE=42 OWNER=myorg REPO=myproject
```

## Installation

### Native Build

```bash
go build -o vibe-git
```

### Docker Build

```bash
make docker-build
```

## Configuration

### Native Mode

Set environment variables:

```bash
export GITHUB_TOKEN="your_github_token"
export ANTHROPIC_API_KEY="your_anthropic_api_key"
```

Or pass as flags:

```bash
vibe-git issue 42 --owner myorg --repo myproject --github-token TOKEN --claude-api-key KEY
```

### Docker Mode

Create `.env` file:

```bash
PROJECT_PATH=/path/to/your/project
ANTHROPIC_API_KEY=sk-ant-api-your-key
GATEWAY_TOKEN=vibe-git-secret-token
WORKER_TOKEN=worker-secret-token
```

## Usage

### Process Issues

```bash
# Single issue
vibe-git issue 42 --owner myorg --repo myproject

# Multiple issues
vibe-git issue "42,43,44" --owner myorg --repo myproject

# Range of issues
vibe-git issue "1-5" --owner myorg --repo myproject

# With auto-merge and close
vibe-git issue 42 --owner myorg --repo myproject --auto-merge --close-issue
```

### Watch Mode

```bash
# Webhook mode (real-time)
vibe-git watch --owner myorg --repo myproject --watch-mode webhook

# Poll mode (every 5 minutes)
vibe-git watch --owner myorg --repo myproject --watch-mode poll

# With auto-merge
vibe-git watch --owner myorg --repo myproject --auto-merge --close-issue
```

## Docker Deployment

For detailed Docker deployment documentation, see [docker/README.md](docker/README.md).

### Architecture

```
┌──────────────────────────────────────────┐
│               Host Machine               │
│  ┌──────────────┐   ┌────────────────┐  │
│  │  vibe-git    │   │  Docker        │  │
│  │  (Git Token) │──▶│  ┌──────────┐  │  │
│  │              │   │  │ Gateway  │  │  │
│  └──────────────┘   │  │ (API Key)│  │  │
│                     │  └────┬─────┘  │  │
│                     │       │        │  │
│                     │  ┌────▼─────┐  │  │
│                     │  │ Worker   │  │  │
│                     │  │ (Claude) │  │  │
│                     │  └──────────┘  │  │
│                     └────────────────┘  │
└──────────────────────────────────────────┘
```

### Quick Commands

```bash
# Start services
make docker-up

# Check status
make docker-status

# Run Claude command in container
make docker-claude-version

# Process issue
make docker-issue ISSUE=42 OWNER=myorg REPO=myproject

# Watch mode
make docker-watch OWNER=myorg REPO=myproject ARGS='--auto-merge'

# Enter container shell
make docker-shell

# View logs
make docker-logs
```

## @File References

You can reference specific files in your issue descriptions using `@` mentions. This helps Claude focus on the relevant files.

Supported formats:

```
@filename.go              - Reference a file in the root
@path/to/file.go          - Reference a file in a subdirectory
@"file with spaces.go"    - Reference a file with spaces in the name
```

Example issue:

```
Title: Fix bug in user authentication

The login function in @auth/login.go is not validating passwords correctly.
Please also check @auth/utils.go for the validation helper.
```

When processing the issue, vibe-git will:
1. Extract the referenced files from the issue
2. Load their contents and include them prominently in the prompt
3. Tell Claude to pay special attention to these files

## Auto-Merge and Close

Automatically merge the created PR and close the original issue after code changes are applied.

### Basic Usage

```bash
# Process issue and auto-merge PR
vibe-git issue 42 --owner myorg --repo myproject --auto-merge

# Process issue, merge PR, and close the issue
vibe-git issue 42 --owner myorg --repo myproject --auto-merge --close-issue
```

### Options

- `--auto-merge` - Automatically merge the PR after creation
- `--close-issue` - Close the original issue after merging (requires `--auto-merge`)
- `--wait-for-checks` - Wait for CI checks to pass before merging (default: true)
- `--merge-timeout` - Maximum time to wait for checks (default: 10m)

### Watch Mode with Auto-Merge

```bash
# Automatically process, merge, and close all new issues
vibe-git watch --owner myorg --repo myproject \
  --watch-mode poll \
  --auto-merge \
  --close-issue
```

**Note:** Requires GitHub token with `repo` scope for merging and closing.

### Auto-Resolve Conflicts

When auto-merge is enabled and a merge conflict occurs, vibe-git will:

1. Detect the conflict
2. Use Claude AI to analyze and resolve the conflict
3. Push the resolved changes
4. Retry the merge

```
=== Processing Issue #42 ===
...
✓ Created PR: https://github.com/org/repo/pull/43
  Merging PR...
  ⚠ Merge conflict detected, attempting to resolve...
    Found 2 conflicted file(s): [auth.go utils.go]
    ✓ Resolved: auth.go
    ✓ Resolved: utils.go
  ✓ Conflicts resolved and committed
  Pushing resolved changes...
  Retrying merge after conflict resolution...
  ✓ PR merged successfully
  Closing issue...
  ✓ Issue closed
```

If conflict resolution fails, you'll be notified to resolve manually.

## How It Works

1. Fetches the issue details from GitHub
2. Reads the current codebase for context
3. Sends the issue and codebase to Claude AI
4. Claude generates the necessary file changes
5. Creates a new branch and applies the changes
6. Commits and pushes the changes
7. Creates a pull request linking to the issue
8. *(Optional)* Waits for CI checks to pass
9. *(Optional)* **Resolves merge conflicts if any**
10. *(Optional)* Merges the PR
11. *(Optional)* Closes the original issue

## Requirements

### Native Mode

- Go 1.21+
- Git
- GitHub Personal Access Token with `repo` scope
- Anthropic API Key

### Docker Mode

- Docker 20.10+
- Docker Compose 2.0+
- Make (optional, for convenience)

## Project Structure

```
vibe-git/
├── main.go                      # Entry point
├── cmd/                         # CLI commands
│   ├── root.go                 # Main command handling
│   └── watch.go                # Watch mode (webhook/poll)
├── internal/                    # Internal packages
│   ├── claude/client.go        # Claude API client
│   ├── ctxloader/              # Context loading (@file references)
│   ├── git/client.go           # Git operations
│   ├── github/client.go        # GitHub API client
│   └── worker/client.go        # Docker Worker client
├── docker/                      # Docker deployment
│   ├── gateway/                # Claude Gateway container
│   │   ├── Dockerfile
│   │   └── main.go             # API proxy
│   ├── worker/                 # Worker container
│   │   ├── Dockerfile
│   │   └── worker-server.go    # HTTP Git API
│   ├── scripts/
│   │   └── claude-exec.sh      # Execution helper
│   └── README.md               # Docker docs
├── docker-compose.yml           # Docker services
├── .env.example                 # Environment template
├── Makefile                     # Build automation
└── README.md                    # This file
```

## Security Considerations

1. **API Keys**:
   - Never commit API keys to git
   - Use `.env` files (already in `.gitignore`)
   - Rotate keys regularly

2. **Docker Mode**:
   - API keys are isolated in Gateway container
   - Worker container has no direct access to keys
   - Tokens protect inter-service communication

3. **GitHub Token**:
   - Use fine-grained tokens when possible
   - Minimum required scopes: `repo`
   - For auto-merge: also requires `workflow` if using required checks

## Troubleshooting

### Common Issues

**Issue**: `ANTHROPIC_API_KEY environment variable is required`
**Solution**: Set the environment variable or use `.env` file

**Issue**: `GitHub token required`
**Solution**: Export `GITHUB_TOKEN` with a valid token

**Issue**: `Worker container 'vibe-git-worker' is not running`
**Solution**: Run `make docker-up` to start services

### Debug Mode

```bash
# Enable verbose logging
export DEBUG=1
./vibe-git issue 42 --owner myorg --repo myproject
```

### Check Docker Services

```bash
# Gateway health
curl http://localhost:8080/health

# Worker health
curl -H "X-Worker-Auth: worker-secret-token" http://localhost:3000/health

# Claude status in worker
curl -H "X-Worker-Auth: worker-secret-token" http://localhost:3000/claude/status
```

## License

MIT

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
