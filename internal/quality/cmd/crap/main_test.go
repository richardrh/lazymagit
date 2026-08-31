package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/richard/lazymagit/internal/quality"
)

func TestParseOptions(t *testing.T) {
	var stderr bytes.Buffer
	opts, err := parseOptions([]string{"-coverprofile", "cover.out", "-threshold", "50", "-update", "-root", "repo"}, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if opts.coverprofile != "cover.out" || opts.threshold != 50 || !opts.update || opts.root != "repo" {
		t.Fatalf("options = %#v", opts)
	}
	if _, err := parseOptions(nil, &stderr); err == nil {
		t.Fatal("missing cover profile was accepted")
	}
}

func TestExecuteHelpers(t *testing.T) {
	functions := []quality.Function{{ID: "p/-/f", Complexity: 2, Coverage: .5, CRAP: 2.5}}
	var output bytes.Buffer
	printFunctions(&output, functions)
	if got := output.String(); !strings.Contains(got, "p/-/f complexity=2 coverage=50.00% crap=2.50") {
		t.Fatalf("output = %q", got)
	}

	baseline := filepath.Join(t.TempDir(), "baseline.json")
	opts := options{baselinePath: baseline, threshold: 20}
	output.Reset()
	if err := updateBaseline(opts, &output, functions); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "updated ") {
		t.Fatalf("update output = %q", output.String())
	}
	if err := gateBaseline(opts, functions); err != nil {
		t.Fatal(err)
	}
	opts.baselinePath = filepath.Join(t.TempDir(), "missing", "baseline.json")
	if err := updateBaseline(opts, &output, functions); err == nil {
		t.Fatal("updateBaseline accepted missing parent")
	}
	if err := gateBaseline(opts, functions); err == nil {
		t.Fatal("gateBaseline accepted missing baseline")
	}
}

func TestRunReportsUsageAndAnalysisErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "coverprofile") {
		t.Fatalf("missing profile: code=%d stderr=%q", code, stderr.String())
	}
	stderr.Reset()
	if code := run([]string{"-coverprofile", "missing", "-root", t.TempDir()}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "crap:") {
		t.Fatalf("analysis failure: code=%d stderr=%q", code, stderr.String())
	}
}
