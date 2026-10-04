package ui

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// updateGolden rewrites the files under internal/ui/testdata. It is a deliberate
// developer action, never part of `make check`.
var updateGolden = flag.Bool("update", false, "rewrite internal/ui/testdata golden files")

// goldenWidth and goldenHeight are the terminal geometry the goldens are
// rendered at. Fixing it makes column alignment and truncation part of what a
// golden pins.
const (
	goldenWidth  = 100
	goldenHeight = 30
)

var (
	goldenObjectID = regexp.MustCompile(`\b[0-9a-f]{7,40}\b`)
	// The detail pane truncates horizontally, so a wide object ID can be cut
	// mid-token. Masking only whole IDs would leave the cut fragment, which
	// differs on every run because the commit hashes themselves differ.
	goldenCutOID   = regexp.MustCompile(`[0-9a-f]{4,40}…`)
	goldenISODate  = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)
	goldenStamp    = regexp.MustCompile(`[A-Z][a-z]{2} [A-Z][a-z]{2} +\d+ \d{2}:\d{2}:\d{2} \d{4} [+-]\d{4}`)
	goldenRelative = regexp.MustCompile(`\d+ (second|minute|hour|day|week|month|year)s? ago`)
)

// normalizeGolden masks the values that change between runs — commit object IDs
// and timestamps — and pins everything else. That is what makes a golden worth
// more than a substring assertion: the gutter, the column alignment, the
// selected row, the panel borders, and the footer are asserted byte for byte,
// and any of them regressing fails the test.
func normalizeGolden(plain string) string {
	plain = goldenObjectID.ReplaceAllString(plain, "[OID]")
	plain = goldenCutOID.ReplaceAllString(plain, "[OID]…")
	plain = goldenStamp.ReplaceAllString(plain, "[STAMP]")
	plain = goldenISODate.ReplaceAllString(plain, "[DATE]")
	plain = goldenRelative.ReplaceAllString(plain, "[AGO]")
	return strings.TrimRight(plain, "\n") + "\n"
}

// renderGoldenAt renders the model at a fixed geometry and normalizes it.
func renderGoldenAt(m *Model, width, height int) string {
	_, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return normalizeGolden(ansi.Strip(m.render()))
}

// assertGolden compares a rendered view against its checked-in golden.
func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create golden directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", name, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run: go test ./internal/ui -update)", name, err)
	}
	if got != string(want) {
		t.Fatalf("golden %s mismatch:\n--- want ---\n%s--- got ---\n%s", name, want, got)
	}
}

// TestLogTabGoldens pins the Log tab's rendered surface. Substring assertions
// cannot catch a column that drifts, a gutter that stops opening, or a footer
// that loses a key; a golden can.
func TestLogTabGoldens(t *testing.T) {
	t.Run("open", func(t *testing.T) {
		m := newInspectE2EModel(t)
		sendInspectSequence(t, m, "alt+l")
		if !m.logTab {
			t.Fatalf("alt+l did not open the Log tab: %q", m.message)
		}
		assertGolden(t, "logtab-open", renderGoldenAt(m, goldenWidth, goldenHeight))
	})

	t.Run("marked", func(t *testing.T) {
		m := newInspectE2EModel(t)
		sendInspectSequence(t, m, "alt+l")
		sendInspectSequence(t, m, "j", "alt+m")
		assertGolden(t, "logtab-marked", renderGoldenAt(m, goldenWidth, goldenHeight))
	})

	t.Run("compared", func(t *testing.T) {
		m := newInspectE2EModel(t)
		sendInspectSequence(t, m, "alt+l")
		sendInspectSequence(t, m, "alt+m", "j", "alt+m", "alt+c")
		assertGolden(t, "logtab-compared", renderGoldenAt(m, goldenWidth, goldenHeight))
	})

	t.Run("narrow", func(t *testing.T) {
		m := newInspectE2EModel(t)
		sendInspectSequence(t, m, "alt+l")
		assertGolden(t, "logtab-narrow", renderGoldenAt(m, 60, 16))
	})

	t.Run("status", func(t *testing.T) {
		m := newInspectE2EModel(t)
		assertGolden(t, "status-tab", renderGoldenAt(m, goldenWidth, goldenHeight))
	})
}
