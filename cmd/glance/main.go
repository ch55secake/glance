package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "glance:", err)
		os.Exit(1)
	}
}

func run(out io.Writer) error {
	_, err := fmt.Fprintln(out, "glance: GitHub dashboard under development")
	return err
}
