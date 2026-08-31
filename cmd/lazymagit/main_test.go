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
		startUI: func(repository, string, string) error {
			*uiCalls++
			return nil
		},
	}
	return r, stderr, initCalls, uiCalls
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantInit   bool
		wantTheme  string
		wantLayout string
		wantPath   string
		wantErr    bool
	}{
		{name: "default", wantTheme: "default", wantLayout: "standard", wantPath: "."},
		{name: "repository", args: []string{"repo"}, wantTheme: "default", wantLayout: "standard", wantPath: "repo"},
		{name: "theme", args: []string{"--theme", "tokyo-night", "repo"}, wantTheme: "tokyo-night", wantLayout: "standard", wantPath: "repo"},
		{name: "theme equals", args: []string{"--theme=catppuccin", "repo"}, wantTheme: "catppuccin", wantLayout: "standard", wantPath: "repo"},
		{name: "compact layout", args: []string{"--layout", "compact", "repo"}, wantTheme: "default", wantLayout: "compact", wantPath: "repo"},
		{name: "layout equals", args: []string{"--layout=standard", "repo"}, wantTheme: "default", wantLayout: "standard", wantPath: "repo"},
		{name: "init", args: []string{"--init", "repo"}, wantInit: true, wantTheme: "default", wantLayout: "standard", wantPath: "repo"},
		{name: "leading dash", args: []string{"--", "-repo"}, wantTheme: "default", wantLayout: "standard", wantPath: "-repo"},
		{name: "init leading dash", args: []string{"--init", "--", "-repo"}, wantInit: true, wantTheme: "default", wantLayout: "standard", wantPath: "-repo"},
		{name: "missing theme", args: []string{"--theme"}, wantErr: true},
		{name: "empty theme", args: []string{"--theme="}, wantErr: true},
		{name: "missing layout", args: []string{"--layout"}, wantErr: true},
		{name: "invalid layout", args: []string{"--layout=wide"}, wantErr: true},
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
			if err == nil && (got.init != tt.wantInit || got.theme != tt.wantTheme || got.layout != tt.wantLayout || got.path != tt.wantPath) {
				t.Fatalf("parseArgs() = %#v, want init=%v theme=%q layout=%q path=%q", got, tt.wantInit, tt.wantTheme, tt.wantLayout, tt.wantPath)
			}
		})
	}
}

func TestArgumentParserHelpers(t *testing.T) {
	p := argumentParser{opts: options{path: ".", theme: "default", layout: "standard"}}
	if consumed, err := p.consumeTheme([]string{"--theme", "night"}); err != nil || consumed != 2 || p.opts.theme != "night" {
		t.Fatalf("consumeTheme = %d, %v, theme %q", consumed, err, p.opts.theme)
	}
	if _, err := p.consumeTheme([]string{"--theme"}); err == nil {
		t.Fatal("consumeTheme accepted missing value")
	}
	if err := p.setTheme(" "); err == nil {
		t.Fatal("setTheme accepted blank value")
	}
	if consumed, err := p.consumeLayout([]string{"--layout", "compact"}); err != nil || consumed != 2 || p.opts.layout != "compact" {
		t.Fatalf("consumeLayout = %d, %v, layout %q", consumed, err, p.opts.layout)
	}
	if _, err := p.consumeLayout([]string{"--layout"}); err == nil {
		t.Fatal("consumeLayout accepted missing value")
	}
	if err := p.setLayout("wide"); err == nil {
		t.Fatal("setLayout accepted invalid value")
	}
	if err := p.setPath("repo"); err != nil || p.opts.path != "repo" {
		t.Fatalf("setPath: %v, %#v", err, p.opts)
	}
	if err := p.setPath("other"); err == nil {
		t.Fatal("setPath accepted a second path")
	}
}

func TestArgumentParserConsumeAfterOptionsEnd(t *testing.T) {
	p := argumentParser{opts: options{path: ".", theme: "default", layout: "standard"}}
	if consumed, err := p.consume([]string{"--"}); err != nil || consumed != 1 || !p.optionsEnded {
		t.Fatalf("consume terminator = %d, %v, ended=%v", consumed, err, p.optionsEnded)
	}
	if consumed, err := p.consume([]string{"-repo"}); err != nil || consumed != 1 || p.opts.path != "-repo" {
		t.Fatalf("consume path = %d, %v, path=%q", consumed, err, p.opts.path)
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

func TestMainExit(t *testing.T) {
	wantErr := errors.New("bad\nerror")
	tests := []struct {
		name        string
		args        []string
		handled     bool
		rebaseErr   error
		runErr      error
		wantCode    int
		wantRunCall bool
		wantOutput  string
	}{
		{name: "help long", args: []string{"--help"}, wantOutput: usage + "\n"},
		{name: "help short", args: []string{"-h"}, wantOutput: usage + "\n"},
		{name: "editor success", handled: true},
		{name: "editor error", handled: true, rebaseErr: wantErr, wantCode: 1},
		{name: "application success", wantRunCall: true},
		{name: "application error", runErr: wantErr, wantCode: 1, wantRunCall: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr strings.Builder
			runCalled := false
			args := tt.args
			if args == nil {
				args = []string{"arg"}
			}
			code := mainExit(args, &stdout, &stderr,
				func(args []string) (bool, error) { return tt.handled, tt.rebaseErr },
				func(args []string) error { runCalled = true; return tt.runErr })
			if code != tt.wantCode || runCalled != tt.wantRunCall {
				t.Fatalf("mainExit = %d, runCalled=%v", code, runCalled)
			}
			if tt.wantCode == 1 && (!strings.Contains(stderr.String(), "lazymagit:") || strings.Contains(stderr.String(), "bad\nerror")) {
				t.Fatalf("unsafe diagnostic: %q", stderr.String())
			}
			if stdout.String() != tt.wantOutput {
				t.Fatalf("stdout = %q, want %q", stdout.String(), tt.wantOutput)
			}
		})
	}
}
