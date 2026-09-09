package ui

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// MessageWorkflow opts a multiline field into the shared modal composer.
// DraftPath is private repository metadata, never a tracked worktree file.
// Result is read on the UI thread only after the submission command completes.
type MessageWorkflow struct {
	DraftPath string
	Context   string
	Preview   string
	Result    func() string
}

type messageWorkflowState struct {
	buffer        *MessageBuffer
	index         int
	prefix        bool
	showPreview   bool
	previewOffset int
	saved         bool
}

type messageDraftTick struct {
	workflow *workflowState
	request  uint64
}

const messageDraftLimit = 1 << 20

func messageDraftPath(gitDir, key string) string {
	if gitDir == "" {
		return ""
	}
	return filepath.Join(gitDir, "lazymagit", "drafts", fmt.Sprintf("%x.json", sha256.Sum256([]byte(key))))
}

func prepareMessageWorkflow(w *workflowState) error {
	if w.dialog.Message == nil {
		return nil
	}
	var values WorkflowValues
	path := w.dialog.Message.DraftPath
	if path != "" {
		file, err := os.Open(path)
		if err == nil {
			data, readErr := io.ReadAll(io.LimitReader(file, messageDraftLimit+1))
			file.Close()
			if readErr != nil {
				return fmt.Errorf("read message draft: %w", readErr)
			}
			if len(data) > messageDraftLimit {
				return errors.New("saved message draft exceeds 1 MiB")
			}
			if err := json.Unmarshal(data, &values); err != nil {
				return fmt.Errorf("read message draft %s: %w", path, err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("read message draft: %w", err)
		}
	}
	for i := range w.dialog.Fields {
		field := &w.dialog.Fields[i]
		if value, ok := values[field.Name]; ok {
			if field.Kind == WorkflowBool {
				field.Bool = value == "true"
			} else {
				field.Value = value
			}
		}
		if field.Kind == WorkflowMultiline {
			w.message = &messageWorkflowState{buffer: NewMessageBuffer(field.Value), index: i, saved: values != nil}
			field.Value = w.message.buffer.Text()
			w.field = i
		}
	}
	if w.message == nil {
		return errors.New("message workflow requires a multiline field")
	}
	return nil
}

func (m *Model) messageDraftChanged() tea.Cmd {
	w := m.workflow
	w.request++
	w.review = nil
	w.message.saved = false
	request := w.request
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
		return messageDraftTick{workflow: w, request: request}
	})
}

func (m *Model) saveMessageDraft() error {
	w := m.workflow
	if w == nil || w.message == nil {
		return nil
	}
	path := w.dialog.Message.DraftPath
	if path == "" {
		return nil
	}
	data, err := json.Marshal(m.workflowValues())
	if err != nil {
		return err
	}
	if len(data) > messageDraftLimit {
		return errors.New("message draft exceeds 1 MiB; shorten it before saving")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".draft-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	w.message.saved = true
	return nil
}

func (m *Model) handleMessageDraftTick(msg messageDraftTick) tea.Cmd {
	if m.workflow != msg.workflow || m.workflow.request != msg.request || m.workflow.message == nil || !m.appActive() {
		return nil
	}
	if err := m.saveMessageDraft(); err != nil {
		m.workflow.error = "Save draft: " + sanitizeSingleLine(err.Error())
	}
	return nil
}

func (m *Model) handleMessagePaste(text string) tea.Cmd {
	w := m.workflow
	if m.mode != modeWorkflow || w == nil || w.message == nil || w.busy || w.review != nil {
		return nil
	}
	if w.field != w.message.index {
		if w.field < len(w.dialog.Fields) && w.dialog.Fields[w.field].Kind == WorkflowText {
			w.dialog.Fields[w.field].Value += strings.ReplaceAll(strings.ReplaceAll(text, "\r", ""), "\n", " ")
			return m.messageDraftChanged()
		}
		return nil
	}
	w.message.buffer.Paste(text)
	w.dialog.Fields[w.message.index].Value = w.message.buffer.Text()
	w.error = ""
	return m.messageDraftChanged()
}

func (m *Model) handleMessageWorkflowKey(msg tea.KeyPressMsg) tea.Cmd {
	w, key := m.workflow, msg.String()
	s := w.message
	if key == "ctrl+g" {
		return m.cancelWorkflow()
	}
	if w.busy {
		return nil
	}
	if s.prefix {
		s.prefix = false
		switch key {
		case "ctrl+c":
			w.field = len(w.dialog.Fields)
			return m.activateWorkflow()
		case "ctrl+k":
			return m.cancelWorkflow()
		default:
			w.error = "C-c C-c submit; C-c C-k save draft and close"
			return nil
		}
	}
	if key == "ctrl+c" {
		s.prefix = true
		return nil
	}
	if key == "ctrl+s" {
		if err := m.saveMessageDraft(); err != nil {
			w.error = "Save draft: " + sanitizeSingleLine(err.Error())
		} else {
			w.error = ""
			m.setMessage("Message draft saved")
		}
		return nil
	}
	if key == "alt+d" {
		s.showPreview = !s.showPreview
		return nil
	}
	if key == "alt+j" || key == "alt+k" {
		delta := 1
		if key == "alt+k" {
			delta = -1
		}
		preview := w.dialog.Message.Preview
		if w.review != nil {
			preview = strings.Join(w.review.Plan, "\n")
		}
		s.previewOffset = max(0, min(s.previewOffset+delta, max(0, strings.Count(preview, "\n"))))
		return nil
	}
	if w.review != nil {
		if key == "esc" {
			w.review = nil
			w.field = s.index
			w.error = ""
		} else if key == "enter" {
			return m.activateWorkflow()
		}
		return nil
	}
	if key == "tab" || key == "shift+tab" {
		delta := 1
		if key == "shift+tab" {
			delta = -1
		}
		w.field = (w.field + delta + len(w.dialog.Fields) + 1) % (len(w.dialog.Fields) + 1)
		return nil
	}
	if w.field == s.index {
		if s.buffer.HandleKey(msg) {
			w.dialog.Fields[s.index].Value = s.buffer.Text()
			w.error = ""
			return m.messageDraftChanged()
		}
		return nil
	}
	if key == "esc" {
		w.field = s.index
		s.buffer.HandleKey(msg)
		return nil
	}
	if w.field >= len(w.dialog.Fields) {
		if key == "enter" {
			return m.activateWorkflow()
		}
		return nil
	}
	if editWorkflowField(&w.dialog.Fields[w.field], msg) {
		w.error = ""
		return m.messageDraftChanged()
	}
	return nil
}

// Keep the original editor state until success, including undo and cursor.
func (m *Model) retainMessageSubmission() {
	if m.workflow != nil && m.workflow.message != nil {
		m.pendingMessage = m.workflow
	}
}

func (m *Model) finishMessageSubmission(msg operationMsg) bool {
	w := m.pendingMessage
	if w == nil {
		return false
	}
	m.pendingMessage = nil
	if msg.opErr != nil {
		w.busy, w.review = false, nil
		w.field = w.message.index
		w.error = sanitizeSingleLine(msg.opErr.Error())
		m.workflow = w
		m.setMode(modeWorkflow)
		m.setMessage("Submission failed; draft preserved. C-c C-c retries; C-g saves and closes.")
		return true
	}
	if path := w.dialog.Message.DraftPath; path != "" {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			m.setError(fmt.Errorf("submission succeeded but could not remove draft: %w", err))
			return false
		}
	}
	if w.dialog.Message.Result != nil && msg.loadErr == nil {
		if result := w.dialog.Message.Result(); result != "" {
			m.setMessage(result)
		}
	}
	return false
}

func (m *Model) renderMessageWorkflow(width, height int) string {
	w, s := m.workflow, m.workflow.message
	if width < 32 || height < 12 {
		return fitBlock("Enlarge terminal to edit message", width, height)
	}
	innerW, innerH := width-4, height-2
	purple := lipgloss.NewStyle().Foreground(colorPurple).Bold(true)
	muted := lipgloss.NewStyle().Foreground(colorMuted)
	lines := []string{purple.Render(w.dialog.Title) + "  " + muted.Render(sanitizeSingleLine(w.dialog.Message.Context))}
	for i, f := range w.dialog.Fields {
		if i != s.index {
			lines = append(lines, renderWorkflowField(f, i == w.field)...)
		}
	}
	line, column := s.buffer.Cursor()
	subject, _, _ := strings.Cut(w.dialog.Fields[s.index].Value, "\n")
	saved := "unsaved"
	if s.saved {
		saved = "draft saved"
	}
	mode := s.buffer.Mode()
	if w.field != s.index {
		mode = "FORM"
	}
	if w.review != nil {
		mode = "REVIEW"
	}
	if s.prefix {
		mode = "C-c …"
	}
	status := fmt.Sprintf("%s  %d:%d  Title %d chars  %s", mode, line+1, column+1, utf8.RuneCountInString(subject), saved)
	lines = append(lines, purple.Render(status))
	if utf8.RuneCountInString(subject) > 72 {
		lines = append(lines, lipgloss.NewStyle().Foreground(colorGold).Render("Long subject: consider keeping it within 72 characters"))
	}
	footer := renderWorkflowAction(workflowActionLabel(w), w.field >= len(w.dialog.Fields), w.busy)
	if w.review != nil {
		footer += "  Esc edit; Enter " + strings.ToLower(workflowActionLabel(w))
	}
	hint := "Esc normal  i insert  C-c C-c submit  C-g save/close  C-s save"
	if innerW >= 95 {
		hint += "  Tab field  Alt-d diff"
	}
	if w.review != nil {
		hint = "Esc edit  Enter submit  C-g save/close  Alt-j/k scroll review"
	}
	if w.error != "" {
		footer = lipgloss.NewStyle().Foreground(colorRed).Render(sanitizeSingleLine(w.error))
	}
	bodyH := max(1, innerH-len(lines)-2)
	bodyW := innerW
	if innerW >= 105 && w.dialog.Message.Preview != "" && !s.showPreview {
		bodyW = innerW * 3 / 5
	}
	body := s.buffer.View(bodyW, bodyH)
	preview := w.dialog.Message.Preview
	if w.review != nil {
		preview = strings.Join(w.review.Plan, "\n")
	}
	if preview == "" {
		preview = "No diff available for this message."
	}
	previewLines := strings.Split(sanitizeDiff(preview), "\n")
	if w.review == nil {
		for i, line := range previewLines {
			previewLines[i] = compactDiffLineStyle(lipgloss.NewStyle(), line).Render(line)
		}
	}
	offset := min(s.previewOffset, max(0, len(previewLines)-bodyH+1))
	heading := "Changes"
	if w.review != nil {
		heading = "Review"
	}
	preview = heading + " (Alt-j/k scroll)\n" + strings.Join(previewLines[offset:], "\n")
	if bodyW != innerW {
		separator := muted.Render("│")
		right := separator + strings.ReplaceAll(fitBlock(preview, innerW-bodyW-1, bodyH), "\n", "\n"+separator)
		body = lipgloss.JoinHorizontal(lipgloss.Top, body, right)
	} else if s.showPreview || w.review != nil {
		body = fitBlock(preview, innerW, bodyH)
	}
	lines = append(lines, body, footer, muted.Render(hint))
	content := fitBlock(strings.Join(lines, "\n"), innerW, innerH)
	return lipgloss.NewStyle().Width(width).Height(height).Padding(0, 1).Border(lipgloss.DoubleBorder()).BorderForeground(colorPurple).Render(content)
}

func (m *Model) messageFooter() string {
	return ansi.Truncate("Vim buffer  C-c C-c submit  C-c C-k save/close  C-s save  Alt-d diff", m.width, "")
}
