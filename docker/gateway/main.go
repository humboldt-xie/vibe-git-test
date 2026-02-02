package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	anthropicAPI = "https://api.anthropic.com"
	apiVersion   = "2023-06-01"
)

var (
	anthropicKey  string
	gatewayToken  string
	proxy         *httputil.ReverseProxy
)

func main() {
	anthropicKey = os.Getenv("ANTHROPIC_API_KEY")
	if anthropicKey == "" {
		log.Fatal("ANTHROPIC_API_KEY environment variable is required")
	}

	gatewayToken = os.Getenv("GATEWAY_TOKEN")
	if gatewayToken == "" {
		gatewayToken = "vibe-git-secret-token"
		log.Println("Warning: Using default gateway token. Set GATEWAY_TOKEN for production.")
	}

	// Create reverse proxy to Anthropic
	targetURL, _ := url.Parse(anthropicAPI)
	proxy = httputil.NewSingleHostReverseProxy(targetURL)

	// Modify the director to add our headers
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = targetURL.Host
		req.Header.Set("X-Api-Key", anthropicKey)
		req.Header.Set("Anthropic-Version", apiVersion)
		// Remove internal auth header before forwarding
		req.Header.Del("X-Gateway-Auth")
	}

	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", handleHealth)

	// Metrics endpoint
	mux.HandleFunc("/metrics", handleMetrics)

	// Proxy all Anthropic API requests
	mux.HandleFunc("/v1/", handleProxy)

	// Claude Code specific endpoints
	mux.HandleFunc("/claude/", handleClaude)

	port := os.Getenv("GATEWAY_PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Claude Gateway starting on port %s", port)
	log.Printf("Protecting Anthropic API key - workers use local authentication")

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      authMiddleware(mux),
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 120 * time.Second,
	}

	log.Fatal(server.ListenAndServe())
}

// authMiddleware validates gateway token
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Health check doesn't require auth
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		// Validate gateway token
		token := r.Header.Get("X-Gateway-Auth")
		if token == "" {
			token = r.URL.Query().Get("token")
		}

		if token != gatewayToken {
			log.Printf("Unauthorized request from %s", r.RemoteAddr)
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":    "healthy",
		"service":   "claude-gateway",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

var requestCount int64
var lastRequestTime time.Time

func handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"requests":          requestCount,
		"last_request_time": lastRequestTime,
		"uptime":            time.Since(time.Now().Add(-time.Hour)).String(),
	})
}

func handleProxy(w http.ResponseWriter, r *http.Request) {
	requestCount++
	lastRequestTime = time.Now()

	log.Printf("Proxying %s %s", r.Method, r.URL.Path)

	// Add CORS headers for local development
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Gateway-Auth")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	proxy.ServeHTTP(w, r)
}

// handleClaude provides additional Claude-specific endpoints
func handleClaude(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/claude/status":
		handleClaudeStatus(w, r)
	case "/claude/models":
		handleModels(w, r)
	default:
		http.NotFound(w, r)
	}
}

func handleClaudeStatus(w http.ResponseWriter, r *http.Request) {
	// Check if Anthropic API is accessible
	req, _ := http.NewRequest("GET", anthropicAPI+"/v1/models", nil)
	req.Header.Set("X-Api-Key", anthropicKey)
	req.Header.Set("Anthropic-Version", apiVersion)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)

	status := map[string]interface{}{
		"gateway":     "ok",
		"timestamp":   time.Now().Format(time.RFC3339),
		"api_key_set": strings.HasPrefix(anthropicKey, "sk-") || strings.HasPrefix(anthropicKey, "sk-ant"),
	}

	if err != nil {
		status["anthropic"] = "error"
		status["error"] = err.Error()
	} else {
		defer resp.Body.Close()
		status["anthropic"] = resp.Status
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func handleModels(w http.ResponseWriter, r *http.Request) {
	req, _ := http.NewRequest("GET", anthropicAPI+"/v1/models", nil)
	req.Header.Set("X-Api-Key", anthropicKey)
	req.Header.Set("Anthropic-Version", apiVersion)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
