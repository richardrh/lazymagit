package ui

import (
	"strings"
	"testing"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	gitbackend "github.com/richard/lazymagit/internal/git"
)

func TestRepositoryTextSanitizers(t *testing.T) {
	single := sanitizeSingleLine("name\x1b[31m\nnext\t\u0085")
	if strings.ContainsAny(single, "\x1b\n\t\u0085") {
		t.Fatalf("single-line value retained terminal controls: %q", single)
	}
	if strings.Contains(single, "[31m") && strings.ContainsRune(single, '\x1b') {
		t.Fatalf("single-line value retained an ANSI sequence: %q", single)
	}

	diff := sanitizeDiff(" one\tcolumn\r\n+two\x1b[2J\n-three\u009b31m")
	if got, want := strings.Count(diff, "\n"), 2; got != want {
		t.Fatalf("diff newlines = %d, want %d in %q", got, want, diff)
	}
	if strings.ContainsRune(diff, '\t') || strings.ContainsRune(diff, '\r') || strings.ContainsRune(diff, '\x1b') {
		t.Fatalf("diff retained unsafe controls or an invisible tab: %q", diff)
	}
	for _, r := range diff {
		if r != '\n' && unicode.IsControl(r) {
			t.Fatalf("diff retained control %U in %q", r, diff)
		}
	}
}

func TestRenderDimensionsAtLayoutBoundaries(t *testing.T) {
	for _, width := range []int{20, 40, 95, 96, 97} {
		for _, height := range []int{5, 6, 7, 8, 12, 24} {
			m := New(&gitbackend.Repository{})
			m.width, m.height = width, height
			got := m.render()
			if gotWidth, gotHeight := lipgloss.Width(got), lipgloss.Height(got); gotWidth != width || gotHeight != height {
				t.Errorf("render %dx%d has ANSI-aware dimensions %dx%d", width, height, gotWidth, gotHeight)
			}
		}
	}
}

func TestOverlayDimensionsAtBoundaryHeights(t *testing.T) {
	for _, mode := range []mode{modeCommit, modeBranches, modeConfirm, modeHelp} {
		for _, height := range []int{5, 7, 12} {
			m := New(&gitbackend.Repository{})
			m.width, m.height, m.mode = 95, height, mode
			got := m.render()
			if width, gotHeight := lipgloss.Width(got), lipgloss.Height(got); width != 95 || gotHeight != height {
				t.Errorf("mode %d render 95x%d has ANSI-aware dimensions %dx%d", mode, height, width, gotHeight)
			}
		}
	}
}

func TestPanelDimensionsAreOuterDimensions(t *testing.T) {
	m := New(&gitbackend.Repository{})
	for _, size := range [][2]int{{20, 3}, {40, 7}, {48, 20}} {
		for name, got := range map[string]string{
			"status": m.renderStatusPanel(size[0], size[1]),
			"detail": m.renderDetailPanel(size[0], size[1]),
		} {
			if w, h := lipgloss.Width(got), lipgloss.Height(got); w != size[0] || h != size[1] {
				t.Errorf("%s panel requested %dx%d, got %dx%d", name, size[0], size[1], w, h)
			}
		}
	}
}

func TestFooterShowsActiveKeyScheme(t *testing.T) {
	m := New(&gitbackend.Repository{})
	m.width = 120
	if footer := m.renderFooter(); !strings.Contains(footer, "[Vim]") || !strings.Contains(footer, "x discard") {
		t.Fatalf("Vim footer is not explicit and accurate: %q", footer)
	}
	m.scheme = schemeMagit
	if footer := m.renderFooter(); !strings.Contains(footer, "[Magit]") || !strings.Contains(footer, "x reserved") {
		t.Fatalf("Magit footer is not explicit and accurate: %q", footer)
	}
}

func TestDetailScrollingIsIndependentAndClamped(t *testing.T) {
	m := New(&gitbackend.Repository{})
	m.width, m.height = 100, 14
	m.detail = strings.Join([]string{"zero", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "eleven"}, "\n")
	cursor := m.tree.Cursor()

	_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
	if m.detailOffset == 0 || m.tree.Cursor() != cursor {
		t.Fatal("PageDown did not scroll detail independently")
	}
	down := m.detailOffset
	_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'u', Mod: tea.ModCtrl}))
	if m.detailOffset >= down {
		t.Fatal("Ctrl-u did not scroll detail up")
	}
	_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace, Text: " ", Mod: tea.ModShift}))
	if m.detailOffset != 0 {
		t.Fatal("Shift-Space did not clamp detail scrolling at the top")
	}
}

func TestDetailOffsetResetsWhenSelectionOrDetailChanges(t *testing.T) {
	m := New(&gitbackend.Repository{})
	m.detailOffset = 8
	_ = m.move(1)
	if m.detailOffset != 0 {
		t.Fatal("selection change did not reset detail offset")
	}
	m.detailOffset = 8
	_, _ = m.Update(diffMsg{request: m.detailRequest, id: m.tree.Cursor(), text: "new detail"})
	if m.detailOffset != 0 {
		t.Fatal("loaded detail change did not reset detail offset")
	}
}

func TestHelpDocumentsDetailScrolling(t *testing.T) {
	m := New(&gitbackend.Repository{})
	m.width, m.height, m.mode = 120, 30, modeHelp
	help := m.renderOverlay(26)
	for _, key := range []string{"Space/PageDown/Ctrl-d", "Shift-Space/PageUp/Ctrl-u"} {
		if !strings.Contains(help, key) {
			t.Fatalf("help omitted detail scrolling key %q", key)
		}
	}
}
