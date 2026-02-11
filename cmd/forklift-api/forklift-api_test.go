package main

import "testing"

func TestInit_SetsLogger(t *testing.T) {
	if log.GetSink() == nil {
		t.Fatalf("expected init() to set a logger sink")
	}
}
