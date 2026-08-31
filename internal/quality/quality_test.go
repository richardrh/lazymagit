package quality

import (
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComplexityCountsSelectCaseAndSkipsLiteralBody(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "sample.go", `package p
func f(ch <-chan int) {
	select { case <-ch: default: }
	_ = func() { if true {} }
}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	fn := file.Decls[0].(*ast.FuncDecl)
	if got := complexity(fn.Body); got != 2 {
		t.Fatalf("complexity = %d, want 2", got)
	}
}

func TestAnalyzeComplexityAndWeightedCoverage(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "go.mod"), "module example.com/project\n\ngo 1.25\n")
	writeTestFile(t, filepath.Join(root, "work.go"), `package project

func Work(xs []int, a, b bool) int {
	if a && b || !a {
		for range xs {
		}
	}
	switch len(xs) {
	case 1, 2:
	default:
	}
	return len(xs)
}

type Worker struct{}
func (w *Worker) Run() { if true {} }
`)
	profile := filepath.Join(root, "cover.out")
	writeTestFile(t, profile, `mode: set
example.com/project/work.go:4.34,5.18 3 1
example.com/project/work.go:5.18,9.2 1 0
example.com/project/work.go:9.2,13.2 2 1
example.com/project/work.go:16.24,16.38 1 0
`)

	functions, err := Analyze(root, profile)
	if err != nil {
		t.Fatal(err)
	}
	if len(functions) != 2 {
		t.Fatalf("got %d functions: %#v", len(functions), functions)
	}
	work := functions[0]
	if work.ID != "example.com/project/-/Work" || work.Complexity != 6 {
		t.Errorf("Work identity/complexity = %q/%d, want example.com/project/-/Work/6", work.ID, work.Complexity)
	}
	if math.Abs(work.Coverage-5.0/6.0) > 1e-9 {
		t.Errorf("weighted coverage = %v, want 5/6", work.Coverage)
	}
	run := functions[1]
	if run.ID != "example.com/project/Worker/Run" || run.Complexity != 2 || run.Coverage != 0 {
		t.Errorf("Run = %#v", run)
	}
}

func TestCoverprofileHelpers(t *testing.T) {
	root := t.TempDir()
	block, err := parseCoverLine("example.com/project/work.go:2.3,4.5 6 7", root, "example.com/project")
	if err != nil {
		t.Fatal(err)
	}
	if block.file != "work.go" || block.startLine != 2 || block.startCol != 3 || block.endLine != 4 || block.endCol != 5 || block.statements != 6 || block.count != 7 {
		t.Fatalf("block = %#v", block)
	}
	for _, text := range []string{"bad", "work.go 1 1", "work.go:2.3 1 1", "work.go:2.3,4.5 -1 1", "work.go:2.3,4.5 1 bad"} {
		if _, err := parseCoverLine(text, root, "example.com/project"); err == nil {
			t.Errorf("parseCoverLine(%q) succeeded", text)
		}
	}
	if _, _, err := parsePositions("2.3"); err == nil {
		t.Fatal("parsePositions accepted one position")
	}
	if _, err := parseNonnegative("-1", "statement"); err == nil {
		t.Fatal("parseNonnegative accepted negative")
	}
	abs := filepath.Join(root, "dir", "work.go")
	if got, err := normalizeCoverName(abs, root, "example.com/project"); err != nil || got != "dir/work.go" {
		t.Fatalf("normalizeCoverName = %q, %v", got, err)
	}
}

func TestParseCoverprofileErrors(t *testing.T) {
	root := t.TempDir()
	profile := filepath.Join(root, "cover.out")
	writeTestFile(t, profile, "mode: set\nbad\n")
	if _, err := parseCoverprofile(profile, root, "example.com/project"); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("parseCoverprofile error = %v", err)
	}
	if _, err := parseCoverprofile(filepath.Join(root, "missing"), root, "example.com/project"); err == nil {
		t.Fatal("parseCoverprofile accepted missing file")
	}
}

func TestScore(t *testing.T) {
	if got, want := Score(4, .5), 6.0; got != want {
		t.Fatalf("Score(4, .5) = %v, want %v", got, want)
	}
}

func TestGate(t *testing.T) {
	tests := []struct {
		name     string
		current  []Function
		baseline Baseline
		want     string
	}{
		{"passes accepted score", []Function{{ID: "p/-/f", CRAP: 31}}, Baseline{30, map[string]float64{"p/-/f": 31}}, ""},
		{"rejects new", []Function{{ID: "p/-/f", CRAP: 31}}, Baseline{30, map[string]float64{}}, "new:"},
		{"rejects worsening", []Function{{ID: "p/-/f", CRAP: 32}}, Baseline{30, map[string]float64{"p/-/f": 31}}, "worsened:"},
		{"rejects stale when improved", []Function{{ID: "p/-/f", CRAP: 30}}, Baseline{30, map[string]float64{"p/-/f": 31}}, "stale:"},
		{"rejects stale when removed", nil, Baseline{30, map[string]float64{"p/-/f": 31}}, "stale:"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Gate(test.current, test.baseline, 30)
			if test.want == "" && err != nil {
				t.Fatal(err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("Gate error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestBaselineRoundTripAndErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	functions := []Function{{ID: "p/-/high", CRAP: 31}, {ID: "p/-/low", CRAP: 30}}
	if err := WriteBaseline(path, 30, functions); err != nil {
		t.Fatal(err)
	}
	got, err := ReadBaseline(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Threshold != 30 || len(got.Scores) != 1 || got.Scores["p/-/high"] != 31 {
		t.Fatalf("baseline = %#v", got)
	}
	empty := filepath.Join(t.TempDir(), "empty.json")
	writeTestFile(t, empty, `{"threshold":30}`)
	got, err = ReadBaseline(empty)
	if err != nil || got.Scores == nil {
		t.Fatalf("nil scores normalization = %#v, %v", got, err)
	}
	invalid := filepath.Join(t.TempDir(), "invalid.json")
	writeTestFile(t, invalid, "{")
	if _, err := ReadBaseline(invalid); err == nil {
		t.Fatal("ReadBaseline accepted invalid JSON")
	}
	if _, err := ReadBaseline(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("ReadBaseline accepted missing file")
	}
	if err := WriteBaseline(filepath.Join(t.TempDir(), "missing", "baseline.json"), 30, nil); err == nil {
		t.Fatal("WriteBaseline accepted missing parent")
	}
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
}
