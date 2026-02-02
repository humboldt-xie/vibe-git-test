# Vibe-Git Docker 部署

这套 Docker Compose 配置提供了安全的隔离部署方案，将 API 密钥保护与工作环境分离。

## 架构设计

```
┌─────────────────────────────────────────────────────────────┐
│                        Host Machine                          │
│                                                              │
│  ┌─────────────────┐         ┌─────────────────────────────┐ │
│  │  vibe-git       │         │     Docker Network          │ │
│  │  (主程序)        │◀────────│  ┌───────────────────────┐  │ │
│  │  - Git Token    │  HTTP   │  │  claude-gateway       │  │ │
│  │  - Git 操作     │         │  │  - 保护 Anthropic Key │  │ │
│  │  - 编排工作流   │         │  │  - API 代理           │  │ │
│  │                 │         │  └───────────────────────┘  │ │
│  └─────────────────┘         │             │               │ │
│                              │      API Proxy (内部)       │ │
│                              │             ▼               │ │
│                              │  ┌───────────────────────┐  │ │
│                              │  │  claude-worker        │  │ │
│                              │  │  - Claude Code CLI    │  │ │
│                              │  │  - 项目代码卷         │  │ │
│                              │  │  - HTTP Git API       │  │ │
│                              │  └───────────────────────┘  │ │
│                              │                             │ │
│                              └─────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

## 核心特性

1. **密钥隔离**: Anthropic API 密钥仅存在于 Gateway 容器
2. **工作隔离**: Claude 在工作容器中运行，与 Host 隔离
3. **HTTP API**: 工作容器提供 HTTP API 替代 Git 命令
4. **卷映射**:
   - 项目代码 → 工作容器
   - Claude 配置 → Gateway 容器
   - Skills → 工作容器

## 快速开始

### 1. 配置环境变量

```bash
cp .env.example .env
# 编辑 .env 文件，设置必要的环境变量
```

### 2. 启动服务

```bash
docker-compose up -d
```

### 3. 验证状态

```bash
./docker/scripts/claude-exec.sh status
```

## 服务说明

### Claude Gateway (端口 8080)

- **作用**: 保护 Anthropic API 密钥
- **访问**: 仅内部网络（Worker）可以访问
- **API**: 代理 Anthropic API，添加认证层

### Claude Worker (端口 3000)

- **作用**: 运行 Claude Code CLI
- **功能**:
  - 提供 HTTP API 执行 Claude 命令
  - 提供 Git 操作 API（替代本地 Git）
  - 文件读写 API

## 使用方法

### 通过脚本执行 Claude

```bash
# 运行 Claude 命令（交互式）
./docker/scripts/claude-exec.sh run init
./docker/scripts/claude-exec.sh run version

# 通过 API 运行（非交互式）
./docker/scripts/claude-exec.sh api version

# 查看状态
./docker/scripts/claude-exec.sh status

# 查看 Git 状态
./docker/scripts/claude-exec.sh git-status

# 进入容器 Shell
./docker/scripts/claude-exec.sh shell

# 查看日志
./docker/scripts/claude-exec.sh logs
```

### 在 Go 代码中使用

```go
import "vibe-git/internal/worker"

// 创建工作容器客户端
w := worker.NewClient("http://localhost:3000", "worker-secret-token")

// 运行 Claude 命令
resp, err := w.RunClaude(ctx, "version", nil, 30)

// 读取文件
content, err := w.FileRead(ctx, "README.md")

// 写入文件
err := w.FileWrite(ctx, "test.txt", "content")

// 获取 Git 状态
status, err := w.GitStatus(ctx)
```

### 直接调用 HTTP API

```bash
# 运行 Claude 命令
curl -X POST http://localhost:3000/claude/run \
  -H "X-Worker-Auth: worker-secret-token" \
  -H "Content-Type: application/json" \
  -d '{
    "command": "version",
    "args": [],
    "timeout": 30
  }'

# 读取文件
curl -X POST http://localhost:3000/file/read \
  -H "X-Worker-Auth: worker-secret-token" \
  -H "Content-Type: application/json" \
  -d '{"path": "README.md"}'

# Git 状态
curl -H "X-Worker-Auth: worker-secret-token" \
  http://localhost:3000/git/status
```

## 安全考虑

1. **Token 保护**:
   - Gateway Token 和 Worker Token 应使用强密码
   - 不要在代码中硬编码 Token
   - 使用 `.env` 文件管理敏感信息

2. **网络隔离**:
   - Gateway 仅绑定到 localhost (127.0.0.1)
   - Worker 仅绑定到 localhost (127.0.0.1)
   - 两个服务在独立的 Docker 网络中通信

3. **文件访问**:
   - Worker 只能访问映射的项目目录
   - 路径安全检查防止目录遍历攻击

## 故障排除

### 容器无法启动

```bash
# 查看日志
docker-compose logs claude-gateway
docker-compose logs claude-worker

# 重新构建
docker-compose build --no-cache
docker-compose up -d
```

### Gateway 连接失败

```bash
# 检查 Gateway 状态
curl http://localhost:8080/health

# 检查 Worker 是否能连接 Gateway
docker exec vibe-git-worker curl http://claude-gateway:8080/health
```

### Claude 命令失败

```bash
# 进入 Worker 容器调试
docker exec -it vibe-git-worker /bin/bash

# 检查 Claude 安装
which claude
claude version

# 检查 Gateway 可访问性
curl http://claude-gateway:8080/claude/status
```

## 更新部署

```bash
# 拉取最新代码
git pull

# 重建镜像
docker-compose build

# 重启服务
docker-compose up -d

# 清理旧镜像
docker image prune -f
```

## 卸载

```bash
# 停止并删除容器
docker-compose down

# 删除镜像
docker rmi vibe-git_claude-gateway vibe-git_claude-worker

# 删除卷（谨慎操作）
docker volume prune
```
