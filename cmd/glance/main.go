package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "glance:", err)
		os.Exit(1)
	}
}

func run() error {
	_, err := tea.NewProgram(initialModel(), tea.WithAltScreen()).Run()
	return err
}
