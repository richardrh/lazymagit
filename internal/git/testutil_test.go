package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRebaseTodoEditorHelper(t *testing.T) {
	source, admin := os.Getenv("LAZYMAGIT_REBASE_TODO_SOURCE"), os.Getenv("LAZYMAGIT_REBASE_TODO_GIT_DIR")
	if source == "" || admin == "" {
		t.Skip("internal rebase editor helper")
	}
	if _, err := RunRebaseTodoEditor([]string{"--lazymagit-rebase-todo-editor", source, admin, os.Args[len(os.Args)-1]}); err != nil {
		t.Fatal(err)
	}
}

func TestMain(m *testing.M) {
	if handled, err := RunRebaseTodoEditor(os.Args[1:]); handled {
		if err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	home, err := os.MkdirTemp("", "lazymagit-test-home-")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("HOME", home)
	_ = os.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	_ = os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}

// testRepo is deliberately implemented in terms of the git executable rather
// than the backend under test. It provides setup and independent assertions.
type testRepo struct {
	t   *testing.T
	dir string
}

func newTestRepo(t *testing.T) *testRepo {
	t.Helper()
	requireGit(t)
	r := &testRepo{t: t, dir: t.TempDir()}
	r.git("init", "-b", "main")
	r.configureIdentity()
	return r
}

func newBareTestRepo(t *testing.T) *testRepo {
	t.Helper()
	requireGit(t)
	r := &testRepo{t: t, dir: t.TempDir()}
	r.git("init", "--bare")
	return r
}

func cloneTestRepo(t *testing.T, remote string) *testRepo {
	t.Helper()
	requireGit(t)
	parent := t.TempDir()
	dir := filepath.Join(parent, "clone")
	runGit(t, parent, "clone", remote, dir)
	r := &testRepo{t: t, dir: dir}
	r.configureIdentity()
	return r
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable is required for backend integration tests")
	}
}

func (r *testRepo) configureIdentity() {
	r.t.Helper()
	r.git("config", "user.name", "Backend Test")
	r.git("config", "user.email", "backend-test@example.invalid")
}

func (r *testRepo) git(args ...string) string {
	r.t.Helper()
	return runGit(r.t, r.dir, args...)
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_NOSYSTEM=1",
		"LC_ALL=C",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func (r *testRepo) write(path, contents string) {
	r.t.Helper()
	full := filepath.Join(r.dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(contents), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *testRepo) read(path string) string {
	r.t.Helper()
	b, err := os.ReadFile(filepath.Join(r.dir, filepath.FromSlash(path)))
	if err != nil {
		r.t.Fatal(err)
	}
	return string(b)
}

func (r *testRepo) commitAll(message string) string {
	r.t.Helper()
	r.git("add", "--all")
	r.git("commit", "-m", message)
	return r.git("rev-parse", "HEAD")
}
