package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/richard/lazymagit/internal/quality"
)

func main() {
	coverprofile := flag.String("coverprofile", "", "native Go coverprofile to analyze (required)")
	baselinePath := flag.String("baseline", "internal/quality/crap-baseline.json", "ratchet baseline")
	threshold := flag.Float64("threshold", 30, "maximum score before ratcheting")
	update := flag.Bool("update", false, "replace the baseline with current violations")
	root := flag.String("root", ".", "module root")
	flag.Parse()
	if *coverprofile == "" {
		fmt.Fprintln(os.Stderr, "crap: -coverprofile is required")
		flag.Usage()
		os.Exit(2)
	}
	functions, err := quality.Analyze(*root, *coverprofile)
	if err != nil {
		fatal(err)
	}
	for _, fn := range functions {
		fmt.Printf("%s complexity=%d coverage=%.2f%% crap=%.2f\n", fn.ID, fn.Complexity, fn.Coverage*100, fn.CRAP)
	}
	if *update {
		if err := quality.WriteBaseline(*baselinePath, *threshold, functions); err != nil {
			fatal(err)
		}
		fmt.Printf("updated %s\n", *baselinePath)
		return
	}
	baseline, err := quality.ReadBaseline(*baselinePath)
	if err != nil {
		fatal(err)
	}
	if err := quality.Gate(functions, baseline, *threshold); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "crap:", err)
	os.Exit(1)
}
