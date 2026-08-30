package main

import (
	"bytes"
	"strings"
	"testing"
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
