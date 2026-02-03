package httpclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	client := NewClient("https://api.example.com")
	if client == nil {
		t.Fatal("expected client to be created")
	}
	if client.baseURL != "https://api.example.com" {
		t.Errorf("expected baseURL to be https://api.example.com, got %s", client.baseURL)
	}
	if client.httpClient.Timeout != 30*time.Second {
		t.Errorf("expected timeout to be 30s, got %v", client.httpClient.Timeout)
	}
}

func TestNewClientWithTimeout(t *testing.T) {
	client := NewClientWithTimeout("https://api.example.com", 60*time.Second)
	if client.httpClient.Timeout != 60*time.Second {
		t.Errorf("expected timeout to be 60s, got %v", client.httpClient.Timeout)
	}
}

func TestSetHeader(t *testing.T) {
	client := NewClient("https://api.example.com")
	client.SetHeader("X-Custom-Header", "custom-value")
	if client.headers["X-Custom-Header"] != "custom-value" {
		t.Errorf("expected header to be set")
	}
}

func TestSetAuthToken(t *testing.T) {
	client := NewClient("https://api.example.com")
	client.SetAuthToken("my-token")
	if client.headers["Authorization"] != "Bearer my-token" {
		t.Errorf("expected Authorization header to be Bearer my-token, got %s", client.headers["Authorization"])
	}
}

func TestSetBasicAuth(t *testing.T) {
	client := NewClient("https://api.example.com")
	client.SetBasicAuth("username", "password")
	expected := "Basic " + base64Encode("username:password")
	if client.headers["Authorization"] != expected {
		t.Errorf("expected Authorization header to be %s, got %s", expected, client.headers["Authorization"])
	}
}

func TestGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET method, got %s", r.Method)
		}
		if r.URL.Path != "/test" {
			t.Errorf("expected path /test, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message": "success"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	resp, err := client.Get(context.Background(), "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if string(resp.Body) != `{"message": "success"}` {
		t.Errorf("unexpected body: %s", string(resp.Body))
	}
}

func TestPost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		if body["key"] != "value" {
			t.Errorf("unexpected body: %v", body)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id": 1}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	opts := &RequestOptions{
		Body: map[string]string{"key": "value"},
	}
	resp, err := client.Post(context.Background(), "/test", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected status 201, got %d", resp.StatusCode)
	}
}

func TestPut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT method, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	resp, err := client.Put(context.Background(), "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestDelete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE method, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	resp, err := client.Delete(context.Background(), "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", resp.StatusCode)
	}
}

func TestQueryParams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("key1") != "value1" {
			t.Errorf("expected key1=value1, got %s", query.Get("key1"))
		}
		if query.Get("key2") != "value2" {
			t.Errorf("expected key2=value2, got %s", query.Get("key2"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	opts := &RequestOptions{
		Query: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}
	_, err := client.Get(context.Background(), "/test", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResponseJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message": "hello", "count": 42}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	resp, err := client.Get(context.Background(), "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]interface{}
	err = resp.JSON(&result)
	if err != nil {
		t.Fatalf("unexpected error parsing JSON: %v", err)
	}
	if result["message"] != "hello" {
		t.Errorf("expected message=hello, got %v", result["message"])
	}
	if result["count"] != float64(42) {
		t.Errorf("expected count=42, got %v", result["count"])
	}
}

func TestResponseString(t *testing.T) {
	resp := &Response{
		Body: []byte("hello world"),
	}
	if resp.String() != "hello world" {
		t.Errorf("expected 'hello world', got %s", resp.String())
	}
}

func TestBase64Encode(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"f", "Zg=="},
		{"fo", "Zm8="},
		{"foo", "Zm9v"},
		{"foob", "Zm9vYg=="},
		{"fooba", "Zm9vYmE="},
		{"foobar", "Zm9vYmFy"},
	}

	for _, test := range tests {
		result := base64Encode(test.input)
		if result != test.expected {
			t.Errorf("base64Encode(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func TestRequestWithTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	opts := &RequestOptions{
		Timeout: 50 * time.Millisecond,
	}
	_, err := client.Get(context.Background(), "/test", opts)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestRequestWithHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom-Header") != "custom-value" {
			t.Errorf("expected X-Custom-Header=custom-value, got %s", r.Header.Get("X-Custom-Header"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	opts := &RequestOptions{
		Headers: map[string]string{
			"X-Custom-Header": "custom-value",
		},
	}
	_, err := client.Get(context.Background(), "/test", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH method, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	resp, err := client.Patch(context.Background(), "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestHead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("expected HEAD method, got %s", r.Method)
		}
		w.Header().Set("X-Test-Header", "test-value")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	resp, err := client.Head(context.Background(), "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if resp.Headers["X-Test-Header"] != "test-value" {
		t.Errorf("expected X-Test-Header=test-value, got %s", resp.Headers["X-Test-Header"])
	}
}

func TestRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CUSTOM" {
			t.Errorf("expected CUSTOM method, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	resp, err := client.Request(context.Background(), "CUSTOM", "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}
