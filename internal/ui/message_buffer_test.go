package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func messageKey(text string) tea.KeyPressMsg {
	switch text {
	case "enter":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	case "esc":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})
	case "backspace":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace})
	case "left", "right", "up", "down", "home", "end":
		return tea.KeyPressMsg(tea.Key{Code: map[string]rune{"left": tea.KeyLeft, "right": tea.KeyRight, "up": tea.KeyUp, "down": tea.KeyDown, "home": tea.KeyHome, "end": tea.KeyEnd}[text]})
	case "ctrl+d", "ctrl+u", "ctrl+f", "ctrl+b", "ctrl+s":
		r := []rune(text)
		return tea.KeyPressMsg(tea.Key{Code: r[5], Mod: tea.ModCtrl})
	case "alt+d", "alt+k":
		r := []rune(text)
		return tea.KeyPressMsg(tea.Key{Code: r[4], Mod: tea.ModAlt})
	}
	r := []rune(text)
	return tea.KeyPressMsg(tea.Key{Code: r[0], Text: text})

}

func TestMessageBufferUnicodeInsertionPasteAndUndo(t *testing.T) {
	b := NewMessageBuffer("αβ")
	if line, col := b.Cursor(); line != 0 || col != 2 {
		t.Fatalf("initial cursor = %d,%d", line, col)
	}
	if !b.HandleKey(messageKey("backspace")) || b.Text() != "α" {
		t.Fatalf("unicode backspace: %q", b.Text())
	}
	b.HandleKey(messageKey("esc"))
	if !b.HandleKey(tea.KeyPressMsg(tea.Key{Code: 'u', Text: "u"})) || b.Text() != "αβ" {
		t.Fatalf("undo: %q", b.Text())
	}

	b.SetText("")
	b.Paste("x\n世界\x1b")
	if got, want := b.Text(), "x\n世界␛"; got != want {
		t.Fatalf("safe paste = %q, want %q", got, want)
	}
	if !b.Modified() {
		t.Fatal("paste did not mark buffer modified")
	}
}

func TestMessageBufferLinewiseAndCharacterwiseOperations(t *testing.T) {
	b := NewMessageBuffer("one\ntwo")
	b.HandleKey(messageKey("esc"))
	b.HandleKey(messageKey("g"))
	b.HandleKey(messageKey("g"))
	b.HandleKey(messageKey("d"))
	if !b.HandleKey(messageKey("d")) {
		t.Fatal("dd was not a change")
	}
	if b.Text() != "two" {
		t.Fatalf("dd = %q", b.Text())
	}
	b.HandleKey(messageKey("p"))
	if b.Text() != "two\none" {
		t.Fatalf("linewise p = %q", b.Text())
	}

	b.SetText("abcd")
	b.HandleKey(messageKey("esc"))
	b.HandleKey(messageKey("0"))
	b.HandleKey(messageKey("v"))
	b.HandleKey(messageKey("l"))
	b.HandleKey(messageKey("l"))
	b.HandleKey(messageKey("y"))
	b.HandleKey(messageKey("p"))
	if b.Text() != "aabcbcd" {
		t.Fatalf("characterwise visual yank/paste = %q", b.Text())
	}
}

func TestMessageBufferWrappedViewAndVisualSelection(t *testing.T) {
	b := NewMessageBuffer("界界界abc")
	view := b.View(4, 3)
	lines := strings.Split(view, "\n")
	if len(lines) != 3 {
		t.Fatalf("view rows = %d", len(lines))
	}
	for i, line := range lines {
		if ansi.StringWidth(line) != 4 {
			t.Fatalf("view row %d width = %d (%q)", i, ansi.StringWidth(line), line)
		}
	}
	if !strings.Contains(view, "\x1b[") {
		t.Fatal("view omitted visible caret styling")
	}
	b.HandleKey(messageKey("esc"))
	b.HandleKey(messageKey("0"))
	b.HandleKey(messageKey("v"))
	b.HandleKey(messageKey("l"))
	if b.Mode() != "VISUAL" || !strings.Contains(b.View(20, 1), "\x1b[") {
		t.Fatalf("visual selection not rendered")
	}
	if b.HandleKey(messageKey("d")) == false || b.Text() != "界abc" {
		t.Fatalf("visual delete = %q", b.Text())
	}
}

func TestMessageBufferWordChangesAndUndoStayModal(t *testing.T) {
	b := NewMessageBuffer("one two\nthree")
	for _, key := range []string{"esc", "g", "g", "c", "w", "NEW", "esc", "u"} {
		b.HandleKey(messageKey(key))
	}
	if b.Text() != "one two\nthree" || b.Mode() != "NORMAL" {
		t.Fatalf("undo must restore a whole change and stay normal: %q %s", b.Text(), b.Mode())
	}
	b.HandleKey(tea.KeyPressMsg(tea.Key{Code: 'r', Mod: tea.ModCtrl}))
	if b.Text() != "NEW two\nthree" {
		t.Fatalf("redo changed wrong range: %q", b.Text())
	}
	b.SetText("one two three")
	for _, key := range []string{"esc", "g", "g", "2", "d", "w"} {
		b.HandleKey(messageKey(key))
	}
	if b.Text() != "three" {
		t.Fatalf("2dw did not remove two words and separators: %q", b.Text())
	}
}

func TestMessageBufferViewportFollowsCursorPastBlankLines(t *testing.T) {
	b := NewMessageBuffer("SUBJECT\n\n" + strings.Repeat("body text\n", 25) + "END")
	if view := ansi.Strip(b.View(12, 3)); !strings.Contains(view, "END") {
		t.Fatalf("cursor below viewport:\n%s", view)
	}
	for _, key := range []string{"esc", "g", "g"} {
		b.HandleKey(messageKey(key))
	}
	if view := ansi.Strip(b.View(12, 3)); !strings.Contains(view, "SUBJECT") {
		t.Fatalf("cursor above viewport:\n%s", view)
	}
}

func TestMessageBufferNormalCommandsAndMotionBoundaries(t *testing.T) {
	tests := []struct {
		name string
		text string
		keys []string
		want string
	}{
		{name: "insert", text: "abc", keys: []string{"esc", "0", "i", "X", "esc"}, want: "Xabc"},
		{name: "insert at first nonblank", text: "  abc", keys: []string{"esc", "0", "I", "X", "esc"}, want: "  Xabc"},
		{name: "append", text: "abc", keys: []string{"esc", "0", "a", "X", "esc"}, want: "aXbc"},
		{name: "append at end", text: "abc", keys: []string{"esc", "A", "X", "esc"}, want: "abcX"},
		{name: "open below", text: "abc", keys: []string{"esc", "o", "X", "esc"}, want: "abc\nX"},
		{name: "open above", text: "abc", keys: []string{"esc", "O", "X", "esc"}, want: "X\nabc"},
		{name: "delete middle", text: "abc", keys: []string{"esc", "0", "l", "x"}, want: "ac"},
		{name: "delete right", text: "abc", keys: []string{"esc", "0", "x"}, want: "bc"},
		{name: "delete left", text: "abc", keys: []string{"esc", "0", "l", "X"}, want: "bc"},
		{name: "delete to end", text: "abc\ndef", keys: []string{"esc", "0", "D"}, want: "abc\n"},
		{name: "line yank and put", text: "one\ntwo", keys: []string{"esc", "g", "g", "Y", "p"}, want: "one\none\ntwo"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := NewMessageBuffer(test.text)
			for _, key := range test.keys {
				b.HandleKey(messageKey(key))
			}
			if got := b.Text(); got != test.want {
				t.Fatalf("text = %q, want %q", got, test.want)
			}
		})
	}

	motionCases := []struct {
		name  string
		text  string
		setup []string
		key   string
		line  int
		col   int
	}{
		{name: "line start", text: "one\ntwo", key: "0", line: 1, col: 0},
		{name: "line end", text: "one\ntwo", key: "$", line: 1, col: 2},
		{name: "first nonblank", text: "  one\ntwo", setup: []string{"g", "g"}, key: "^", line: 0, col: 2},
		{name: "next line", text: "one\ntwo\nthree", setup: []string{"g", "g"}, key: "j", line: 1, col: 0},
		{name: "previous line", text: "one\ntwo", key: "k", line: 0, col: 2},
		{name: "counted next line", text: "one\ntwo\nthree", setup: []string{"g", "g", "2"}, key: "j", line: 2, col: 0},
		{name: "first line", text: "one\ntwo", key: "gg", line: 0, col: 0},
		{name: "last line", text: "one\ntwo", key: "G", line: 1, col: 0},
		{name: "next word", text: "one two\nthree", setup: []string{"g", "g"}, key: "w", line: 0, col: 4},
		{name: "previous word", text: "one two\nthree", setup: []string{"g", "g", "w"}, key: "b", line: 0, col: 0},
		{name: "word end", text: "one two\nthree", setup: []string{"g", "g"}, key: "e", line: 0, col: 2},
	}
	for _, test := range motionCases {
		t.Run(test.name, func(t *testing.T) {
			b := NewMessageBuffer(test.text)
			b.HandleKey(messageKey("esc"))
			for _, key := range test.setup {
				b.HandleKey(messageKey(key))
			}
			b.HandleKey(messageKey(test.key))
			if line, col := b.Cursor(); line != test.line || col != test.col {
				t.Fatalf("cursor = %d:%d, want %d:%d", line, col, test.line, test.col)
			}
		})
	}

	page := NewMessageBuffer(strings.TrimSuffix(strings.Repeat("line\n", 30), "\n"))
	page.HandleKey(messageKey("esc"))
	page.HandleKey(messageKey("ctrl+u"))
	if line, _ := page.Cursor(); line != 19 {
		t.Fatalf("ctrl+u moved to line %d, want 19", line)
	}
	page.HandleKey(messageKey("ctrl+d"))
	if line, _ := page.Cursor(); line != 29 {
		t.Fatalf("ctrl+d moved to line %d, want 29", line)
	}
	page.HandleKey(messageKey("ctrl+b"))
	if line, _ := page.Cursor(); line != 9 {
		t.Fatalf("ctrl+b moved to line %d, want 9", line)
	}
	page.HandleKey(messageKey("ctrl+f"))
	if line, _ := page.Cursor(); line != 29 {
		t.Fatalf("ctrl+f moved to line %d, want 29", line)
	}

	invalid := NewMessageBuffer("one two\nthree")
	invalid.HandleKey(messageKey("esc"))
	invalid.HandleKey(messageKey("g"))
	invalid.HandleKey(messageKey("x"))
	if invalid.Text() != "one two\nthree" {
		t.Fatalf("invalid prefix changed text: %q", invalid.Text())
	}

	insert := NewMessageBuffer("ab")
	insert.HandleKey(messageKey("enter"))
	if insert.Text() != "ab\n" {
		t.Fatalf("enter insertion = %q", insert.Text())
	}
	insert.SetText("ab")
	insert.HandleKey(messageKey("delete"))
	if insert.Text() != "ab" {
		t.Fatalf("delete at end changed text: %q", insert.Text())
	}
	insert.HandleKey(messageKey("left"))
	if _, col := insert.Cursor(); col != 1 {
		t.Fatalf("left insertion motion moved to column %d, want 1", col)
	}

	b := NewMessageBuffer("one two\nthree")
	b.HandleKey(messageKey("esc"))
	b.HandleKey(messageKey("v"))
	b.HandleKey(messageKey("V"))
	if b.Mode() != "V-LINE" {
		t.Fatalf("V did not enter linewise visual mode: %s", b.Mode())
	}
	b.HandleKey(messageKey("V"))
	if b.Mode() != "NORMAL" {
		t.Fatalf("V did not leave linewise visual mode: %s", b.Mode())
	}
}
