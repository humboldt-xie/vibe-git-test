// Package httpclient provides a generic HTTP client for making requests to external services.
// It supports various HTTP methods, custom headers, query parameters, and request/response handling.
package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client provides methods to make HTTP requests to external services
type Client struct {
	baseURL    string
	httpClient *http.Client
	headers    map[string]string
}

// Response represents an HTTP response
type Response struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
}

// String returns the response body as a string
func (r *Response) String() string {
	return string(r.Body)
}

// JSON unmarshals the response body into the provided value
func (r *Response) JSON(v interface{}) error {
	return json.Unmarshal(r.Body, v)
}

// RequestOptions contains optional parameters for HTTP requests
type RequestOptions struct {
	Headers map[string]string
	Query   map[string]string
	Body    interface{}
	Timeout time.Duration
}

// NewClient creates a new HTTP client
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
		headers:    make(map[string]string),
	}
}

// NewClientWithTimeout creates a new HTTP client with a custom timeout
func NewClientWithTimeout(baseURL string, timeout time.Duration) *Client {
	c := NewClient(baseURL)
	c.httpClient.Timeout = timeout
	return c
}

// SetHeader sets a default header for all requests
func (c *Client) SetHeader(key, value string) {
	c.headers[key] = value
}

// SetAuthToken sets the Authorization header with a Bearer token
func (c *Client) SetAuthToken(token string) {
	c.headers["Authorization"] = "Bearer " + token
}

// SetBasicAuth sets the Authorization header with Basic auth credentials
func (c *Client) SetBasicAuth(username, password string) {
	c.headers["Authorization"] = "Basic " + basicAuth(username, password)
}

// Get makes a GET request
func (c *Client) Get(ctx context.Context, path string, opts *RequestOptions) (*Response, error) {
	return c.doRequest(ctx, http.MethodGet, path, opts)
}

// Post makes a POST request
func (c *Client) Post(ctx context.Context, path string, opts *RequestOptions) (*Response, error) {
	return c.doRequest(ctx, http.MethodPost, path, opts)
}

// Put makes a PUT request
func (c *Client) Put(ctx context.Context, path string, opts *RequestOptions) (*Response, error) {
	return c.doRequest(ctx, http.MethodPut, path, opts)
}

// Patch makes a PATCH request
func (c *Client) Patch(ctx context.Context, path string, opts *RequestOptions) (*Response, error) {
	return c.doRequest(ctx, http.MethodPatch, path, opts)
}

// Delete makes a DELETE request
func (c *Client) Delete(ctx context.Context, path string, opts *RequestOptions) (*Response, error) {
	return c.doRequest(ctx, http.MethodDelete, path, opts)
}

// Head makes a HEAD request
func (c *Client) Head(ctx context.Context, path string, opts *RequestOptions) (*Response, error) {
	return c.doRequest(ctx, http.MethodHead, path, opts)
}

// Request makes an HTTP request with the specified method
func (c *Client) Request(ctx context.Context, method, path string, opts *RequestOptions) (*Response, error) {
	return c.doRequest(ctx, method, path, opts)
}

// doRequest performs the actual HTTP request
func (c *Client) doRequest(ctx context.Context, method, path string, opts *RequestOptions) (*Response, error) {
	if opts == nil {
		opts = &RequestOptions{}
	}

	// Build URL
	reqURL := c.baseURL + path
	if len(opts.Query) > 0 {
		query := url.Values{}
		for k, v := range opts.Query {
			query.Set(k, v)
		}
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		reqURL += sep + query.Encode()
	}

	// Prepare body
	var bodyReader io.Reader
	if opts.Body != nil {
		switch v := opts.Body.(type) {
		case string:
			bodyReader = strings.NewReader(v)
		case []byte:
			bodyReader = bytes.NewReader(v)
		case io.Reader:
			bodyReader = v
		default:
			jsonBody, err := json.Marshal(opts.Body)
			if err != nil {
				return nil, fmt.Errorf("marshaling request body: %w", err)
			}
			bodyReader = bytes.NewReader(jsonBody)
		}
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Set default headers
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	// Set request-specific headers
	for k, v := range opts.Headers {
		req.Header.Set(k, v)
	}

	// Set default Content-Type if body is present and not already set
	if opts.Body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// Apply timeout if specified
	client := c.httpClient
	if opts.Timeout > 0 {
		client = &http.Client{Timeout: opts.Timeout}
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	// Build response headers
	headers := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Body:       body,
	}, nil
}

// basicAuth encodes username and password for Basic auth
func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64Encode(auth)
}

// base64Encode performs base64 encoding
func base64Encode(s string) string {
	const base64Chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var result strings.Builder
	data := []byte(s)

	for i := 0; i < len(data); i += 3 {
		b := []int{0, 0, 0}
		n := 0
		for j := 0; j < 3 && i+j < len(data); j++ {
			b[j] = int(data[i+j])
			n++
		}

		switch n {
		case 1:
			result.WriteByte(base64Chars[b[0]>>2])
			result.WriteByte(base64Chars[(b[0]&0x03)<<4])
			result.WriteString("==")
		case 2:
			result.WriteByte(base64Chars[b[0]>>2])
			result.WriteByte(base64Chars[((b[0]&0x03)<<4)|(b[1]>>4)])
			result.WriteByte(base64Chars[(b[1]&0x0f)<<2])
			result.WriteByte('=')
		case 3:
			result.WriteByte(base64Chars[b[0]>>2])
			result.WriteByte(base64Chars[((b[0]&0x03)<<4)|(b[1]>>4)])
			result.WriteByte(base64Chars[((b[1]&0x0f)<<2)|(b[2]>>6)])
			result.WriteByte(base64Chars[b[2]&0x3f])
		}
	}

	return result.String()
}
