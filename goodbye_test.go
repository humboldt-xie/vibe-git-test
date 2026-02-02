package main

import "testing"

func TestGoodbye(t *testing.T) {
	// This test ensures Goodbye doesn't panic
	// Since Goodbye prints to stdout, we just verify it runs without error
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Goodbye panicked: %v", r)
		}
	}()
	
	Goodbye("World")
}
