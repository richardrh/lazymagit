package ui

import (
	"fmt"
	"image/color"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
)

type Palette struct {
	Purple, Cyan, Green, Red, Gold, Muted, Border, Text, OnAccent string
}

var themes = map[string]Palette{
	"default": {
		Purple: "#A78BFA", Cyan: "#67E8F9", Green: "#86EFAC", Red: "#FDA4AF", Gold: "#FDE68A",
		Muted: "#94A3B8", Border: "#475569", Text: "#CBD5E1", OnAccent: "#0F172A",
	},
	"tokyo-night": {
		Purple: "#BB9AF7", Cyan: "#7DCFFF", Green: "#9ECE6A", Red: "#F7768E", Gold: "#E0AF68",
		Muted: "#787C99", Border: "#3B4261", Text: "#C0CAF5", OnAccent: "#1A1B26",
	},
	"catppuccin-mocha": {
		Purple: "#CBA6F7", Cyan: "#89DCEB", Green: "#A6E3A1", Red: "#F38BA8", Gold: "#F9E2AF",
		Muted: "#7F849C", Border: "#45475A", Text: "#CDD6F4", OnAccent: "#1E1E2E",
	},
	"nord": {
		Purple: "#B48EAD", Cyan: "#88C0D0", Green: "#A3BE8C", Red: "#BF616A", Gold: "#EBCB8B",
		Muted: "#7B88A1", Border: "#4C566A", Text: "#D8DEE9", OnAccent: "#2E3440",
	},
	"dracula": {
		Purple: "#BD93F9", Cyan: "#8BE9FD", Green: "#50FA7B", Red: "#FF5555", Gold: "#F1FA8C",
		Muted: "#6272A4", Border: "#44475A", Text: "#F8F8F2", OnAccent: "#282A36",
	},
	"gruvbox-dark": {
		Purple: "#D3869B", Cyan: "#83A598", Green: "#B8BB26", Red: "#FB4934", Gold: "#FABD2F",
		Muted: "#928374", Border: "#504945", Text: "#EBDBB2", OnAccent: "#282828",
	},
	"solarized-dark": {
		Purple: "#6C71C4", Cyan: "#2AA198", Green: "#859900", Red: "#DC322F", Gold: "#B58900",
		Muted: "#657B83", Border: "#586E75", Text: "#839496", OnAccent: "#002B36",
	},
}

var (
	colorPurple   color.Color
	colorCyan     color.Color
	colorGreen    color.Color
	colorRed      color.Color
	colorGold     color.Color
	colorMuted    color.Color
	colorBorder   color.Color
	colorText     color.Color
	colorOnAccent color.Color
)

func init() { _ = ApplyTheme("default") }

func ThemeNames() []string {
	names := make([]string, 0, len(themes))
	for name := range themes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func ApplyTheme(name string) error {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.Join(strings.Fields(name), "-")
	if name == "catppuccin" {
		name = "catppuccin-mocha"
	}
	palette, ok := themes[name]
	if !ok {
		return fmt.Errorf("unknown theme %q; available themes: %s", name, strings.Join(ThemeNames(), ", "))
	}
	colorPurple = lipgloss.Color(palette.Purple)
	colorCyan = lipgloss.Color(palette.Cyan)
	colorGreen = lipgloss.Color(palette.Green)
	colorRed = lipgloss.Color(palette.Red)
	colorGold = lipgloss.Color(palette.Gold)
	colorMuted = lipgloss.Color(palette.Muted)
	colorBorder = lipgloss.Color(palette.Border)
	colorText = lipgloss.Color(palette.Text)
	colorOnAccent = lipgloss.Color(palette.OnAccent)
	return nil
}
