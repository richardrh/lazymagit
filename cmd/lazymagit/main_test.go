package main

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	gitbackend "github.com/richard/lazymagit/internal/git"
)

type fakeRepository struct {
	workTree string
	gitDir   string
	bare     bool
}

func (r fakeRepository) WorkTree() string { return r.workTree }
func (r fakeRepository) GitDir() string   { return r.gitDir }
func (r fakeRepository) IsBare() bool     { return r.bare }

func testRuntime(input string, interactive bool) (*runtimeDeps, *strings.Builder, *int, *int) {
	stderr := new(strings.Builder)
	initCalls, uiCalls := new(int), new(int)
	r := &runtimeDeps{
		stdin:       strings.NewReader(input),
		stderr:      stderr,
		interactive: interactive,
		init: func(context.Context, string) (repository, error) {
			*initCalls++
			return fakeRepository{workTree: "/repo", gitDir: "/repo/.git"}, nil
		},
		startUI: func(repository) error {
			*uiCalls++
			return nil
		},
	}
	return r, stderr, initCalls, uiCalls
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantInit bool
		wantPath string
		wantErr  bool
	}{
		{name: "default", wantPath: "."},
		{name: "repository", args: []string{"repo"}, wantPath: "repo"},
		{name: "init", args: []string{"--init", "repo"}, wantInit: true, wantPath: "repo"},
		{name: "leading dash", args: []string{"--", "-repo"}, wantPath: "-repo"},
		{name: "init leading dash", args: []string{"--init", "--", "-repo"}, wantInit: true, wantPath: "-repo"},
		{name: "unknown option", args: []string{"--bad"}, wantErr: true},
		{name: "extra argument", args: []string{"one", "two"}, wantErr: true},
		{name: "option after path", args: []string{"repo", "--init"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseArgs() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && (got.init != tt.wantInit || got.path != tt.wantPath) {
				t.Fatalf("parseArgs() = %#v, want init=%v path=%q", got, tt.wantInit, tt.wantPath)
			}
		})
	}
}

func TestInteractiveNonRepositoryAnswers(t *testing.T) {
	for _, tt := range []struct {
		name        string
		input       string
		wantInit    int
		wantUI      int
		wantPrompts int
	}{
		{name: "yes", input: "yes\n", wantInit: 1, wantUI: 1, wantPrompts: 1},
		{name: "short yes after invalid", input: "perhaps\ny\n", wantInit: 1, wantUI: 1, wantPrompts: 2},
		{name: "no", input: "no\n", wantPrompts: 1},
		{name: "short no", input: "n\n", wantPrompts: 1},
		{name: "empty", input: "\n", wantPrompts: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r, stderr, initCalls, uiCalls := testRuntime(tt.input, true)
			r.discover = func(string) (repository, error) { return nil, gitbackend.ErrNotRepository }
			path := filepath.Join(t.TempDir(), "unsafe\nrepo")
			if err := runWith(context.Background(), []string{path}, r); err != nil {
				t.Fatalf("runWith: %v", err)
			}
			if *initCalls != tt.wantInit || *uiCalls != tt.wantUI {
				t.Fatalf("calls init=%d UI=%d, want %d and %d", *initCalls, *uiCalls, tt.wantInit, tt.wantUI)
			}
			if got := strings.Count(stderr.String(), "Create repository in "); got != tt.wantPrompts {
				t.Fatalf("prompt count = %d, want %d; stderr %q", got, tt.wantPrompts, stderr.String())
			}
			if strings.Contains(stderr.String(), "unsafe\nrepo") {
				t.Fatalf("prompt contains an unsanitized path: %q", stderr.String())
			}
		})
	}
}

func TestNonInteractiveNonRepositoryDoesNotReadOrInitialize(t *testing.T) {
	r, _, initCalls, uiCalls := testRuntime("yes\n", false)
	r.stdin = panicReader{}
	r.discover = func(string) (repository, error) { return nil, gitbackend.ErrNotRepository }
	err := runWith(context.Background(), []string{"repo"}, r)
	if err == nil || !strings.Contains(err.Error(), "use --init") {
		t.Fatalf("runWith error = %v, want actionable --init error", err)
	}
	if *initCalls != 0 || *uiCalls != 0 {
		t.Fatalf("calls init=%d UI=%d, want zero", *initCalls, *uiCalls)
	}
}

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) { panic("stdin was read") }

func TestUnclassifiedDiscoveryErrorNeverPromptsOrInitializes(t *testing.T) {
	for _, args := range [][]string{nil, {"--init"}} {
		want := errors.New("permission denied")
		r, stderr, initCalls, uiCalls := testRuntime("yes\n", true)
		r.stdin = panicReader{}
		r.discover = func(string) (repository, error) { return nil, want }
		err := runWith(context.Background(), args, r)
		if !errors.Is(err, want) {
			t.Fatalf("runWith(%v) error = %v, want wrapped discovery error", args, err)
		}
		if stderr.Len() != 0 || *initCalls != 0 || *uiCalls != 0 {
			t.Fatalf("runWith(%v): stderr=%q init=%d UI=%d, want no side effects", args, stderr, *initCalls, *uiCalls)
		}
	}
}

func TestExistingRepositoryStartsUI(t *testing.T) {
	r, _, initCalls, uiCalls := testRuntime("", false)
	r.discover = func(string) (repository, error) {
		return fakeRepository{workTree: "/repo", gitDir: "/repo/.git"}, nil
	}
	if err := runWith(context.Background(), []string{"/repo"}, r); err != nil {
		t.Fatal(err)
	}
	if *initCalls != 0 || *uiCalls != 1 {
		t.Fatalf("calls init=%d UI=%d, want 0 and 1", *initCalls, *uiCalls)
	}
}

func TestExplicitInitBehavior(t *testing.T) {
	t.Run("nonrepository noninteractive", func(t *testing.T) {
		r, _, initCalls, uiCalls := testRuntime("", false)
		r.stdin = panicReader{}
		r.discover = func(string) (repository, error) { return nil, gitbackend.ErrNotRepository }
		if err := runWith(context.Background(), []string{"--init", "repo"}, r); err != nil {
			t.Fatal(err)
		}
		if *initCalls != 1 || *uiCalls != 1 {
			t.Fatalf("calls init=%d UI=%d, want one each", *initCalls, *uiCalls)
		}
	})

	t.Run("directory below parent repository", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "child")
		r, _, initCalls, _ := testRuntime("", false)
		r.discover = func(string) (repository, error) {
			return fakeRepository{workTree: filepath.Dir(target), gitDir: filepath.Join(filepath.Dir(target), ".git")}, nil
		}
		if err := runWith(context.Background(), []string{"--init", target}, r); err != nil {
			t.Fatal(err)
		}
		if *initCalls != 1 {
			t.Fatalf("init calls = %d, want 1", *initCalls)
		}
	})

	t.Run("exact repository", func(t *testing.T) {
		target := t.TempDir()
		r, _, initCalls, uiCalls := testRuntime("", false)
		r.discover = func(string) (repository, error) {
			return fakeRepository{workTree: target, gitDir: filepath.Join(target, ".git")}, nil
		}
		if err := runWith(context.Background(), []string{"--init", target}, r); err != nil {
			t.Fatal(err)
		}
		if *initCalls != 0 || *uiCalls != 1 {
			t.Fatalf("calls init=%d UI=%d, want 0 and 1", *initCalls, *uiCalls)
		}
	})

	t.Run("bare repository", func(t *testing.T) {
		target := t.TempDir()
		r, _, initCalls, uiCalls := testRuntime("", false)
		r.discover = func(string) (repository, error) {
			return fakeRepository{bare: true, gitDir: target}, nil
		}
		err := runWith(context.Background(), []string{"--init", target}, r)
		if err == nil || !strings.Contains(err.Error(), "bare repository") {
			t.Fatalf("runWith error = %v, want bare repository error", err)
		}
		if *initCalls != 0 || *uiCalls != 0 {
			t.Fatalf("calls init=%d UI=%d, want zero", *initCalls, *uiCalls)
		}
	})
}

var _ io.Reader = panicReader{}

func TestTerminalSafeDiagnostic(t *testing.T) {
	input := "fatal: bad\x1b[31m\nbranch\t\"topic\"\u0085 café"
	want := `fatal: bad\x1b[31m\nbranch\t\"topic\"\u0085 café`

	got := terminalSafeDiagnostic(input)
	if got != want {
		t.Fatalf("terminalSafeDiagnostic() = %q, want %q", got, want)
	}
	if strings.IndexFunc(got, unicode.IsControl) >= 0 {
		t.Fatalf("terminalSafeDiagnostic() retained a control character: %q", got)
	}
}

func TestTerminalSafeDiagnosticEscapesAllTerminalControls(t *testing.T) {
	var input strings.Builder
	for r := rune(0); r <= 0x1f; r++ {
		input.WriteRune(r)
	}
	for r := rune(0x7f); r <= 0x9f; r++ {
		input.WriteRune(r)
	}

	got := terminalSafeDiagnostic(input.String())
	if strings.IndexFunc(got, unicode.IsControl) >= 0 {
		t.Fatalf("terminalSafeDiagnostic() retained a control character: %q", got)
	}
}
