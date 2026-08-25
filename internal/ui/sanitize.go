package ui

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// sanitizeSingleLine makes repository-controlled values safe to place in a
// terminal without allowing them to create lines or terminal control
// sequences. Control pictures keep unusual file and ref names recognizable.
func sanitizeSingleLine(value string) string {
	var result strings.Builder
	for _, r := range value {
		result.WriteString(visibleRune(r))
	}
	return result.String()
}

// sanitizeDiff preserves the line boundaries supplied by git, but makes every
// line terminal-safe. Tabs are expanded to eight-column stops so indentation
// remains visible without retaining a C0 control byte.
func sanitizeDiff(value string) string {
	var result strings.Builder
	column := 0
	for i := 0; i < len(value); {
		r, size := utf8.DecodeRuneInString(value[i:])
		i += size
		switch r {
		case '\n':
			result.WriteByte('\n')
			column = 0
		case '\r':
			if i < len(value) && value[i] == '\n' {
				continue
			}
			text := visibleRune(r)
			result.WriteString(text)
			column += ansi.StringWidth(text)
		case '\t':
			spaces := 8 - column%8
			result.WriteString(strings.Repeat(" ", spaces))
			column += spaces
		default:
			text := visibleRune(r)
			result.WriteString(text)
			column += ansi.StringWidth(text)
		}
	}
	return result.String()
}

func visibleRune(r rune) string {
	switch {
	case r <= 0x1f:
		return string(rune(0x2400) + r)
	case r == 0x7f:
		return "␡"
	case r >= 0x80 && r <= 0x9f:
		return fmt.Sprintf("\\x%02X", r)
	case unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029':
		return fmt.Sprintf("\\u%04X", r)
	default:
		return string(r)
	}
}
