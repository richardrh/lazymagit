package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	gitbackend "github.com/richard/lazymagit/internal/git"
)

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
	if err := ApplyTheme("catppuccin"); err != nil {
		t.Fatalf("catppuccin alias: %v", err)
	}
	if err := ApplyTheme("missing"); err == nil || !strings.Contains(err.Error(), "available themes") {
		t.Fatalf("unknown theme error = %v", err)
	}
}
