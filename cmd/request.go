package cmd

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// runRequest handles the HTTP request command
func runRequest(args []string) error {
	fs := flag.NewFlagSet("request", flag.ExitOnError)

	var (
		method      string
		headers     stringSlice
		queryParams stringSlice
		body        string
		bodyFile    string
		timeout     time.Duration
		outputFile  string
		showHeaders bool
		formatJSON  bool
	)

	fs.StringVar(&method, "method", "GET", "HTTP method (GET, POST, PUT, PATCH, DELETE, HEAD)")
	fs.Var(&headers, "header", "HTTP header (can be used multiple times, format: key=value)")
	fs.Var(&queryParams, "query", "Query parameter (can be used multiple times, format: key=value)")
	fs.StringVar(&body, "body", "", "Request body (string)")
	fs.StringVar(&bodyFile, "body-file", "", "File containing request body")
	fs.DurationVar(&timeout, "timeout", 30*time.Second, "Request timeout")
	fs.StringVar(&outputFile, "output", "", "Output file (default: stdout)")
	fs.BoolVar(&showHeaders, "include-headers", false, "Include response headers in output")
	fs.BoolVar(&formatJSON, "format-json", false, "Format JSON response with indentation")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	if fs.NArg() < 1 {
		printRequestUsage()
		return fmt.Errorf("URL required")
	}

	requestURL := fs.Arg(0)

	// Validate URL
	if _, err := url.Parse(requestURL); err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Validate method
	method = strings.ToUpper(method)
	validMethods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	if !contains(validMethods, method) {
		return fmt.Errorf("invalid HTTP method: %s", method)
	}

	// Build request body
	var bodyReader io.Reader
	if bodyFile != "" {
		data, err := os.ReadFile(bodyFile)
		if err != nil {
			return fmt.Errorf("reading body file: %w", err)
		}
		bodyReader = strings.NewReader(string(data))
	} else if body != "" {
		bodyReader = strings.NewReader(body)
	}

	// Create request
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, requestURL, bodyReader)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	// Add query parameters
	if len(queryParams) > 0 {
		q := req.URL.Query()
		for _, param := range queryParams {
			parts := strings.SplitN(param, "=", 2)
			if len(parts) == 2 {
				q.Set(parts[0], parts[1])
			} else {
				q.Add(parts[0], "")
			}
		}
		req.URL.RawQuery = q.Encode()
	}

	// Add headers
	for _, header := range headers {
		parts := strings.SplitN(header, ":", 2)
		if len(parts) == 2 {
			req.Header.Set(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
		} else {
			return fmt.Errorf("invalid header format: %s (expected key:value)", header)
		}
	}

	// Set default Content-Type if body is present and not already set
	if bodyReader != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// Execute request
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}

	// Format JSON if requested
	output := respBody
	if formatJSON && json.Valid(respBody) {
		var jsonData interface{}
		if err := json.Unmarshal(respBody, &jsonData); err == nil {
			formatted, err := json.MarshalIndent(jsonData, "", "  ")
			if err == nil {
				output = formatted
			}
		}
	}

	// Build output
	var outputBuilder strings.Builder

	if showHeaders {
		outputBuilder.WriteString(fmt.Sprintf("HTTP/%d.%d %d %s\n", resp.ProtoMajor, resp.ProtoMinor, resp.StatusCode, resp.Status))
		for key, values := range resp.Header {
			for _, value := range values {
				outputBuilder.WriteString(fmt.Sprintf("%s: %s\n", key, value))
			}
		}
		outputBuilder.WriteString("\n")
	}

	outputBuilder.Write(output)

	// Write output
	result := outputBuilder.String()
	if outputFile != "" {
		if err := os.WriteFile(outputFile, []byte(result), 0644); err != nil {
			return fmt.Errorf("writing output file: %w", err)
		}
		fmt.Printf("Response saved to %s (HTTP %d)\n", outputFile, resp.StatusCode)
	} else {
		fmt.Println(result)
	}

	// Return error for non-2xx status codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP error %d: %s", resp.StatusCode, resp.Status)
	}

	return nil
}

func printRequestUsage() {
	fmt.Println(`vibe-git request - Make HTTP requests to external services

Usage:
  vibe-git request <url> [flags]

Flags:
  -method string          HTTP method (GET, POST, PUT, PATCH, DELETE, HEAD) (default "GET")
  -header string          HTTP header (can be used multiple times, format: key:value)
  -query string           Query parameter (can be used multiple times, format: key=value)
  -body string            Request body (string)
  -body-file string       File containing request body
  -timeout duration       Request timeout (default 30s)
  -output string          Output file (default: stdout)
  -include-headers        Include response headers in output
  -format-json            Format JSON response with indentation

Examples:
  # Simple GET request
  vibe-git request https://api.example.com/users

  # POST request with JSON body
  vibe-git request https://api.example.com/users -method POST -body '{"name":"John"}' -header "Content-Type: application/json"

  # Request with query parameters
  vibe-git request https://api.example.com/search -query "q=golang" -query "limit=10"

  # Save response to file
  vibe-git request https://api.example.com/data -output data.json

  # Format JSON response
  vibe-git request https://api.example.com/users -format-json

  # Include response headers
  vibe-git request https://api.example.com/users -include-headers`)
}

// stringSlice is a custom flag type for collecting multiple values
type stringSlice []string

func (s *stringSlice) String() string {
	return strings.Join(*s, ", ")
}

func (s *stringSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
