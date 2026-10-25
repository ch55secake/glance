package main

import "github.com/charmbracelet/lipgloss"

const defaultThemeID = "midnight"

type theme struct {
	id, name, description                                      string
	brand, accent, text, muted, good, warning, error           string
	selection, active                                          string
	openText, openBackground, draftText, draftBackground       string
	mergedText, mergedBackground, unreadText, unreadBackground string
	addedText, addedBackground, deletedText, deletedBackground string
	hunkText, hunkBackground, fileText, fileBackground         string
	keyword, codeString, number, comment                       string
}

var themes = []theme{
	{
		id: "midnight", name: "Midnight", description: "Cool cyan and violet on a deep slate",
		brand: "#82E9F0", accent: "#C9A7FF", text: "#F1F2F6", muted: "#A1AAC4",
		good: "#9AE8B8", warning: "#FBD389", error: "#FF959A", selection: "#343C57", active: "#7252AB",
		openText: "#B8F3CF", openBackground: "#214D42", draftText: "#FFE2A6", draftBackground: "#61492A",
		mergedText: "#D6C5FF", mergedBackground: "#443460", unreadText: "#D6C5FF", unreadBackground: "#443460",
		addedText: "#B8F3CF", addedBackground: "#183A34", deletedText: "#FFB6B9", deletedBackground: "#452B36",
		hunkText: "#C9A7FF", hunkBackground: "#302946", fileText: "#F1F2F6", fileBackground: "#343C57",
		keyword: "#D6C5FF", codeString: "#FBD389", number: "#82E9F0", comment: "#A1AAC4",
	},
	{
		id: "nord", name: "Nord", description: "Arctic blue, frost, and muted pine",
		brand: "#88C0D0", accent: "#B48EAD", text: "#ECEFF4", muted: "#A3B2C2",
		good: "#A3BE8C", warning: "#EBCB8B", error: "#BF616A", selection: "#3B4656", active: "#5E81AC",
		openText: "#A3BE8C", openBackground: "#2B3B3A", draftText: "#EBCB8B", draftBackground: "#51452F",
		mergedText: "#B48EAD", mergedBackground: "#443B4B", unreadText: "#B48EAD", unreadBackground: "#443B4B",
		addedText: "#A3BE8C", addedBackground: "#293B38", deletedText: "#D57780", deletedBackground: "#432F39",
		hunkText: "#B48EAD", hunkBackground: "#383848", fileText: "#ECEFF4", fileBackground: "#3B4656",
		keyword: "#B48EAD", codeString: "#EBCB8B", number: "#8FBCBB", comment: "#A3B2C2",
	},
	{
		id: "dracula", name: "Dracula", description: "Vivid purple, pink, and mint",
		brand: "#8BE9FD", accent: "#BD93F9", text: "#F8F8F2", muted: "#A9A7B7",
		good: "#50FA7B", warning: "#F1FA8C", error: "#FF5555", selection: "#414052", active: "#7B5AA6",
		openText: "#8AFFA7", openBackground: "#234432", draftText: "#F1FA8C", draftBackground: "#514B2C",
		mergedText: "#FF79C6", mergedBackground: "#4A3046", unreadText: "#FF79C6", unreadBackground: "#4A3046",
		addedText: "#8AFFA7", addedBackground: "#203A32", deletedText: "#FF7A83", deletedBackground: "#482C39",
		hunkText: "#BD93F9", hunkBackground: "#342E4A", fileText: "#F8F8F2", fileBackground: "#414052",
		keyword: "#BD93F9", codeString: "#F1FA8C", number: "#8BE9FD", comment: "#A9A7B7",
	},
	{
		id: "solarized", name: "Solarized", description: "Balanced teal, blue, and amber",
		brand: "#2AA198", accent: "#268BD2", text: "#EEE8D5", muted: "#93A1A1",
		good: "#859900", warning: "#B58900", error: "#DC322F", selection: "#33464D", active: "#276B78",
		openText: "#B6C84A", openBackground: "#354532", draftText: "#E6BD55", draftBackground: "#51452C",
		mergedText: "#6CB8CF", mergedBackground: "#30434A", unreadText: "#6CB8CF", unreadBackground: "#30434A",
		addedText: "#A9C45B", addedBackground: "#2E4038", deletedText: "#E97870", deletedBackground: "#482F36",
		hunkText: "#6CB8CF", hunkBackground: "#30434A", fileText: "#EEE8D5", fileBackground: "#33464D",
		keyword: "#6CB8CF", codeString: "#B58900", number: "#2AA198", comment: "#93A1A1",
	},
}

var activeTheme theme

var (
	brandStyle       lipgloss.Style
	accentStyle      lipgloss.Style
	textStyle        lipgloss.Style
	mutedStyle       lipgloss.Style
	goodStyle        lipgloss.Style
	warnStyle        lipgloss.Style
	errorStyle       lipgloss.Style
	selectedStyle    lipgloss.Style
	activeStyle      lipgloss.Style
	openBadge        lipgloss.Style
	draftBadge       lipgloss.Style
	mergedBadge      lipgloss.Style
	unreadBadge      lipgloss.Style
	addLineStyle     lipgloss.Style
	delLineStyle     lipgloss.Style
	hunkLineStyle    lipgloss.Style
	fileLineStyle    lipgloss.Style
	syntaxKeyword    string
	syntaxString     string
	syntaxNumber     string
	syntaxComment    string
	themeAccentColor string
	themeTextColor   string
	themeBorderColor string
)

func init() { applyTheme(defaultThemeID) }

func themeByID(id string) (theme, bool) {
	for _, candidate := range themes {
		if candidate.id == id {
			return candidate, true
		}
	}
	return theme{}, false
}

func themeIndex(id string) int {
	for i, candidate := range themes {
		if candidate.id == id {
			return i
		}
	}
	return 0
}

func applyTheme(id string) string {
	selected, ok := themeByID(id)
	if !ok {
		selected, _ = themeByID(defaultThemeID)
	}
	activeTheme = selected
	color := func(value string) lipgloss.Color { return lipgloss.Color(value) }
	brandStyle = lipgloss.NewStyle().Bold(true).Foreground(color(selected.brand))
	accentStyle = lipgloss.NewStyle().Bold(true).Foreground(color(selected.accent))
	textStyle = lipgloss.NewStyle().Foreground(color(selected.text))
	mutedStyle = lipgloss.NewStyle().Foreground(color(selected.muted))
	goodStyle = lipgloss.NewStyle().Foreground(color(selected.good))
	warnStyle = lipgloss.NewStyle().Foreground(color(selected.warning))
	errorStyle = lipgloss.NewStyle().Bold(true).Foreground(color(selected.error))
	selectedStyle = lipgloss.NewStyle().Background(color(selected.selection)).Foreground(color(selected.text))
	activeStyle = lipgloss.NewStyle().Bold(true).Foreground(color(selected.text)).Background(color(selected.active))
	openBadge = lipgloss.NewStyle().Bold(true).Foreground(color(selected.openText)).Background(color(selected.openBackground))
	draftBadge = lipgloss.NewStyle().Bold(true).Foreground(color(selected.draftText)).Background(color(selected.draftBackground))
	mergedBadge = lipgloss.NewStyle().Bold(true).Foreground(color(selected.mergedText)).Background(color(selected.mergedBackground))
	unreadBadge = lipgloss.NewStyle().Bold(true).Foreground(color(selected.unreadText)).Background(color(selected.unreadBackground))
	addLineStyle = lipgloss.NewStyle().Foreground(color(selected.addedText)).Background(color(selected.addedBackground))
	delLineStyle = lipgloss.NewStyle().Foreground(color(selected.deletedText)).Background(color(selected.deletedBackground))
	hunkLineStyle = lipgloss.NewStyle().Foreground(color(selected.hunkText)).Background(color(selected.hunkBackground))
	fileLineStyle = lipgloss.NewStyle().Bold(true).Foreground(color(selected.fileText)).Background(color(selected.fileBackground))
	syntaxKeyword = selected.keyword
	syntaxString = selected.codeString
	syntaxNumber = selected.number
	syntaxComment = selected.comment
	themeAccentColor = selected.accent
	themeTextColor = selected.text
	themeBorderColor = selected.muted
	return selected.id
}
