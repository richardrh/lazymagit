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
