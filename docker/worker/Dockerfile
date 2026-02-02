FROM node:20-slim

# Install dependencies
RUN apt-get update && apt-get install -y \
    git \
    curl \
    wget \
    vim \
    nano \
    build-essential \
    python3 \
    python3-pip \
    golang-go \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Install Claude Code globally
RUN npm install -g @anthropic-ai/claude-code

# Install claude-code-guide globally
RUN npm install -g claude-code-guide || true

# Create workspace
WORKDIR /workspace
RUN mkdir -p /workspace/project /app

# Copy worker server
COPY worker-server.go /app/
COPY go.mod /app/ 2>/dev/null || echo "module worker" > /app/go.mod

# Build worker server
WORKDIR /app
RUN go mod tidy 2>/dev/null || true
RUN go build -o worker-server worker-server.go

# Create non-root user for security
RUN useradd -m -s /bin/bash claude && \
    chown -R claude:claude /workspace

# Claude Code configuration
ENV CLAUDE_CONFIG_DIR=/root/.claude
ENV CLAUDE_API_URL=http://claude-gateway:8080/v1
ENV CLAUDE_API_KEY=local-mode

# Set up git defaults
RUN git config --global user.email "vibe-git@docker.local" && \
    git config --global user.name "Vibe Git Worker" && \
    git config --global init.defaultBranch main

EXPOSE 3000

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:3000/health || exit 1

CMD ["/app/worker-server"]
