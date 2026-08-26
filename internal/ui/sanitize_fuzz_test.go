package ui

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

func FuzzSanitizeSingleLine(f *testing.F) {
	f.Add("normal\x1b[2J\nnext\u2028line")
	f.Add(string([]byte{0xff, 0x00, 0x9b}))
	f.Fuzz(func(t *testing.T, value string) {
		if len(value) > 64<<10 {
			t.Skip()
		}
		got := sanitizeSingleLine(value)
		assertTerminalSafe(t, got, false)
		if again := sanitizeSingleLine(got); again != got {
			t.Fatal("single-line sanitization is not idempotent")
		}
	})
}

func FuzzSanitizeDiff(f *testing.F) {
	f.Add("\x1b[31mred\x1b[0m\r\n\ttext\rmore\n")
	f.Add(string([]byte{0xff, '\n', 0x9b}))
	f.Fuzz(func(t *testing.T, value string) {
		if len(value) > 64<<10 {
			t.Skip()
		}
		got := sanitizeDiff(value)
		assertTerminalSafe(t, got, true)
		if strings.Count(got, "\n") != strings.Count(value, "\n") {
			t.Fatal("diff sanitizer changed line boundaries")
		}
		if again := sanitizeDiff(got); again != got {
			t.Fatal("diff sanitization is not idempotent")
		}
	})
}

func assertTerminalSafe(t *testing.T, value string, allowNewline bool) {
	t.Helper()
	if !utf8.ValidString(value) {
		t.Fatal("sanitizer returned invalid UTF-8")
	}
	for _, r := range value {
		if allowNewline && r == '\n' {
			continue
		}
		if r <= 0x1f || r == 0x7f || r >= 0x80 && r <= 0x9f || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			t.Fatalf("unsafe terminal rune U+%04X", r)
		}
	}
}
