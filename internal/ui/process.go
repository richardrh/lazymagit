package ui

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	gitbackend "github.com/richard/lazymagit/internal/git"
)

const (
	maxProcessBatches      = 32
	maxProcessStreamBytes  = 64 << 10
	maxProcessHistoryBytes = 256 << 10
	maxProcessRecordsBatch = 64
	processTruncated       = "\n[... process output truncated ...]\n"
)

type processBatch struct{ text string }

func cloneProcessRecord(record gitbackend.ProcessRecord) gitbackend.ProcessRecord {
	record.Args = append([]string(nil), record.Args...)
	return record
}

func (m *Model) appendProcessBatch(name string, records []gitbackend.ProcessRecord, opErr error) {
	text := formatProcessBatch(name, records, opErr)
	text = truncateProcessText(text, maxProcessHistoryBytes)
	m.processBatches = append(m.processBatches, processBatch{text: text})
	for len(m.processBatches) > maxProcessBatches || processHistorySize(m.processBatches) > maxProcessHistoryBytes {
		m.processBatches = m.processBatches[1:]
	}
	m.clampProcessOffset()
}

func processHistorySize(batches []processBatch) int {
	total := max(0, len(batches)-1) * 2
	for _, batch := range batches {
		total += len(batch.text)
	}
	return total
}

func (m *Model) processTranscript() string {
	parts := make([]string, len(m.processBatches))
	for i, batch := range m.processBatches {
		parts[i] = batch.text
	}
	return strings.Join(parts, "\n\n")
}

func formatProcessBatch(name string, records []gitbackend.ProcessRecord, opErr error) string {
	records = append([]gitbackend.ProcessRecord(nil), records...)
	sort.SliceStable(records, func(i, j int) bool { return records[i].Started.Before(records[j].Started) })
	status := "complete"
	if opErr != nil {
		status = "failed"
	}
	var out strings.Builder
	out.WriteString("== " + sanitizeSingleLine(name) + " — " + status + " ==")
	limit := min(len(records), maxProcessRecordsBatch)
	for _, record := range records[:limit] {
		out.WriteString("\n$ git -C ")
		out.WriteString(humanQuote(record.Dir))
		for _, arg := range record.Args {
			out.WriteByte(' ')
			out.WriteString(humanQuote(arg))
		}
		out.WriteString("\nexit ")
		out.WriteString(strconv.Itoa(record.ExitCode))
		out.WriteString(" · ")
		out.WriteString(formatProcessDuration(record.Duration))
		if record.Stdout != "" {
			out.WriteString("\nstdout:\n")
			out.WriteString(boundedProcessOutput(record.Stdout))
		}
		if record.Stderr != "" {
			out.WriteString("\nstderr:\n")
			out.WriteString(boundedProcessOutput(record.Stderr))
		}
	}
	if len(records) > limit {
		out.WriteString("\n[... additional process records truncated ...]")
	}
	if opErr != nil && !processErrorRecordedOnStderr(opErr, records) {
		out.WriteString("\nerror:\n")
		out.WriteString(boundedProcessOutput(opErr.Error()))
	}
	return strings.TrimSuffix(out.String(), "\n")
}

func processErrorRecordedOnStderr(opErr error, records []gitbackend.ProcessRecord) bool {
	candidates := []string{strings.TrimSpace(sanitizeDiff(opErr.Error()))}
	var commandErr *gitbackend.CommandError
	if errors.As(opErr, &commandErr) && commandErr.Stderr != "" {
		candidates = append(candidates, strings.TrimSpace(sanitizeDiff(commandErr.Stderr)))
	}
	for _, record := range records {
		stderr := strings.TrimSpace(sanitizeDiff(record.Stderr))
		if stderr == "" {
			continue
		}
		for _, candidate := range candidates {
			if candidate != "" && stderr == candidate {
				return true
			}
		}
	}
	return false
}

func formatProcessDuration(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	return strconv.FormatInt(duration.Round(time.Millisecond).Milliseconds(), 10) + "ms"
}

func boundedProcessOutput(value string) string {
	return truncateProcessText(sanitizeDiff(value), maxProcessStreamBytes)
}

func truncateProcessText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	keep := max(0, limit-len(processTruncated))
	head := keep / 2
	for head > 0 && !utf8.RuneStart(value[head]) {
		head--
	}
	tail := len(value) - (keep - head)
	for tail < len(value) && !utf8.RuneStart(value[tail]) {
		tail++
	}
	return value[:head] + processTruncated + value[tail:]
}

// humanQuote is a deterministic display representation. It makes argument
// boundaries unambiguous; the transcript is descriptive and is never executed.
func humanQuote(value string) string {
	if value != "" {
		safe := true
		for _, r := range value {
			if r > unicode.MaxASCII || !(unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("/._-+:=@", r)) {
				safe = false
				break
			}
		}
		if safe {
			return value
		}
	}
	return "'" + strings.ReplaceAll(sanitizeSingleLine(value), "'", "'\\''") + "'"
}

func (m *Model) openProcesses() {
	m.cancelPrefix()
	m.mode = modeProcess
	m.scrollProcessesToEnd()
}

func (m *Model) closeProcesses() { m.mode = modeStatus }

func (m *Model) handleProcessKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "esc", "$":
		m.closeProcesses()
	case "up":
		m.processOffset--
		m.clampProcessOffset()
	case "down":
		m.processOffset++
		m.clampProcessOffset()
	case "pgup":
		m.processOffset -= max(1, m.processViewportHeight())
		m.clampProcessOffset()
	case "pgdown":
		m.processOffset += max(1, m.processViewportHeight())
		m.clampProcessOffset()
	case "y":
		transcript := m.processTranscript()
		if transcript == "" {
			m.setMessage("No process output to copy")
			return m, nil
		}
		m.setMessage("Clipboard copy requested")
		return m, tea.SetClipboard(transcript)
	}
	return m, nil
}

func (m *Model) processPanelHeight() int {
	body := m.height - 4
	if body < 7 {
		return max(0, body-3)
	}
	return min(body-4, max(7, (body*2+4)/5))
}

func (m *Model) processViewportHeight() int { return max(0, m.processPanelHeight()-3) }

func (m *Model) processLines() []string {
	return m.processLinesAtWidth(max(1, m.width-2))
}

func (m *Model) processLinesAtWidth(width int) []string {
	transcript := m.processTranscript()
	if transcript == "" {
		return []string{"No Git process output recorded."}
	}
	return strings.Split(ansi.Hardwrap(transcript, max(1, width), true), "\n")
}

func (m *Model) processMaximumOffset() int {
	return max(0, len(m.processLines())-m.processViewportHeight())
}

func (m *Model) clampProcessOffset() {
	m.processOffset = min(max(0, m.processOffset), m.processMaximumOffset())
}

func (m *Model) scrollProcessesToEnd() { m.processOffset = m.processMaximumOffset() }
