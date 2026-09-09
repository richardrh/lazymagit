package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	gitbackend "github.com/richardrh/lazymagit/internal/git"
)

func TestThemePickerHotkeyChangesTheme(t *testing.T) {
	defer func() { _ = ApplyTheme("default") }()
	if err := ApplyTheme("default"); err != nil {
		t.Fatal(err)
	}
	m := New(&gitbackend.Repository{})
	m.width, m.height, m.loading = 100, 20, false

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyF2}))
	if cmd != nil || m.mode != modeTheme {
		t.Fatalf("F2 opened theme picker: mode=%v cmd=%v", m.mode, cmd)
	}
	if rendered := ansi.Strip(m.render()); !strings.Contains(rendered, "Change theme") || !strings.Contains(rendered, "Tokyo Night") {
		t.Fatalf("theme picker render omitted title or choices: %q", rendered)
	}
	original := m.themeName
	_, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if cmd != nil || m.themeCursor == 0 {
		t.Fatalf("theme picker down: cursor=%d cmd=%v", m.themeCursor, cmd)
	}
	_, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd != nil || m.mode != modeStatus || activeThemeName == original || !strings.Contains(m.message, "Theme changed") {
		t.Fatalf("theme selection: mode=%v theme=%q original=%q message=%q cmd=%v", m.mode, activeThemeName, original, m.message, cmd)
	}

	_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyF2}))
	if m.mode != modeTheme || m.themeName != activeThemeName {
		t.Fatalf("reopened picker did not select active theme: mode=%v selected=%q active=%q", m.mode, m.themeName, activeThemeName)
	}
	_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if m.mode != modeStatus {
		t.Fatalf("theme picker escape mode=%v", m.mode)
	}

}

func TestBundledThemesApplyAndRender(t *testing.T) {
	defer func() { _ = ApplyTheme("default") }()
	want := []string{"catppuccin-mocha", "default", "dracula", "gruvbox-dark", "nord", "solarized-dark", "tokyo-night"}
	if got := ThemeNames(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("theme names = %v, want %v", got, want)
	}
	for _, name := range want {
		if err := ApplyTheme(name); err != nil {
			t.Fatalf("ApplyTheme(%q): %v", name, err)
		}
		m := New(&gitbackend.Repository{})
		m.width, m.height, m.loading = 80, 18, false
		view := m.View().Content
		if !strings.Contains(ansi.Strip(view), "LAZYMAGIT") || !strings.Contains(view, "\x1b[") {
			t.Fatalf("theme %q did not render styled TUI", name)
		}
	}
	for _, alias := range []string{"catppuccin", "Catppuccin Mocha", "Tokyo Night", "gruvbox_dark"} {
		if err := ApplyTheme(alias); err != nil {
			t.Fatalf("ApplyTheme(%q) alias: %v", alias, err)
		}
	}
	if err := ApplyTheme("missing"); err == nil || !strings.Contains(err.Error(), "available themes") {
		t.Fatalf("unknown theme error = %v", err)
	}
}
