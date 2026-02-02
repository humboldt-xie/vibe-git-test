package main

import "testing"

func TestHello(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"with name", "Alice", "Hello, Alice!"},
		{"empty name", "", "Hello, World!"},
		{"with another name", "Bob", "Hello, Bob!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Hello(tt.input)
			if result != tt.expected {
				t.Errorf("Hello(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
