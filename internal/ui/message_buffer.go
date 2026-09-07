package ui

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	messageInsert = "INSERT"
	messageNormal = "NORMAL"
	messageVisual = "VISUAL"
	messageVLine  = "V-LINE"
)

type messageUndo struct {
	text   []rune
	cursor int
}

// MessageBuffer is a modal editor. Positions are rune offsets; rendering uses
// terminal display widths. Undo snapshots are taken per command/insert session,
// not per keystroke. The unnamed register distinguishes linewise from text edits.
type MessageBuffer struct {
	text                         []rune
	cursor, anchor, desired, top int
	mode                         string
	pending                      string
	operator                     string
	count, operatorCount         int
	register                     string
	linewise                     bool
	undo, redo                   []messageUndo
	editing                      bool
	baseline                     string
}

func NewMessageBuffer(text string) *MessageBuffer {
	b := new(MessageBuffer)
	b.SetText(text)
	return b
}

func (b *MessageBuffer) SetText(text string) {
	*b = MessageBuffer{text: []rune(safeMessageText(text)), mode: messageInsert, desired: -1}
	b.cursor = len(b.text)
	b.baseline = string(b.text)
}
func (b *MessageBuffer) Text() string   { return string(b.text) }
func (b *MessageBuffer) Mode() string   { return b.mode }
func (b *MessageBuffer) Modified() bool { return string(b.text) != b.baseline }
func (b *MessageBuffer) Cursor() (line, column int) {
	for _, r := range b.text[:b.cursor] {
		if r == '\n' {
			line++
			column = 0
		} else {
			column++
		}
	}
	return
}

func (b *MessageBuffer) bounds(pos int) (start, end int) {
	start, end = min(max(0, pos), len(b.text)), min(max(0, pos), len(b.text))
	for start > 0 && b.text[start-1] != '\n' {
		start--
	}
	for end < len(b.text) && b.text[end] != '\n' {
		end++
	}
	return
}
func (b *MessageBuffer) clamp() {
	b.cursor = min(max(0, b.cursor), len(b.text))
	if b.mode != messageInsert {
		start, end := b.bounds(b.cursor)
		if b.cursor == end && end > start {
			b.cursor--
		}
	}
}
func (b *MessageBuffer) remember() {
	if !b.editing {
		b.undo = append(b.undo, messageUndo{append([]rune(nil), b.text...), b.cursor})
		b.editing = true
	}
	b.redo = nil
}
func (b *MessageBuffer) replace(start, end int, text string) bool {
	if start == end && text == "" {
		return false
	}
	b.remember()
	add := []rune(text)
	oldLen := len(b.text)
	delta := len(add) - (end - start)
	if delta > 0 {
		b.text = append(b.text, make([]rune, delta)...)
	}
	copy(b.text[start+len(add):], b.text[end:oldLen])
	copy(b.text[start:], add)
	if delta < 0 {
		b.text = b.text[:oldLen+delta]
	}
	b.cursor, b.desired = start+len(add), -1
	return true
}
func (b *MessageBuffer) history(redo bool) bool {
	from, to := &b.undo, &b.redo
	if redo {
		from, to = to, from
	}
	if len(*from) == 0 {
		return false
	}
	*to = append(*to, messageUndo{append([]rune(nil), b.text...), b.cursor})
	last := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	b.text, b.cursor = last.text, last.cursor
	b.mode, b.editing, b.desired = messageNormal, false, -1
	b.clamp()
	return true
}

// HandleKey reports text changes. The owner must consume navigation keys too.
func (b *MessageBuffer) HandleKey(msg tea.KeyPressMsg) bool {
	defer b.clamp()
	key := msg.String()
	if b.mode == messageInsert {
		return b.insertKey(key, msg.Key().Text)
	}
	if key == "esc" {
		b.mode, b.pending, b.operator, b.count = messageNormal, "", "", 0
		b.editing = false
		return false
	}
	if len(key) == 1 && key[0] >= '0' && key[0] <= '9' && (key != "0" || b.count > 0) {
		b.count = min(10000, b.count*10+int(key[0]-'0'))
		return false
	}
	count, counted := max(1, b.count), b.count > 0
	b.count = 0
	if b.pending == "g" {
		b.pending = ""
		if key != "g" {
			b.operator = ""
			return false
		}
		key = "gg"
		if b.operatorCount > 0 && b.operator == "" {
			count = b.operatorCount
		}
	}
	if key == "g" {
		b.pending = "g"
		if b.operator == "" {
			b.operatorCount = count
		}
		return false
	}
	visual := b.mode == messageVisual || b.mode == messageVLine
	if visual && (key == "d" || key == "x" || key == "c" || key == "y") {
		a, z := b.selection()
		linewise := b.mode == messageVLine
		if key == "x" {
			key = "d"
		}
		return b.operate(key, a, z, linewise)
	}
	if b.operator != "" {
		op := b.operator
		b.operator = ""
		count = min(10000, count*max(1, b.operatorCount))
		if key == op {
			start, end := b.bounds(b.cursor)
			for i := 1; i < count && end < len(b.text); i++ {
				_, end = b.bounds(end + 1)
			}
			if end < len(b.text) {
				end++
			}
			return b.operate(op, start, end, true)
		}
		start := b.cursor
		if op == "c" && key == "w" && start < len(b.text) && !unicode.IsSpace(b.text[start]) {
			end := start
			for i := 1; i < count; i++ {
				end = b.word(end, "w")
			}
			if end >= len(b.text) {
				return b.operate(op, start, len(b.text), false)
			}
			class := messageWordClass(b.text[end])
			for end < len(b.text) && messageWordClass(b.text[end]) == class {
				end++
			}
			return b.operate(op, start, end, false)
		}
		if !b.motion(key, count, counted) {
			return false
		}
		end := b.cursor
		linewise := key == "j" || key == "k" || key == "gg" || key == "G"
		if linewise {
			low, high := min(start, end), max(start, end)
			start, _ = b.bounds(low)
			_, end = b.bounds(high)
			if end < len(b.text) {
				end++
			}
		} else {
			if start > end {
				start, end = end, start
			} else if key == "e" || key == "$" {
				end = min(end+1, len(b.text))
			}
		}
		b.cursor = start
		return b.operate(op, start, end, linewise)
	}
	if b.motion(key, count, counted) {
		b.editing = false
		return false
	}
	b.editing = false
	switch key {
	case "i":
		b.mode = messageInsert
	case "I":
		b.firstNonBlank()
		b.mode = messageInsert
	case "a":
		_, end := b.bounds(b.cursor)
		b.cursor = min(b.cursor+1, end)
		b.mode = messageInsert
	case "A":
		_, b.cursor = b.bounds(b.cursor)
		b.mode = messageInsert
	case "o":
		_, end := b.bounds(b.cursor)
		b.mode = messageInsert
		return b.replace(end, end, "\n")
	case "O":
		start, _ := b.bounds(b.cursor)
		b.mode = messageInsert
		changed := b.replace(start, start, "\n")
		b.cursor = start
		return changed
	case "x":
		_, end := b.bounds(b.cursor)
		return b.operate("d", b.cursor, min(b.cursor+count, end), false)
	case "X":
		start, _ := b.bounds(b.cursor)
		return b.operate("d", max(start, b.cursor-count), b.cursor, false)
	case "D", "C":
		_, end := b.bounds(b.cursor)
		op := "d"
		if key == "C" {
			op = "c"
		}
		return b.operate(op, b.cursor, end, false)
	case "d", "c", "y":
		b.operator = key
		b.operatorCount = count
	case "Y":
		start, end := b.bounds(b.cursor)
		for i := 1; i < count && end < len(b.text); i++ {
			_, end = b.bounds(end + 1)
		}
		if end < len(b.text) {
			end++
		}
		return b.operate("y", start, end, true)
	case "p", "P":
		return b.put(key == "P", count)
	case "u":
		return b.history(false)
	case "ctrl+r":
		return b.history(true)
	case "v":
		if b.mode == messageVisual {
			b.mode = messageNormal
		} else {
			if !visual {
				b.anchor = b.cursor
			}
			b.mode = messageVisual
		}
	case "V":
		if b.mode == messageVLine {
			b.mode = messageNormal
		} else {
			if !visual {
				b.anchor = b.cursor
			}
			b.mode = messageVLine
		}
	}
	return false
}

func (b *MessageBuffer) insertKey(key, text string) bool {
	switch key {
	case "esc":
		b.mode, b.editing = messageNormal, false
		start, _ := b.bounds(b.cursor)
		if b.cursor > start {
			b.cursor--
		}
		b.desired = -1
		return false
	case "enter", "ctrl+j", "shift+enter":
		return b.replace(b.cursor, b.cursor, "\n")
	case "backspace", "ctrl+h":
		if b.cursor > 0 {
			return b.replace(b.cursor-1, b.cursor, "")
		}
		return false
	case "delete":
		if b.cursor < len(b.text) {
			return b.replace(b.cursor, b.cursor+1, "")
		}
		return false
	case "left", "right", "up", "down", "home", "end":
		b.motion(key, 1, false)
		b.editing = false
		return false
	}
	if text != "" {
		return b.replace(b.cursor, b.cursor, safeMessageText(text))
	}
	return false
}

func (b *MessageBuffer) motion(key string, count int, counted bool) bool {
	start, end := b.bounds(b.cursor)
	switch key {
	case "h", "left":
		b.cursor = max(start, b.cursor-count)
		b.desired = -1
	case "l", "right":
		b.cursor = min(end, b.cursor+count)
		b.desired = -1
	case "0", "home":
		b.cursor = start
		b.desired = -1
	case "^":
		b.firstNonBlank()
		b.desired = -1
	case "$", "end":
		b.cursor = end
		if b.mode != messageInsert && end > start {
			b.cursor--
		}
		b.desired = -1
	case "j", "down", "k", "up", "ctrl+d", "ctrl+u", "ctrl+f", "ctrl+b":
		if b.desired < 0 {
			b.desired = b.cursor - start
		}
		if key == "ctrl+d" || key == "ctrl+u" {
			count *= 10
		}
		if key == "ctrl+f" || key == "ctrl+b" {
			count *= 20
		}
		back := key == "k" || key == "up" || key == "ctrl+u" || key == "ctrl+b"
		for range count {
			if back {
				if start == 0 {
					break
				}
				start, end = b.bounds(start - 1)
			} else {
				if end == len(b.text) {
					break
				}
				start, end = b.bounds(end + 1)
			}
		}
		b.cursor = min(start+b.desired, end)
	case "gg", "G":
		b.cursor = 0
		if key == "G" && !counted {
			b.cursor = len(b.text)
			b.cursor, _ = b.bounds(b.cursor)
		} else {
			for i := 1; i < count; i++ {
				_, end = b.bounds(b.cursor)
				if end == len(b.text) {
					break
				}
				b.cursor = end + 1
			}
		}
		b.desired = -1
	case "w", "b", "e":
		for range count {
			next := b.word(b.cursor, key)
			if next == b.cursor {
				break
			}
			b.cursor = next
		}
		b.desired = -1
	default:
		return false
	}
	return true
}
func (b *MessageBuffer) firstNonBlank() {
	start, end := b.bounds(b.cursor)
	for start < end && unicode.IsSpace(b.text[start]) {
		start++
	}
	b.cursor = start
}
func messageWordClass(r rune) int {
	if unicode.IsSpace(r) {
		return 0
	}
	if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
		return 1
	}
	return 2
}
func (b *MessageBuffer) word(pos int, key string) int {
	if len(b.text) == 0 {
		return 0
	}
	switch key {
	case "b":
		pos = max(0, pos-1)
		for pos > 0 && unicode.IsSpace(b.text[pos]) {
			pos--
		}
		class := messageWordClass(b.text[pos])
		for pos > 0 && messageWordClass(b.text[pos-1]) == class {
			pos--
		}
	case "w":
		if pos == len(b.text) {
			return pos
		}
		class := messageWordClass(b.text[pos])
		for pos < len(b.text) && messageWordClass(b.text[pos]) == class {
			pos++
		}
		for pos < len(b.text) && unicode.IsSpace(b.text[pos]) {
			pos++
		}
	case "e":
		pos = min(pos+1, len(b.text)-1)
		for pos < len(b.text)-1 && unicode.IsSpace(b.text[pos]) {
			pos++
		}
		class := messageWordClass(b.text[pos])
		for pos+1 < len(b.text) && messageWordClass(b.text[pos+1]) == class {
			pos++
		}
	}
	return pos
}
func (b *MessageBuffer) selection() (int, int) {
	a, z := min(b.anchor, b.cursor), max(b.anchor, b.cursor)
	if b.mode == messageVLine {
		a, _ = b.bounds(a)
		_, z = b.bounds(z)
		if z < len(b.text) {
			z++
		}
	} else {
		z = min(z+1, len(b.text))
	}
	return a, z
}
func (b *MessageBuffer) operate(op string, start, end int, linewise bool) bool {
	start, end = min(start, len(b.text)), min(end, len(b.text))
	b.register = string(b.text[start:end])
	b.linewise = linewise
	if linewise && !strings.HasSuffix(b.register, "\n") {
		b.register += "\n"
	}
	if op == "y" {
		b.cursor = start
		b.mode = messageNormal
		return false
	}
	b.editing = false
	if linewise {
		if op == "c" {
			if end > start && b.text[end-1] == '\n' {
				end--
			}
		} else if end == len(b.text) && start > 0 {
			start--
		}
	}
	changed := b.replace(start, end, "")
	b.mode = messageNormal
	if op == "c" {
		b.mode = messageInsert
	} else {
		b.editing = false
	}
	return changed
}
func (b *MessageBuffer) put(before bool, count int) bool {
	if b.register == "" {
		return false
	}
	text := strings.Repeat(b.register, count)
	pos := b.cursor
	if b.linewise {
		start, end := b.bounds(pos)
		if before {
			pos = start
		} else if end < len(b.text) {
			pos = end + 1
		} else {
			pos = end
			text = "\n" + strings.TrimSuffix(text, "\n")
		}
	} else if !before {
		_, end := b.bounds(pos)
		pos = min(pos+1, end)
	}
	changed := b.replace(pos, pos, text)
	if b.linewise {
		b.cursor = pos
		if len(text) > 0 && text[0] == '\n' {
			b.cursor++
		}
	} else {
		b.cursor = max(pos, b.cursor-1)
	}
	b.editing = false
	return changed
}

func (b *MessageBuffer) Paste(text string) {
	b.editing = false
	b.mode = messageInsert
	b.replace(b.cursor, b.cursor, safeMessageText(text))
	b.editing = false
}
func safeMessageText(text string) string {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	var out strings.Builder
	for _, r := range text {
		if r == '\n' || r == '\t' || r >= 32 && r != 127 {
			out.WriteRune(r)
		} else {
			out.WriteString(visibleRune(r))
		}
	}
	return out.String()
}

type messageDisplayRow struct {
	text   string
	cursor bool
}

// View soft-wraps without modifying the message, follows the active cursor in
// every mode, and reserves a real cell for an insertion caret at a wrap edge.
func (b *MessageBuffer) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	rows := b.displayRows(width)
	cursor := 0
	for i, row := range rows {
		if row.cursor {
			cursor = i
			break
		}
	}
	if cursor < b.top {
		b.top = cursor
	}
	if cursor >= b.top+height {
		b.top = cursor - height + 1
	}
	b.top = min(b.top, max(0, len(rows)-height))
	var out strings.Builder
	for i := range height {
		if i > 0 {
			out.WriteByte('\n')
		}
		if index := b.top + i; index < len(rows) {
			out.WriteString(rows[index].text)
			out.WriteString(strings.Repeat(" ", max(0, width-ansi.StringWidth(rows[index].text))))
		} else {
			out.WriteString(strings.Repeat(" ", width))
		}
	}
	return out.String()
}
func (b *MessageBuffer) displayRows(width int) []messageDisplayRow {
	var rows []messageDisplayRow
	var row strings.Builder
	used, hasCursor := 0, false
	a, z := 0, 0
	if b.mode == messageVisual || b.mode == messageVLine {
		a, z = b.selection()
	}
	reverse := lipgloss.NewStyle().Reverse(true)
	flush := func() {
		rows = append(rows, messageDisplayRow{row.String(), hasCursor})
		row.Reset()
		used = 0
		hasCursor = false
	}
	cell := func(text string, index int) {
		w := ansi.StringWidth(text)
		if w > width {
			text = "�"
			w = 1
		}
		if used+w > width {
			flush()
		}
		selected := index >= a && index < z
		if index == b.cursor {
			hasCursor = true
			selected = true
		}
		if selected {
			text = reverse.Render(text)
		}
		row.WriteString(text)
		used += w
	}
	for i, r := range b.text {
		if r == '\n' {
			if i == b.cursor || i >= a && i < z {
				cell(" ", i)
			}
			flush()
			continue
		}
		text := visibleRune(r)
		if r == '\t' {
			text = strings.Repeat(" ", min(width, 4-used%4))
		}
		cell(text, i)
	}
	if b.cursor == len(b.text) {
		cell(" ", len(b.text))
	}
	flush()
	return rows
}
