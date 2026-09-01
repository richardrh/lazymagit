package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	gitbackend "github.com/richardrh/lazymagit/internal/git"
)

func TestRebaseTodoEditorHelper(t *testing.T) {
	source, admin := os.Getenv("LAZYMAGIT_REBASE_TODO_SOURCE"), os.Getenv("LAZYMAGIT_REBASE_TODO_GIT_DIR")
	if source == "" || admin == "" {
		t.Skip("internal rebase editor helper")
	}
	if _, err := gitbackend.RunRebaseTodoEditor([]string{"--lazymagit-rebase-todo-editor", source, admin, os.Args[len(os.Args)-1]}); err != nil {
		t.Fatal(err)
	}
}

func TestMain(m *testing.M) {
	if handled, err := gitbackend.RunRebaseTodoEditor(os.Args[1:]); handled {
		if err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type uiE2ERepo struct {
	t   *testing.T
	dir string
}

func newUIE2ERepo(t *testing.T) *uiE2ERepo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable is required for UI end-to-end tests")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	dir := filepath.Join(root, "repo")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
	r := &uiE2ERepo{t: t, dir: dir}
	r.git("init", "-b", "main")
	r.git("config", "user.name", "UI E2E Test")
	r.git("config", "user.email", "ui-e2e@example.invalid")
	return r
}

func (r *uiE2ERepo) git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", r.dir}, args...)...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func (r *uiE2ERepo) write(path, content string) {
	r.t.Helper()
	full := filepath.Join(r.dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func newE2EModel(t *testing.T, r *uiE2ERepo) *Model {
	t.Helper()
	repo, err := gitbackend.Discover(r.dir)
	if err != nil {
		t.Fatalf("discover repository: %v", err)
	}
	m := New(repo)
	_, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	runE2ECmd(t, m, m.Init())
	if m.loading || m.isError {
		t.Fatalf("initial load: loading=%v error=%v message=%q", m.loading, m.isError, m.message)
	}
	return m
}

func runE2ECmd(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	for cmd != nil {
		msg := cmd()
		if msg == nil {
			return
		}
		_, cmd = m.Update(msg)
	}
}

func sendE2EKey(t *testing.T, m *Model, msg tea.KeyPressMsg) {
	t.Helper()
	_, cmd := m.Update(msg)
	runE2ECmd(t, m, cmd)
}

func selectE2EPath(t *testing.T, m *Model, path string, kind rowKind) {
	t.Helper()
	// Status sections remember their folds across refreshes. Reveal the fixture
	// rows before driving selection with j/k; this helper is about selecting an
	// exact projected row, not asserting a particular initial fold preference.
	m.tree.RevealGlobalDepth(4)
	ids := m.tree.VisibleSectionIDs()
	current, target := -1, -1
	for i, id := range ids {
		if id == m.tree.Cursor() {
			current = i
		}
		if candidate := m.rows[id]; candidate.path == path && candidate.kind == kind {
			target = i
		}
	}
	if current < 0 || target < 0 {
		t.Fatalf("could not find row kind %d for %q; cursor=%q", kind, path, m.tree.Cursor())
	}
	for current < target {
		sendE2EKey(t, m, keyMsg("j"))
		current++
	}
	for current > target {
		sendE2EKey(t, m, keyMsg("k"))
		current--
	}
}

func TestE2EInitialViewAndShiftSStagesOnlyTrackedChanges(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("tracked.txt", "base\n")
	r.git("add", "--", "tracked.txt")
	r.git("commit", "-m", "initial")
	r.write("tracked.txt", "changed\n")
	r.write("untracked.txt", "new\n")

	m := newE2EModel(t, r)
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"LAZYMAGIT", "main", "Untracked files (1)", "Unstaged changes (1)", "tracked.txt"} {
		if !strings.Contains(view, want) {
			t.Fatalf("initial view omitted %q:\n%s", want, view)
		}
	}

	shiftS := tea.KeyPressMsg(tea.Key{Code: 's', Text: "S", Mod: tea.ModShift})
	sendE2EKey(t, m, shiftS)
	if got := r.git("diff", "--cached", "--name-only"); got != "tracked.txt" {
		t.Fatalf("Shift-S staged %q, want only tracked.txt", got)
	}
	if got := r.git("status", "--porcelain", "--", "untracked.txt"); got != "?? untracked.txt" {
		t.Fatalf("Shift-S changed untracked status: %q", got)
	}
	if m.message != "stage all tracked changes complete" {
		t.Fatalf("Shift-S status = %q", m.message)
	}

	sendE2EKey(t, m, shiftS)
	if m.isError || m.message != "No tracked unstaged changes to stage" {
		t.Fatalf("second Shift-S error=%v message=%q", m.isError, m.message)
	}
}

func TestE2EMarkedFilesStageAndUnstageAsOneBatch(t *testing.T) {
	r := newUIE2ERepo(t)
	for _, path := range []string{"a.txt", "b.txt"} {
		r.write(path, "base\n")
	}
	r.git("add", "--", "a.txt", "b.txt")
	r.git("commit", "-m", "initial")
	r.write("a.txt", "changed a\n")
	r.write("b.txt", "changed b\n")

	m := newE2EModel(t, r)
	for _, path := range []string{"a.txt", "b.txt"} {
		selectE2EPath(t, m, path, rowUnstaged)
		sendE2EKey(t, m, keyMsg("alt+m"))
	}
	sendE2EKey(t, m, keyMsg("s"))
	if got := strings.Fields(r.git("diff", "--cached", "--name-only")); len(got) != 2 || got[0] != "a.txt" || got[1] != "b.txt" {
		t.Fatalf("batch stage paths = %v", got)
	}

	for _, path := range []string{"a.txt", "b.txt"} {
		selectE2EPath(t, m, path, rowStaged)
		sendE2EKey(t, m, keyMsg("alt+m"))
	}
	sendE2EKey(t, m, keyMsg("u"))
	if got := r.git("diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("batch unstage left paths %q", got)
	}

	for _, path := range []string{"a.txt", "b.txt"} {
		selectE2EPath(t, m, path, rowUnstaged)
		sendE2EKey(t, m, keyMsg("alt+m"))
	}
	sendE2EKey(t, m, keyMsg("x"))
	if m.mode != modeConfirm || len(m.confirmPaths) != 2 {
		t.Fatalf("batch discard confirmation mode=%v paths=%v", m.mode, m.confirmPaths)
	}
	sendE2EKey(t, m, keyMsg("y"))
	if got := r.git("diff", "--name-only"); got != "" {
		t.Fatalf("batch discard left paths %q", got)
	}
}

func TestE2EStageUnstageDiscardCommitAndSwitchBranchByKeys(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("tracked.txt", "base\n")
	r.git("add", "--", "tracked.txt")
	r.git("commit", "-m", "initial")
	r.git("branch", "feature")
	r.write("tracked.txt", "working\n")

	m := newE2EModel(t, r)
	selectE2EPath(t, m, "tracked.txt", rowUnstaged)
	sendE2EKey(t, m, keyMsg("s"))
	if got := r.git("diff", "--cached", "--name-only"); got != "tracked.txt" {
		t.Fatalf("s staged %q", got)
	}

	selectE2EPath(t, m, "tracked.txt", rowStaged)
	sendE2EKey(t, m, keyMsg("u"))
	if got := r.git("diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("u left staged paths %q", got)
	}

	selectE2EPath(t, m, "tracked.txt", rowUnstaged)
	sendE2EKey(t, m, keyMsg("x"))
	if m.mode != modeConfirm {
		t.Fatal("x did not open discard confirmation")
	}
	sendE2EKey(t, m, keyMsg("y"))
	content, err := os.ReadFile(filepath.Join(r.dir, "tracked.txt"))
	if err != nil || string(content) != "base\n" {
		t.Fatalf("confirmed discard content=%q err=%v", content, err)
	}

	r.write("tracked.txt", "committed through UI\n")
	// Toggle to Magit keys so g refreshes immediately; no timers or sleeps are
	// involved in this end-to-end driver.
	sendE2EKey(t, m, keyMsg("f2"))
	sendE2EKey(t, m, keyMsg("g"))
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: 's', Text: "S", Mod: tea.ModShift}))
	sendE2EKey(t, m, keyMsg("c"))
	sendE2EKey(t, m, keyMsg("c"))
	for _, char := range "UI commit" {
		sendE2EKey(t, m, keyMsg(string(char)))
	}
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if got := r.git("log", "-1", "--format=%s"); got != "UI commit" {
		t.Fatalf("commit subject = %q", got)
	}

	sendE2EKey(t, m, keyMsg("b"))
	sendE2EKey(t, m, keyMsg("b"))
	if m.mode != modeWorkflow || m.workflow == nil {
		t.Fatalf("b b mode = %d", m.mode)
	}
	for m.workflow.dialog.Fields[0].Value != "" {
		sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	}
	for _, char := range "feature" {
		sendE2EKey(t, m, keyMsg(string(char)))
	}
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if got := r.git("branch", "--show-current"); got != "feature" {
		t.Fatalf("selected branch = %q", got)
	}
}
