package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func historyE2EReplaceField(t *testing.T, m *Model, value string) {
	t.Helper()
	if m.mode != modeWorkflow || m.workflow == nil {
		t.Fatalf("expected workflow, mode=%d message=%q", m.mode, m.message)
	}
	for m.workflow.dialog.Fields[m.workflow.field].Value != "" {
		sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	}
	for _, char := range value {
		sendE2EKey(t, m, keyMsg(string(char)))
	}
}

func historyE2ESubmit(t *testing.T, m *Model) {
	t.Helper()
	for m.workflow != nil && m.workflow.field < len(m.workflow.dialog.Fields) {
		sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	}
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
}

func TestHistoryE2EApplyCancelAndStaleResetByKeys(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("base.txt", "base\n")
	base := func() string { r.git("add", "--all"); r.git("commit", "-m", "base"); return r.git("rev-parse", "HEAD") }()
	r.git("checkout", "-b", "source")
	r.write("applied.txt", "from source\n")
	source := func() string {
		r.git("add", "--all")
		r.git("commit", "-m", "source")
		return r.git("rev-parse", "HEAD")
	}()
	r.git("checkout", "main")
	m := newE2EModel(t, r)
	sendE2EKey(t, m, keyMsg("f2"))

	// Lower-case a is Magit's no-commit cherry apply.
	sendE2EKey(t, m, keyMsg("a"))
	historyE2EReplaceField(t, m, source)
	historyE2ESubmit(t, m)
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if got := r.git("rev-parse", "HEAD"); got != base {
		t.Fatalf("apply moved HEAD to %s, want %s", got, base)
	}
	if got := r.git("diff", "--cached", "--name-only"); got != "applied.txt" {
		t.Fatalf("apply index = %q", got)
	}
	if content, err := os.ReadFile(filepath.Join(r.dir, "applied.txt")); err != nil || string(content) != "from source\n" {
		t.Fatalf("apply worktree=%q err=%v", content, err)
	}
	r.git("reset", "--hard", base)
	sendE2EKey(t, m, keyMsg("g"))

	sendE2EKey(t, m, keyMsg("a"))
	historyE2EReplaceField(t, m, source)
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if got := r.git("status", "--porcelain"); got != "" {
		t.Fatalf("cancel changed repository: %q", got)
	}

	// X h reviews a hard reset. Change worktree after review and before the
	// second Enter; execution must reject the stale token and preserve it.
	sendE2EKey(t, m, keyMsg("X"))
	sendE2EKey(t, m, keyMsg("h"))
	historyE2ESubmit(t, m)
	if m.workflow == nil || m.workflow.review == nil {
		t.Fatalf("hard reset did not reach review: mode=%d message=%q", m.mode, m.message)
	}
	r.write("base.txt", "changed after review\n")
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if got := r.git("rev-parse", "HEAD"); got != base {
		t.Fatalf("stale reset moved HEAD to %s", got)
	}
	content, err := os.ReadFile(filepath.Join(r.dir, "base.txt"))
	if err != nil || string(content) != "changed after review\n" {
		t.Fatalf("stale reset changed worktree=%q err=%v", content, err)
	}
	if !strings.Contains(m.message, "stale") {
		t.Fatalf("stale reset message = %q", m.message)
	}
}

func TestHistoryE2EInteractiveTodoEditContinueAndAbortByKeys(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("base.txt", "base\n")
	base := func() string { r.git("add", "--all"); r.git("commit", "-m", "base"); return r.git("rev-parse", "HEAD") }()
	r.git("switch", "-c", "topic")
	r.write("one.txt", "one\n")
	one := func() string { r.git("add", "--all"); r.git("commit", "-m", "one"); return r.git("rev-parse", "HEAD") }()
	r.write("two.txt", "two\n")
	two := func() string { r.git("add", "--all"); r.git("commit", "-m", "two"); return r.git("rev-parse", "HEAD") }()
	r.git("switch", "main")
	r.write("main.txt", "main\n")
	main := func() string { r.git("add", "--all"); r.git("commit", "-m", "main"); return r.git("rev-parse", "HEAD") }()
	r.git("switch", "topic")

	m := newE2EModel(t, r)
	sendE2EKey(t, m, keyMsg("f2"))
	sendE2EKey(t, m, keyMsg("r"))
	sendE2EKey(t, m, keyMsg("i"))
	if m.workflow == nil || m.workflow.dialog.Title != "Review interactive rebase todo" {
		t.Fatalf("interactive todo did not open: workflow=%+v message=%q", m.workflow, m.message)
	}
	// edit pauses after the first replayed commit, while drop proves that the
	// reviewed terminal todo controls Git history rather than an external editor.
	m.workflow.dialog.Fields[0].Value = base
	m.workflow.dialog.Fields[1].Value = main
	m.workflow.dialog.Fields[2].Value = "edit " + one + "\ndrop " + two + "\n"
	historyE2ESubmit(t, m)
	if m.workflow == nil || m.workflow.review == nil || !strings.Contains(strings.Join(m.workflow.review.Plan, "\n"), "drop "+two) {
		t.Fatalf("interactive todo review = %+v", m.workflow)
	}
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if _, err := os.Stat(filepath.Join(r.dir, ".git", "rebase-merge")); err != nil {
		t.Fatalf("interactive rebase did not pause: %v; message=%q", err, m.message)
	}
	if got := r.git("log", "-1", "--format=%s"); got != "one" {
		t.Fatalf("paused rebase HEAD subject = %q", got)
	}

	// r e routes to the bounded active-todo editor. Mutating the administrative
	// file after review must reject the stale token instead of replacing it.
	sendE2EKey(t, m, keyMsg("r"))
	sendE2EKey(t, m, keyMsg("e"))
	if m.workflow == nil || m.workflow.dialog.Title != "Review active rebase todo" {
		t.Fatalf("active todo editor did not open: workflow=%+v message=%q", m.workflow, m.message)
	}
	historyE2ESubmit(t, m)
	if m.workflow == nil || m.workflow.review == nil {
		t.Fatal("active todo did not reach review")
	}
	adminTodo := filepath.Join(r.dir, ".git", "rebase-merge", "git-rebase-todo")
	if err := os.WriteFile(adminTodo, []byte("pick "+two+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if !strings.Contains(m.message, "stale") {
		t.Fatalf("stale todo edit message = %q", m.message)
	}

	// Restore the reviewed instruction, then continue through the actual Magit
	// shortcut. The dropped commit is absent and the rewritten parent is main.
	if err := os.WriteFile(adminTodo, []byte("drop "+two+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sendE2EKey(t, m, keyMsg("r"))
	sendE2EKey(t, m, keyMsg("r"))
	historyE2ESubmit(t, m)
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if got := r.git("rev-parse", "HEAD^"); got != main {
		t.Fatalf("continued rebase parent = %s, want %s", got, main)
	}
	if got := r.git("log", "--format=%s", main+"..HEAD"); got != "one" {
		t.Fatalf("rebased commits = %q, want one", got)
	}
	if _, err := os.Stat(filepath.Join(r.dir, ".git", "rebase-merge")); !os.IsNotExist(err) {
		t.Fatalf("rebase metadata remains after continue: %v", err)
	}

	// A new edit rebase can be abandoned through r a and restores the original
	// branch tip, exercising the terminal-native abort route as well.
	original := r.git("rev-parse", "HEAD")
	sendE2EKey(t, m, keyMsg("r"))
	sendE2EKey(t, m, keyMsg("i"))
	m.workflow.dialog.Fields[2].Value = "edit " + original + "\n"
	historyE2ESubmit(t, m)
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if _, err := os.Stat(filepath.Join(r.dir, ".git", "rebase-merge")); err != nil {
		t.Fatalf("abort fixture did not pause: %v", err)
	}
	sendE2EKey(t, m, keyMsg("r"))
	sendE2EKey(t, m, keyMsg("a"))
	historyE2ESubmit(t, m)
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if got := r.git("rev-parse", "HEAD"); got != original {
		t.Fatalf("abort HEAD=%s want %s", got, original)
	}
	if _, err := os.Stat(filepath.Join(r.dir, ".git", "rebase-merge")); !os.IsNotExist(err) {
		t.Fatalf("rebase metadata remains after abort: %v", err)
	}
}

func TestHistoryE2ERebaseOntoPushRemoteByKeys(t *testing.T) {
	local, _, _ := newPushWorkflowE2E(t)
	local.git("push", "-u", "origin", "main")
	local.git("config", "remote.pushDefault", "origin")
	local.write("local.txt", "local\n")
	local.git("add", "local.txt")
	local.git("commit", "-m", "local change")
	head := local.git("rev-parse", "HEAD")
	m := newE2EModel(t, local)
	sendE2EKey(t, m, keyMsg("f2"))

	sendE2EKey(t, m, keyMsg("r"))
	sendE2EKey(t, m, keyMsg("p"))
	if m.workflow == nil {
		t.Fatalf("push-remote rebase did not open: %q", m.message)
	}
	historyE2ESubmit(t, m)
	originMain := local.git("rev-parse", "origin/main")
	if m.workflow == nil || m.workflow.review == nil || !strings.Contains(strings.Join(m.workflow.review.Plan, "\n"), originMain) {
		t.Fatalf("push-remote rebase review = %#v", m.workflow)
	}
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.isError || local.git("rev-parse", "HEAD") != head || local.git("status", "--porcelain") != "" {
		t.Fatalf("push-remote rebase result: error=%v message=%q", m.isError, m.message)
	}
}

func TestHistoryE2EBisectFirstParentOptionByKeys(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("number", "0\n")
	good := func() string { r.git("add", "--all"); r.git("commit", "-m", "good"); return r.git("rev-parse", "HEAD") }()
	r.write("number", "1\n")
	r.git("add", "--all")
	r.git("commit", "-m", "middle")
	r.write("number", "2\n")
	r.git("add", "--all")
	r.git("commit", "-m", "bad")
	m := newE2EModel(t, r)
	sendE2EKey(t, m, keyMsg("f2"))

	sendE2EKey(t, m, keyMsg("B"))
	sendE2EKey(t, m, keyMsg("-p"))
	sendE2EKey(t, m, keyMsg("B"))
	if m.workflow == nil {
		t.Fatalf("bisect start did not open: %q", m.message)
	}
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	historyE2EReplaceField(t, m, good)
	historyE2ESubmit(t, m)
	if m.workflow == nil || m.workflow.review == nil || !strings.Contains(strings.Join(m.workflow.review.Plan, "\n"), "Follow only first parents") {
		t.Fatalf("bisect review = %#v", m.workflow)
	}
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if _, err := os.Stat(filepath.Join(r.dir, ".git", "BISECT_START")); err != nil {
		t.Fatalf("bisect did not start: %v; message=%q", err, m.message)
	}

	sendE2EKey(t, m, keyMsg("B"))
	sendE2EKey(t, m, keyMsg("r"))
	historyE2ESubmit(t, m)
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if _, err := os.Stat(filepath.Join(r.dir, ".git", "BISECT_START")); !os.IsNotExist(err) {
		t.Fatalf("bisect reset did not clear state: %v", err)
	}
}

func TestHistoryE2ERevertConflictContinueAndAbortByKeys(t *testing.T) {
	for _, finish := range []string{"continue", "abort"} {
		t.Run(finish, func(t *testing.T) {
			r := newUIE2ERepo(t)
			r.write("conflict.txt", "base\n")
			r.git("add", "--all")
			r.git("commit", "-m", "base")
			r.git("checkout", "-b", "source")
			r.write("conflict.txt", "source\n")
			source := func() string {
				r.git("add", "--all")
				r.git("commit", "-m", "source")
				return r.git("rev-parse", "HEAD")
			}()
			r.git("checkout", "main")
			r.write("conflict.txt", "main\n")
			main := func() string { r.git("add", "--all"); r.git("commit", "-m", "main"); return r.git("rev-parse", "HEAD") }()
			m := newE2EModel(t, r)
			sendE2EKey(t, m, keyMsg("f2"))
			if finish == "continue" {
				// A A is Magit's committing cherry-copy path. Unlike lower-case
				// cherry-apply (--no-commit), a conflict has sequencer state and a
				// CHERRY_PICK_HEAD that can be continued.
				sendE2EKey(t, m, keyMsg("A"))
				sendE2EKey(t, m, keyMsg("A"))
			} else {
				sendE2EKey(t, m, keyMsg("V"))
				sendE2EKey(t, m, keyMsg("V"))
			}
			historyE2EReplaceField(t, m, source)
			historyE2ESubmit(t, m)
			sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
			headName := "REVERT_HEAD"
			if finish == "continue" {
				headName = "CHERRY_PICK_HEAD"
			}
			headBytes, headErr := os.ReadFile(filepath.Join(r.dir, ".git", headName))
			if got := strings.TrimSpace(string(headBytes)); headErr != nil || got != source {
				t.Fatalf("%s=%s err=%v want %s; message=%q status=%q", headName, got, headErr, source, m.message, r.git("status", "--porcelain"))
			}
			if got := r.git("rev-parse", "HEAD"); got != main {
				t.Fatalf("conflict moved HEAD=%s want %s", got, main)
			}

			if finish == "continue" {
				r.write("conflict.txt", "source\n")
				r.git("add", "--", "conflict.txt")
				sendE2EKey(t, m, keyMsg("A"))
				sendE2EKey(t, m, keyMsg("A"))
				historyE2ESubmit(t, m)
				sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
				if got := r.git("rev-parse", "HEAD^"); got != main {
					t.Fatalf("continue parent=%s want %s", got, main)
				}
				if got := r.git("status", "--porcelain"); got != "" {
					t.Fatalf("continue status=%q", got)
				}
			} else {
				sendE2EKey(t, m, keyMsg("V"))
				sendE2EKey(t, m, keyMsg("a"))
				historyE2ESubmit(t, m) // review
				sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
				if got := r.git("rev-parse", "HEAD"); got != main {
					t.Fatalf("abort HEAD=%s want %s", got, main)
				}
				if got := r.git("status", "--porcelain"); got != "" {
					t.Fatalf("abort status=%q", got)
				}
				if _, err := os.Stat(filepath.Join(r.dir, ".git", "REVERT_HEAD")); !os.IsNotExist(err) {
					t.Fatalf("REVERT_HEAD remains: %v", err)
				}
			}
		})
	}
}
