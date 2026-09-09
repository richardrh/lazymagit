package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func messageKey(text string) tea.KeyPressMsg {
	if text == "enter" {
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	}
	if text == "esc" {
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})
	}
	if text == "backspace" {
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace})
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

	b := NewMessageBuffer("one two\nthree")
	for _, key := range []string{"esc", "g", "x", "g", "g", "2", "d", "w"} {
		b.HandleKey(messageKey(key))
	}
	if b.Text() != "three" {
		t.Fatalf("invalid g prefix or counted delete changed text incorrectly: %q", b.Text())
	}
	for _, key := range []string{"0", "^", "$", "end", "j", "down", "k", "up", "ctrl+d", "ctrl+u", "ctrl+f", "ctrl+b", "G", "g", "g", "w", "b", "e"} {
		b.HandleKey(messageKey(key))
	}
	if line, column := b.Cursor(); line < 0 || column < 0 {
		t.Fatalf("motion produced invalid cursor %d:%d", line, column)
	}
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
