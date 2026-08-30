package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/richard/lazymagit/internal/quality"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

type options struct {
	coverprofile, baselinePath, root string
	threshold                        float64
	update                           bool
}

func parseOptions(args []string, stderr io.Writer) (options, error) {
	var opts options
	flags := flag.NewFlagSet("crap", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.coverprofile, "coverprofile", "", "native Go coverprofile to analyze (required)")
	flags.StringVar(&opts.baselinePath, "baseline", "internal/quality/crap-baseline.json", "ratchet baseline")
	flags.Float64Var(&opts.threshold, "threshold", 30, "maximum score before ratcheting")
	flags.BoolVar(&opts.update, "update", false, "replace the baseline with current violations")
	flags.StringVar(&opts.root, "root", ".", "module root")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	if opts.coverprofile == "" {
		return options{}, fmt.Errorf("-coverprofile is required")
	}
	return opts, nil
}

func run(args []string, stdout, stderr io.Writer) int {
	opts, err := parseOptions(args, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "crap:", err)
		return 2
	}
	if err := execute(opts, stdout); err != nil {
		fmt.Fprintln(stderr, "crap:", err)
		return 1
	}
	return 0
}

func execute(opts options, stdout io.Writer) error {
	functions, err := quality.Analyze(opts.root, opts.coverprofile)
	if err != nil {
		return err
	}
	for _, fn := range functions {
		fmt.Fprintf(stdout, "%s complexity=%d coverage=%.2f%% crap=%.2f\n", fn.ID, fn.Complexity, fn.Coverage*100, fn.CRAP)
	}
	if opts.update {
		if err := quality.WriteBaseline(opts.baselinePath, opts.threshold, functions); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "updated %s\n", opts.baselinePath)
		return nil
	}
	baseline, err := quality.ReadBaseline(opts.baselinePath)
	if err != nil {
		return err
	}
	return quality.Gate(functions, baseline, opts.threshold)
}
