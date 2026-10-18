package main

import (
	"bytes"
	"testing"
)

func TestMainOutput(t *testing.T) {
	var output bytes.Buffer
	if err := run(&output); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "glance: GitHub dashboard under development\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
