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
	os.Exit(mainExit(os.Args[1:], os.Stderr, gitbackend.RunRebaseTodoEditor, run))
}

func mainExit(args []string, stderr io.Writer, rebaseEditor func([]string) (bool, error), appRun func([]string) error) int {
	if handled, err := rebaseEditor(args); handled {
		if err != nil {
			fmt.Fprintln(stderr, "lazymagit:", terminalSafeDiagnostic(err.Error()))
			return 1
		}
		return 0
	}
	if err := appRun(args); err != nil {
		fmt.Fprintln(stderr, "lazymagit:", terminalSafeDiagnostic(err.Error()))
		return 1
	}
	return 0
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
	parser := argumentParser{opts: options{path: ".", theme: "default", layout: "standard"}}
	for len(args) > 0 {
		consumed, err := parser.consume(args)
		if err != nil {
			return options{}, err
		}
		args = args[consumed:]
	}
	return parser.opts, nil
}

type argumentParser struct {
	opts         options
	pathSet      bool
	optionsEnded bool
}

func (p *argumentParser) consume(args []string) (int, error) {
	arg := args[0]
	if p.optionsEnded {
		return 1, p.setPath(arg)
	}
	switch {
	case arg == "--":
		p.optionsEnded = true
		return 1, nil
	case arg == "--init" && !p.pathSet && !p.opts.init:
		p.opts.init = true
		return 1, nil
	case arg == "--theme" && !p.pathSet:
		return p.consumeTheme(args)
	case strings.HasPrefix(arg, "--theme=") && !p.pathSet:
		return 1, p.setTheme(strings.TrimPrefix(arg, "--theme="))
	case arg == "--layout" && !p.pathSet:
		return p.consumeLayout(args)
	case strings.HasPrefix(arg, "--layout=") && !p.pathSet:
		return 1, p.setLayout(strings.TrimPrefix(arg, "--layout="))
	case strings.HasPrefix(arg, "-"):
		return 0, fmt.Errorf("%s", usage)
	default:
		return 1, p.setPath(arg)
	}
}

func (p *argumentParser) consumeTheme(args []string) (int, error) {
	if len(args) < 2 {
		return 0, fmt.Errorf("%s", usage)
	}
	return 2, p.setTheme(args[1])
}

func (p *argumentParser) setTheme(theme string) error {
	if strings.TrimSpace(theme) == "" {
		return fmt.Errorf("%s", usage)
	}
	p.opts.theme = theme
	return nil
}

func (p *argumentParser) consumeLayout(args []string) (int, error) {
	if len(args) < 2 {
		return 0, fmt.Errorf("%s", usage)
	}
	return 2, p.setLayout(args[1])
}

func (p *argumentParser) setLayout(layout string) error {
	if layout != "standard" && layout != "compact" {
		return fmt.Errorf("layout must be standard or compact")
	}
	p.opts.layout = layout
	return nil
}

func (p *argumentParser) setPath(path string) error {
	if p.pathSet {
		return fmt.Errorf("%s", usage)
	}
	p.opts.path = path
	p.pathSet = true
	return nil
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
