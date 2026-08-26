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

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
}
