package ui

import (
	"strings"
	"testing"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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
	for _, mode := range []mode{modeCommit, modeBranches, modeConfirm, modeHelp, modeAddRemote, modeRemotes} {
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

func TestFooterPrioritizesWorkflowsAndUsesUppercasePush(t *testing.T) {
	m := New(&gitbackend.Repository{})
	for _, width := range []int{40, 120} {
		m.width = width
		footer := ansi.Strip(m.renderFooter())
		for _, want := range []string{"c Commit", "f Fetch", "P Push", "b Branch"} {
			if !strings.Contains(footer, want) {
				t.Fatalf("width %d footer omitted %q: %q", width, want, footer)
			}
		}
		if strings.Contains(footer, "p Push") {
			t.Fatalf("footer advertised lowercase push navigation collision: %q", footer)
		}
	}
	m.width, m.mode = 120, modeCommit
	if footer := ansi.Strip(m.renderFooter()); strings.Contains(footer, "c Commit") || strings.Contains(footer, "P Push") {
		t.Fatalf("modal footer misleadingly exposed intercepted globals: %q", footer)
	}
	m.mode = modeStatus
	if footer := ansi.Strip(m.renderFooter()); !strings.Contains(footer, "$ Processes") {
		t.Fatalf("wide status footer omitted processes: %q", footer)
	}
}

func TestNarrowFooterPrioritizesStatusErrors(t *testing.T) {
	m := New(&gitbackend.Repository{})
	m.mode, m.isError, m.message = modeStatus, true, "push rejected"
	for _, width := range []int{20, 30, 40} {
		m.width = width
		footer := ansi.Strip(m.renderFooter())
		if !strings.Contains(footer, "push rejected") {
			t.Fatalf("width %d hid meaningful status error: %q", width, footer)
		}
	}
}

func TestRemoteChooserKeepsSelectionAndActionAtShortHeights(t *testing.T) {
	for _, height := range []int{5, 7, 8, 16} {
		m := New(&gitbackend.Repository{})
		m.width, m.height, m.loading = 48, height, false
		m.mode, m.remotePurpose, m.remoteCursor = modeRemotes, remoteConfigureAndPush, 1
		m.snapshot.remotes = []gitbackend.Remote{{Name: "origin"}, {Name: "publish"}}
		got := m.render()
		if width, renderedHeight := lipgloss.Width(got), lipgloss.Height(got); width != 48 || renderedHeight != height {
			t.Fatalf("chooser 48x%d rendered %dx%d", height, width, renderedHeight)
		}
		plain := ansi.Strip(got)
		for _, want := range []string{"publish", "Enter choose destination"} {
			if !strings.Contains(plain, want) {
				t.Fatalf("height %d chooser omitted %q: %q", height, want, plain)
			}
		}
		if height == 5 && strings.Contains(plain, "origin") {
			t.Fatalf("short chooser showed an unselected remote instead of selected publish: %q", plain)
		}
		if height == 16 && !strings.Contains(plain, "Push and set upstream") {
			t.Fatalf("normal chooser omitted title: %q", plain)
		}
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
	m.install(snapshot{status: gitbackend.Status{Files: []gitbackend.FileStatus{{Path: "a", Unstaged: gitbackend.ChangeModified}}}})
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

func TestHelpUsesBorderlessMagitDispatcher(t *testing.T) {
	m := New(&gitbackend.Repository{})
	m.width, m.height, m.mode = 120, 30, modeHelp
	help := m.renderOverlay(26)
	if strings.ContainsAny(help, "╔╗╚╝║═") {
		t.Fatalf("help retained generic transient border: %q", help)
	}
	for _, heading := range []string{"Transient and dwim commands", "Applying changes", "Essential commands"} {
		if !strings.Contains(help, heading) {
			t.Fatalf("help omitted %q", heading)
		}
	}
}
