package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
	gitbackend "github.com/richard/lazymagit/internal/git"
	"github.com/richard/lazymagit/internal/ui"
)

const usage = "usage: lazymagit [--init] [--theme NAME] [--layout standard|compact] [repository]"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "lazymagit:", terminalSafeDiagnostic(err.Error()))
		os.Exit(1)
	}
}

func terminalSafeDiagnostic(text string) string {
	quoted := strconv.QuoteToGraphic(text)
	return quoted[1 : len(quoted)-1]
}

type repository interface {
	GitDir() string
	WorkTree() string
	IsBare() bool
}

type runtimeDeps struct {
	stdin       io.Reader
	stderr      io.Writer
	interactive bool
	discover    func(string) (repository, error)
	init        func(context.Context, string) (repository, error)
	startUI     func(repository, string, string) error
}

func productionRuntime() *runtimeDeps {
	return &runtimeDeps{
		stdin:       os.Stdin,
		stderr:      os.Stderr,
		interactive: term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stderr.Fd()),
		discover: func(path string) (repository, error) {
			return gitbackend.Discover(path)
		},
		init: func(ctx context.Context, path string) (repository, error) {
			return gitbackend.Init(ctx, path)
		},
		startUI: func(repo repository, theme, layout string) error {
			backendRepo, ok := repo.(*gitbackend.Repository)
			if !ok {
				return errors.New("invalid repository backend")
			}
			if err := ui.ApplyTheme(theme); err != nil {
				return err
			}
			_, err := tea.NewProgram(ui.NewWithOptions(backendRepo, ui.Options{Compact: layout == "compact"})).Run()
			return err
		},
	}
}

// run is the small production entry point; runWith contains the injected,
// testable startup policy.
func run(args []string) error {
	return runWith(context.Background(), args, productionRuntime())
}

type options struct {
	init   bool
	theme  string
	layout string
	path   string
}

func parseArgs(args []string) (options, error) {
	opts := options{path: ".", theme: "default", layout: "standard"}
	pathSet := false
	optionsEnded := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !optionsEnded {
			switch {
			case arg == "--":
				optionsEnded = true
				continue
			case arg == "--init" && !pathSet && !opts.init:
				opts.init = true
				continue
			case arg == "--theme" && !pathSet:
				i++
				if i >= len(args) || strings.TrimSpace(args[i]) == "" {
					return options{}, fmt.Errorf("%s", usage)
				}
				opts.theme = args[i]
				continue
			case strings.HasPrefix(arg, "--theme=") && !pathSet:
				opts.theme = strings.TrimPrefix(arg, "--theme=")
				if strings.TrimSpace(opts.theme) == "" {
					return options{}, fmt.Errorf("%s", usage)
				}
				continue
			case arg == "--layout" && !pathSet:
				i++
				if i >= len(args) {
					return options{}, fmt.Errorf("%s", usage)
				}
				opts.layout = args[i]
				if opts.layout != "standard" && opts.layout != "compact" {
					return options{}, fmt.Errorf("layout must be standard or compact")
				}
				continue
			case strings.HasPrefix(arg, "--layout=") && !pathSet:
				opts.layout = strings.TrimPrefix(arg, "--layout=")
				if opts.layout != "standard" && opts.layout != "compact" {
					return options{}, fmt.Errorf("layout must be standard or compact")
				}
				continue
			case strings.HasPrefix(arg, "-"):
				return options{}, fmt.Errorf("%s", usage)
			}
		}
		if pathSet {
			return options{}, fmt.Errorf("%s", usage)
		}
		opts.path = arg
		pathSet = true
	}
	return opts, nil
}

func runWith(ctx context.Context, args []string, rt *runtimeDeps) error {
	opts, err := parseArgs(args)
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(opts.path)
	if err != nil {
		return fmt.Errorf("resolve repository path %s: %w", quotePath(opts.path), err)
	}

	repo, discoverErr := rt.discover(opts.path)
	if discoverErr == nil {
		if opts.init && !isExactRepository(repo, abs) {
			repo, err = initialize(ctx, rt, abs)
			if err != nil {
				return err
			}
		}
		return startUI(rt, repo, opts.theme, opts.layout)
	}
	if !errors.Is(discoverErr, gitbackend.ErrNotRepository) {
		return fmt.Errorf("cannot open repository %s: %w", quotePath(abs), discoverErr)
	}

	if opts.init {
		repo, err = initialize(ctx, rt, abs)
		if err != nil {
			return err
		}
		return startUI(rt, repo, opts.theme, opts.layout)
	}
	if !rt.interactive {
		return fmt.Errorf("%s is not a Git repository; use --init to create one", quotePath(abs))
	}

	create, err := promptCreate(rt.stdin, rt.stderr, abs)
	if err != nil {
		return err
	}
	if !create {
		return nil
	}
	repo, err = initialize(ctx, rt, abs)
	if err != nil {
		return err
	}
	return startUI(rt, repo, opts.theme, opts.layout)
}

func isExactRepository(repo repository, abs string) bool {
	want := filepath.Clean(abs)
	if repo.IsBare() {
		return filepath.Clean(repo.GitDir()) == want
	}
	return filepath.Clean(repo.WorkTree()) == want
}

func initialize(ctx context.Context, rt *runtimeDeps, abs string) (repository, error) {
	repo, err := rt.init(ctx, abs)
	if err != nil {
		return nil, fmt.Errorf("cannot initialize repository %s: %w", quotePath(abs), err)
	}
	return repo, nil
}

func startUI(rt *runtimeDeps, repo repository, theme, layout string) error {
	if repo.IsBare() {
		return fmt.Errorf("bare repository %s has no work tree", quotePath(repo.GitDir()))
	}
	if err := rt.startUI(repo, theme, layout); err != nil {
		return fmt.Errorf("terminal UI: %w", err)
	}
	return nil
}

func promptCreate(input io.Reader, output io.Writer, abs string) (bool, error) {
	reader := bufio.NewReader(input)
	for {
		if _, err := fmt.Fprintf(output, "Create repository in %s? [y/N] ", quotePath(abs)); err != nil {
			return false, fmt.Errorf("write initialization prompt: %w", err)
		}
		answer, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return false, fmt.Errorf("read initialization response: %w", err)
		}
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "y", "yes":
			return true, nil
		case "", "n", "no":
			return false, nil
		}
	}
}

func quotePath(path string) string {
	return strconv.QuoteToGraphic(path)
}
