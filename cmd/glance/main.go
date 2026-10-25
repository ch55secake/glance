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
	m := initialModel()
	if themeID, configErr := loadThemePreference(); configErr != nil {
		m.themeConfigError = safeText(configErr.Error())
	} else {
		m.theme = applyTheme(themeID)
	}
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}
