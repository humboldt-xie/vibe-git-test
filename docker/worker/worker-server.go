package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	workerToken string
	projectPath string
)

func main() {
	workerToken = os.Getenv("WORKER_TOKEN")
	if workerToken == "" {
		workerToken = "worker-secret-token"
		log.Println("Warning: Using default worker token. Set WORKER_TOKEN for production.")
	}

	projectPath = os.Getenv("PROJECT_PATH")
	if projectPath == "" {
		projectPath = "/workspace/project"
	}

	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", handleHealth)

	// Claude operations
	mux.HandleFunc("/claude/run", handleClaudeRun)
	mux.HandleFunc("/claude/status", handleClaudeStatus)

	// Git operations (替代 Git 命令)
	mux.HandleFunc("/git/status", handleGitStatus)
	mux.HandleFunc("/git/diff", handleGitDiff)
	mux.HandleFunc("/git/log", handleGitLog)
	mux.HandleFunc("/git/show", handleGitShow)
	mux.HandleFunc("/git/ls-files", handleGitLsFiles)
	mux.HandleFunc("/git/cat-file", handleGitCatFile)

	// File operations
	mux.HandleFunc("/file/read", handleFileRead)
	mux.HandleFunc("/file/write", handleFileWrite)
	mux.HandleFunc("/file/list", handleFileList)
	mux.HandleFunc("/file/stat", handleFileStat)

	// Project info
	mux.HandleFunc("/project/info", handleProjectInfo)
	mux.HandleFunc("/project/tree", handleProjectTree)

	port := os.Getenv("WORKER_HTTP_PORT")
	if port == "" {
		port = "3000"
	}

	log.Printf("Worker server starting on port %s", port)
	log.Printf("Project path: %s", projectPath)

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      authMiddleware(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second,
	}

	log.Fatal(server.ListenAndServe())
}

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		token := r.Header.Get("X-Worker-Auth")
		if token == "" {
			token = r.URL.Query().Get("token")
		}

		if token != workerToken {
			log.Printf("Unauthorized request from %s", r.RemoteAddr)
			writeError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"status":       "healthy",
		"service":      "claude-worker",
		"timestamp":    time.Now().Format(time.RFC3339),
		"project_path": projectPath,
	})
}

// ClaudeRunRequest represents a request to run Claude
type ClaudeRunRequest struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Timeout int      `json:"timeout"` // seconds
	Stdin   string   `json:"stdin"`
}

// ClaudeRunResponse represents the response from running Claude
type ClaudeRunResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Duration string `json:"duration"`
}

func handleClaudeRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ClaudeRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(req.Timeout)*time.Second)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, "claude", append([]string{req.Command}, req.Args...)...)
	cmd.Dir = projectPath

	if req.Stdin != "" {
		cmd.Stdin = strings.NewReader(req.Stdin)
	}

	stdout, err := cmd.Output()
	exitCode := 0
	stderr := ""

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
			stderr = string(exitError.Stderr)
		} else {
			exitCode = 1
			stderr = err.Error()
		}
	}

	writeJSON(w, ClaudeRunResponse{
		Stdout:   string(stdout),
		Stderr:   stderr,
		ExitCode: exitCode,
	})
}

func handleClaudeStatus(w http.ResponseWriter, r *http.Request) {
	cmd := exec.Command("which", "claude")
	output, err := cmd.Output()

	status := map[string]interface{}{
		"installed": err == nil,
		"path":      strings.TrimSpace(string(output)),
	}

	// Check if gateway is accessible
	resp, err := http.Get("http://claude-gateway:8080/health")
	if err != nil {
		status["gateway"] = "unreachable"
		status["gateway_error"] = err.Error()
	} else {
		defer resp.Body.Close()
		status["gateway"] = resp.Status
	}

	writeJSON(w, status)
}

// GitRequest represents a git command request
type GitRequest struct {
	Args []string `json:"args"`
}

func handleGitStatus(w http.ResponseWriter, r *http.Request) {
	runGitCommand(w, []string{"status", "--porcelain"})
}

func handleGitDiff(w http.ResponseWriter, r *http.Request) {
	args := []string{"diff"}
	if r.URL.Query().Get("cached") == "true" {
		args = append(args, "--cached")
	}
	if file := r.URL.Query().Get("file"); file != "" {
		args = append(args, file)
	}
	runGitCommand(w, args)
}

func handleGitLog(w http.ResponseWriter, r *http.Request) {
	limit := r.URL.Query().Get("limit")
	if limit == "" {
		limit = "10"
	}
	runGitCommand(w, []string{"log", "--oneline", "-" + limit})
}

func handleGitShow(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		ref = "HEAD"
	}
	runGitCommand(w, []string{"show", "--stat", ref})
}

func handleGitLsFiles(w http.ResponseWriter, r *http.Request) {
	runGitCommand(w, []string{"ls-files"})
}

func handleGitCatFile(w http.ResponseWriter, r *http.Request) {
	object := r.URL.Query().Get("object")
	if object == "" {
		writeError(w, "object parameter required", http.StatusBadRequest)
		return
	}
	runGitCommand(w, []string{"cat-file", "-p", object})
}

func runGitCommand(w http.ResponseWriter, args []string) {
	cmd := exec.Command("git", args...)
	cmd.Dir = projectPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		writeJSON(w, map[string]interface{}{
			"success": false,
			"output":  string(output),
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, map[string]interface{}{
		"success": true,
		"output":  string(output),
	})
}

// FileReadRequest represents a file read request
type FileReadRequest struct {
	Path string `json:"path"`
}

func handleFileRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req FileReadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Security: ensure path is within project
	fullPath := filepath.Join(projectPath, req.Path)
	if !strings.HasPrefix(fullPath, projectPath) {
		writeError(w, "Invalid path", http.StatusForbidden)
		return
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"path":    req.Path,
		"content": string(content),
		"size":    len(content),
	})
}

// FileWriteRequest represents a file write request
type FileWriteRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func handleFileWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req FileWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Security: ensure path is within project
	fullPath := filepath.Join(projectPath, req.Path)
	if !strings.HasPrefix(fullPath, projectPath) {
		writeError(w, "Invalid path", http.StatusForbidden)
		return
	}

	// Create directory if needed
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(fullPath, []byte(req.Content), 0644); err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"success": true,
		"path":    req.Path,
		"size":    len(req.Content),
	})
}

func handleFileList(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("dir")
	if dir == "" {
		dir = "."
	}

	fullPath := filepath.Join(projectPath, dir)
	if !strings.HasPrefix(fullPath, projectPath) {
		writeError(w, "Invalid path", http.StatusForbidden)
		return
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var files []map[string]interface{}
	for _, entry := range entries {
		info, _ := entry.Info()
		files = append(files, map[string]interface{}{
			"name":    entry.Name(),
			"is_dir":  entry.IsDir(),
			"size":    info.Size(),
			"mod_time": info.ModTime(),
		})
	}

	writeJSON(w, map[string]interface{}{
		"path":  dir,
		"files": files,
	})
}

func handleFileStat(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, "path parameter required", http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(projectPath, path)
	if !strings.HasPrefix(fullPath, projectPath) {
		writeError(w, "Invalid path", http.StatusForbidden)
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"path":     path,
		"size":     info.Size(),
		"mod_time": info.ModTime(),
		"is_dir":   info.IsDir(),
		"mode":     info.Mode().String(),
	})
}

func handleProjectInfo(w http.ResponseWriter, r *http.Request) {
	info := map[string]interface{}{
		"path": projectPath,
	}

	// Check if git repo
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = projectPath
	output, err := cmd.Output()
	info["is_git_repo"] = err == nil
	if err == nil {
		info["git_dir"] = strings.TrimSpace(string(output))
	}

	// Get branch
	cmd = exec.Command("git", "branch", "--show-current")
	cmd.Dir = projectPath
	output, err = cmd.Output()
	if err == nil {
		info["branch"] = strings.TrimSpace(string(output))
	}

	// Get last commit
	cmd = exec.Command("git", "log", "-1", "--format=%H")
	cmd.Dir = projectPath
	output, err = cmd.Output()
	if err == nil {
		info["last_commit"] = strings.TrimSpace(string(output))
	}

	writeJSON(w, info)
}

func handleProjectTree(w http.ResponseWriter, r *http.Request) {
	depth := r.URL.Query().Get("depth")
	if depth == "" {
		depth = "3"
	}

	cmd := exec.Command("find", ".", "-maxdepth", depth, "-type", "f")
	cmd.Dir = projectPath
	output, err := cmd.Output()
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	writeJSON(w, map[string]interface{}{
		"files": files,
		"depth": depth,
	})
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error":   message,
		"success": false,
	})
}
