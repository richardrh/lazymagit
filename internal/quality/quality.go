// Package quality computes dependency-free CRAP scores for Go code.
package quality

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Function is the quality measurement for one production function or method.
type Function struct {
	ID         string
	Complexity int
	Coverage   float64
	CRAP       float64
}

type sourceFunction struct {
	Function
	file       string
	bodyStart  token.Position
	bodyEnd    token.Position
	statements int
	covered    int
}

type coverBlock struct {
	file                string
	startLine, startCol int
	endLine, endCol     int
	statements, count   int
}

// Score applies the CRAP formula. Coverage is a fraction from zero to one.
func Score(complexity int, coverage float64) float64 {
	m := float64(complexity)
	return m*m*math.Pow(1-coverage, 3) + m
}

// Analyze parses production Go declarations under root and attributes the
// statements in a native Go coverprofile to them.
func Analyze(root, coverprofile string) ([]Function, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	module, err := modulePath(root)
	if err != nil {
		return nil, err
	}
	functions, err := parseFunctions(root, module)
	if err != nil {
		return nil, err
	}
	blocks, err := parseCoverprofile(coverprofile, root, module)
	if err != nil {
		return nil, err
	}
	for _, block := range blocks {
		best := -1
		for i := range functions {
			fn := &functions[i]
			if fn.file != block.file || !contains(fn, block) {
				continue
			}
			if best < 0 || positionSpan(fn) < positionSpan(&functions[best]) {
				best = i
			}
		}
		if best >= 0 {
			functions[best].statements += block.statements
			if block.count > 0 {
				functions[best].covered += block.statements
			}
		}
	}
	result := make([]Function, len(functions))
	for i := range functions {
		fn := &functions[i]
		if fn.statements > 0 {
			fn.Coverage = float64(fn.covered) / float64(fn.statements)
		}
		fn.CRAP = Score(fn.Complexity, fn.Coverage)
		result[i] = fn.Function
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func parseFunctions(root, module string) ([]sourceFunction, error) {
	set := token.NewFileSet()
	var result []sourceFunction
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "vendor" || entry.Name() == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(set, path, nil, 0)
		if err != nil {
			return err
		}
		relDir, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		pkg := module
		if relDir != "." {
			pkg += "/" + filepath.ToSlash(relDir)
		}
		relFile, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			receiver := "-"
			if fn.Recv != nil && len(fn.Recv.List) != 0 {
				receiver = receiverName(fn.Recv.List[0].Type)
			}
			result = append(result, sourceFunction{
				Function:  Function{ID: pkg + "/" + receiver + "/" + fn.Name.Name, Complexity: complexity(fn.Body)},
				file:      filepath.ToSlash(relFile),
				bodyStart: set.Position(fn.Body.Pos()),
				bodyEnd:   set.Position(fn.Body.End()),
			})
		}
		return nil
	})
	return result, err
}

func receiverName(expr ast.Expr) string {
	switch value := expr.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.StarExpr:
		return receiverName(value.X)
	case *ast.IndexExpr:
		return receiverName(value.X)
	case *ast.IndexListExpr:
		return receiverName(value.X)
	default:
		return "?"
	}
}

func complexity(body *ast.BlockStmt) int {
	value := 1
	ast.Inspect(body, func(node ast.Node) bool {
		if literal, ok := node.(*ast.FuncLit); ok && literal != nil {
			return false
		}
		switch node := node.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt:
			value++
		case *ast.CaseClause:
			if node.List != nil { // nil is default
				value++
			}
		case *ast.CommClause:
			if node.Comm != nil { // nil is default
				value++
			}
		case *ast.BinaryExpr:
			if node.Op == token.LAND || node.Op == token.LOR {
				value++
			}
		}
		return true
	})
	return value
}

func parseCoverprofile(path, root, module string) ([]coverBlock, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open coverprofile: %w", err)
	}
	defer file.Close()
	var blocks []coverBlock
	scanner := bufio.NewScanner(file)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if line == 1 && strings.HasPrefix(text, "mode:") {
			continue
		}
		fields := strings.Fields(text)
		if len(fields) != 3 {
			return nil, fmt.Errorf("coverprofile line %d: malformed entry", line)
		}
		colon := strings.LastIndex(fields[0], ":")
		if colon < 0 {
			return nil, fmt.Errorf("coverprofile line %d: missing position", line)
		}
		positions := strings.Split(fields[0][colon+1:], ",")
		if len(positions) != 2 {
			return nil, fmt.Errorf("coverprofile line %d: malformed position", line)
		}
		start, err := parsePosition(positions[0])
		if err != nil {
			return nil, fmt.Errorf("coverprofile line %d: %w", line, err)
		}
		end, err := parsePosition(positions[1])
		if err != nil {
			return nil, fmt.Errorf("coverprofile line %d: %w", line, err)
		}
		statements, err := strconv.Atoi(fields[1])
		if err != nil || statements < 0 {
			return nil, fmt.Errorf("coverprofile line %d: invalid statement count", line)
		}
		count, err := strconv.Atoi(fields[2])
		if err != nil || count < 0 {
			return nil, fmt.Errorf("coverprofile line %d: invalid execution count", line)
		}
		name := filepath.FromSlash(fields[0][:colon])
		if strings.HasPrefix(filepath.ToSlash(name), module+"/") {
			name = filepath.FromSlash(strings.TrimPrefix(filepath.ToSlash(name), module+"/"))
		} else if filepath.IsAbs(name) {
			name, err = filepath.Rel(root, name)
			if err != nil {
				return nil, err
			}
		}
		blocks = append(blocks, coverBlock{filepath.ToSlash(filepath.Clean(name)), start[0], start[1], end[0], end[1], statements, count})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return blocks, nil
}

func parsePosition(value string) ([2]int, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return [2]int{}, errors.New("malformed line.column")
	}
	line, err := strconv.Atoi(parts[0])
	if err != nil {
		return [2]int{}, errors.New("invalid line")
	}
	column, err := strconv.Atoi(parts[1])
	if err != nil {
		return [2]int{}, errors.New("invalid column")
	}
	return [2]int{line, column}, nil
}

func contains(fn *sourceFunction, block coverBlock) bool {
	startsAfter := block.startLine > fn.bodyStart.Line || block.startLine == fn.bodyStart.Line && block.startCol >= fn.bodyStart.Column
	endsBefore := block.endLine < fn.bodyEnd.Line || block.endLine == fn.bodyEnd.Line && block.endCol <= fn.bodyEnd.Column
	return startsAfter && endsBefore
}

func positionSpan(fn *sourceFunction) int {
	return (fn.bodyEnd.Line-fn.bodyStart.Line)*100000 + fn.bodyEnd.Column - fn.bodyStart.Column
}

func modulePath(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" {
			return fields[1], nil
		}
	}
	return "", errors.New("go.mod has no module directive")
}

// Baseline is the checked-in set of accepted scores over the threshold.
type Baseline struct {
	Threshold float64            `json:"threshold"`
	Scores    map[string]float64 `json:"scores"`
}

// ReadBaseline reads a baseline JSON file.
func ReadBaseline(path string) (Baseline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Baseline{}, err
	}
	var baseline Baseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return Baseline{}, err
	}
	if baseline.Scores == nil {
		baseline.Scores = map[string]float64{}
	}
	return baseline, nil
}

// WriteBaseline replaces path with the current scores over threshold.
func WriteBaseline(path string, threshold float64, functions []Function) error {
	baseline := Baseline{Threshold: threshold, Scores: map[string]float64{}}
	for _, fn := range functions {
		if fn.CRAP > threshold {
			baseline.Scores[fn.ID] = fn.CRAP
		}
	}
	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

// Gate rejects new regressions, worsened accepted scores, and stale entries.
func Gate(functions []Function, baseline Baseline, threshold float64) error {
	if math.Abs(baseline.Threshold-threshold) > 1e-9 {
		return fmt.Errorf("baseline threshold %.2f does not match gate threshold %.2f (run with -update)", baseline.Threshold, threshold)
	}
	current := make(map[string]float64)
	var failures []string
	for _, fn := range functions {
		if fn.CRAP <= threshold {
			continue
		}
		current[fn.ID] = fn.CRAP
		old, exists := baseline.Scores[fn.ID]
		if !exists {
			failures = append(failures, fmt.Sprintf("new: %s is %.2f", fn.ID, fn.CRAP))
		} else if fn.CRAP > old+1e-9 {
			failures = append(failures, fmt.Sprintf("worsened: %s is %.2f, baseline %.2f", fn.ID, fn.CRAP, old))
		}
	}
	for id := range baseline.Scores {
		if _, exists := current[id]; !exists {
			failures = append(failures, "stale: "+id)
		}
	}
	sort.Strings(failures)
	if len(failures) != 0 {
		return errors.New(strings.Join(failures, "\n"))
	}
	return nil
}
