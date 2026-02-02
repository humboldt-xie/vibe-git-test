#!/bin/bash
# ===========================================
# Claude Worker 执行脚本
# 在 Host 机器上运行，通过 docker exec 调用工作容器中的 Claude
# ===========================================

set -e

# 配置
WORKER_CONTAINER="vibe-git-worker"
GATEWAY_URL="http://localhost:8080"
WORKER_URL="http://localhost:3000"
WORKER_TOKEN="${WORKER_TOKEN:-worker-secret-token}"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 检查容器状态
check_containers() {
    if ! docker ps --format "{{.Names}}" | grep -q "^${WORKER_CONTAINER}$"; then
        echo -e "${RED}Error: Worker container '${WORKER_CONTAINER}' is not running${NC}"
        echo "Run: docker-compose up -d"
        exit 1
    fi

    # 检查 Gateway
    if ! curl -sf "${GATEWAY_URL}/health" > /dev/null 2>&1; then
        echo -e "${RED}Error: Claude Gateway is not accessible${NC}"
        exit 1
    fi
}

# 在容器中运行 Claude 命令
run_claude() {
    local cmd="$1"
    shift
    local args="$@"

    echo -e "${YELLOW}Running: claude ${cmd} ${args}${NC}"

    # 通过 docker exec 执行 claude
    docker exec -it "${WORKER_CONTAINER}" \
        claude "${cmd}" ${args}
}

# 通过 HTTP API 在工作容器中运行 Claude（非交互式）
run_claude_api() {
    local cmd="$1"
    shift
    local args=("$@")

    # 构建 JSON 请求
    local json_body
    json_body=$(jq -n \
        --arg cmd "$cmd" \
        --argjson args "$(printf '%s\n' "${args[@]}" | jq -R . | jq -s .)" \
        '{command: $cmd, args: $args, timeout: 300}')

    echo -e "${YELLOW}Running via API: claude ${cmd} ${args[*]}${NC}"

    # 调用 Worker HTTP API
    curl -s -X POST \
        -H "Content-Type: application/json" \
        -H "X-Worker-Auth: ${WORKER_TOKEN}" \
        -d "$json_body" \
        "${WORKER_URL}/claude/run" | jq -r '.'
}

# 获取 Worker 状态
status() {
    echo -e "${GREEN}=== Claude Worker Status ===${NC}"

    # 检查容器
    echo "Container Status:"
    docker ps --filter "name=${WORKER_CONTAINER}" --format "  {{.Names}}: {{.Status}}"

    # 检查 Gateway
    echo -e "\nGateway Health:"
    curl -s "${GATEWAY_URL}/health" | jq -r '.' 2>/dev/null || echo "  Gateway: Unreachable"

    # 检查 Worker
    echo -e "\nWorker Health:"
    curl -s -H "X-Worker-Auth: ${WORKER_TOKEN}" "${WORKER_URL}/health" | jq -r '.' 2>/dev/null || echo "  Worker: Unreachable"

    # 检查 Claude 状态
    echo -e "\nClaude Status:"
    curl -s -H "X-Worker-Auth: ${WORKER_TOKEN}" "${WORKER_URL}/claude/status" | jq -r '.' 2>/dev/null || echo "  Claude: Not available"
}

# 查看项目 Git 状态
git_status() {
    echo -e "${GREEN}=== Git Status ===${NC}"
    curl -s -H "X-Worker-Auth: ${WORKER_TOKEN}" "${WORKER_URL}/git/status" | jq -r '.output'
}

# 查看项目信息
project_info() {
    echo -e "${GREEN}=== Project Info ===${NC}"
    curl -s -H "X-Worker-Auth: ${WORKER_TOKEN}" "${WORKER_URL}/project/info" | jq -r '.'
}

# 读取文件
file_read() {
    local path="$1"
    curl -s -X POST \
        -H "Content-Type: application/json" \
        -H "X-Worker-Auth: ${WORKER_TOKEN}" \
        -d "{\"path\": \"$path\"}" \
        "${WORKER_URL}/file/read" | jq -r '.content'
}

# 写入文件
file_write() {
    local path="$1"
    local content="$2"

    curl -s -X POST \
        -H "Content-Type: application/json" \
        -H "X-Worker-Auth: ${WORKER_TOKEN}" \
        -d "{\"path\": \"$path\", \"content\": $(echo "$content" | jq -Rs .)}" \
        "${WORKER_URL}/file/write" | jq -r '.'
}

# 使用说明
usage() {
    cat << EOF
Claude Worker 执行脚本

Usage:
    $0 <command> [options]

Commands:
    run <cmd> [args]     在容器中运行 Claude 命令（交互式）
    api <cmd> [args]     通过 HTTP API 运行 Claude（非交互式）
    status               查看容器和 Claude 状态
    git-status           查看项目 Git 状态
    project-info         查看项目信息
    read <path>          读取项目中的文件
    write <path>         写入文件（从 stdin 读取内容）
    shell                进入工作容器 shell
    logs                 查看工作容器日志

Examples:
    $0 run init
    $0 api version
    $0 status
    $0 read README.md
    echo "content" | $0 write test.txt

Environment:
    WORKER_TOKEN    Worker 认证令牌（默认: worker-secret-token）
    WORKER_URL      Worker HTTP 地址（默认: http://localhost:3000）
    GATEWAY_URL     Gateway HTTP 地址（默认: http://localhost:8080）
EOF
}

# 主逻辑
case "${1:-}" in
    run)
        shift
        check_containers
        run_claude "$@"
        ;;
    api)
        shift
        check_containers
        run_claude_api "$@"
        ;;
    status)
        status
        ;;
    git-status|git_status)
        git_status
        ;;
    project-info|project_info)
        project_info
        ;;
    read)
        file_read "$2"
        ;;
    write)
        file_write "$2" "$(cat)"
        ;;
    shell)
        docker exec -it "${WORKER_CONTAINER}" /bin/bash
        ;;
    logs)
        docker logs -f "${WORKER_CONTAINER}"
        ;;
    *)
        usage
        exit 1
        ;;
esac
