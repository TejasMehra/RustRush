package rustrush

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	SupportCandidateWorkflow = "candidate_workflow"
	SupportAnalysisOnly      = "analysis_only"
	SupportUnsupported       = "unsupported"

	maxGuideCandidates = 20
)

type ProjectType struct {
	Name    string `json:"name"`
	Marker  string `json:"marker"`
	Support string `json:"support"`
}

type CandidateOperation struct {
	Function  string   `json:"function"`
	Location  string   `json:"location"`
	LoopDepth int      `json:"loop_depth"`
	Signals   []string `json:"signals"`
	Notes     string   `json:"notes"`
}

type GuideResult struct {
	Project        string               `json:"project"`
	Support        string               `json:"support"`
	ProjectTypes   []ProjectType        `json:"project_types"`
	MainPackages   []string             `json:"main_packages"`
	Candidates     []CandidateOperation `json:"candidate_operations"`
	Analysis       Analysis             `json:"analysis"`
	Limitations    []string             `json:"limitations"`
	NextSteps      []string             `json:"next_steps"`
	FilesNotParsed int                  `json:"go_files_not_parsed"`
}

var projectMarkers = []struct {
	file, name, support string
}{
	{"go.mod", "Go module", SupportCandidateWorkflow},
	{"package.json", "Node.js", SupportAnalysisOnly},
	{"pyproject.toml", "Python", SupportAnalysisOnly},
	{"requirements.txt", "Python", SupportAnalysisOnly},
	{"pom.xml", "Java (Maven)", SupportAnalysisOnly},
	{"build.gradle", "Java/Kotlin (Gradle)", SupportAnalysisOnly},
	{"Cargo.toml", "Rust", SupportAnalysisOnly},
	{"Gemfile", "Ruby", SupportAnalysisOnly},
	{"composer.json", "PHP", SupportAnalysisOnly},
}

// Guide inspects a project without executing it and reports whether the
// guided candidate workflow applies, plus low-confidence candidate operations.
func Guide(root string) (GuideResult, error) {
	analysis, err := Analyze(root)
	if err != nil {
		return GuideResult{}, err
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return GuideResult{}, fmt.Errorf("resolve project path: %w", err)
	}
	result := GuideResult{
		Project: analysis.Project, Support: SupportUnsupported, Analysis: analysis,
		ProjectTypes: []ProjectType{}, MainPackages: []string{}, Candidates: []CandidateOperation{},
		Limitations: []string{
			"The guided project workflow targets Go modules. The candidate command separately accepts a single Python baseline file; other languages are not executable baselines.",
			"Candidate operations are static hypotheses from source structure, not profiler output or evidence of a bottleneck.",
			"Candidate execution needs a Go main package or a single Python file, plus a hand-written single-file, dependency-free Rust candidate that follow the documented command contract.",
			"RustRush does not generate, translate, or upload code.",
		},
	}
	for _, marker := range projectMarkers {
		if info, err := os.Stat(filepath.Join(absolute, marker.file)); err == nil && !info.IsDir() {
			result.ProjectTypes = append(result.ProjectTypes, ProjectType{Name: marker.name, Marker: marker.file, Support: marker.support})
			if marker.support == SupportCandidateWorkflow {
				result.Support = SupportCandidateWorkflow
			} else if result.Support == SupportUnsupported {
				result.Support = SupportAnalysisOnly
			}
		}
	}
	if result.Support == SupportCandidateWorkflow {
		discoverGo(absolute, &result)
	}
	switch result.Support {
	case SupportCandidateWorkflow:
		result.NextSteps = []string{
			"Pick one operation above and write a Go main package that runs it deterministically: accept --iterations N, print a single deterministic result line, and read inputs only from --input FILE.",
			"Write a dependency-free single-file Rust candidate (candidate.rs) with the same contract.",
			"Run: rustrush candidate --baseline <go-main-dir> --candidate <candidate.rs> [--input <file>] --allow-execute",
			"RustRush copies both into a temporary directory, builds them there, requires identical outputs, and only then times them.",
		}
		if len(result.MainPackages) == 0 {
			result.Limitations = append(result.Limitations, "No Go main package was found; create a small benchmark main package that calls the operation under test.")
		}
	case SupportAnalysisOnly:
		result.NextSteps = []string{
			"Use the static findings to decide what to profile with your language's own profiler.",
			"For a direct candidate run, use a single Python baseline with --baseline-language python; otherwise create a small Go main package or use the built-in demos.",
		}
	default:
		result.NextSteps = []string{
			"No supported project marker (go.mod, package.json, pyproject.toml, ...) was found at the path; check that you passed the project root.",
			"Try the built-in demo: go run ./cmd/rustrush demo",
		}
		result.Limitations = append(result.Limitations, "No recognized project type was found at this path.")
	}
	return result, nil
}

func discoverGo(root string, result *GuideResult) {
	mains := map[string]bool{}
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			if path != root && (skippedDirectory(entry.Name()) || sensitiveFilename(entry.Name())) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 || sensitiveFilename(entry.Name()) ||
			filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		info, err := entry.Info()
		if err != nil || info.Size() > maxSourceFileBytes {
			result.FilesNotParsed++
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			result.FilesNotParsed++
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, contents, 0)
		if err != nil {
			result.FilesNotParsed++
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if file.Name.Name == "main" {
			dir := filepath.ToSlash(filepath.Dir(rel))
			mains[dir] = true
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || (function.Name.Name == "main" && function.Recv == nil) {
				continue
			}
			depth := maxLoopDepth(function.Body)
			signals := functionSignals(function.Body)
			if depth == 0 && len(signals) == 0 {
				continue
			}
			name := function.Name.Name
			if function.Recv != nil && len(function.Recv.List) == 1 {
				name = receiverName(function.Recv.List[0].Type) + "." + name
			}
			notes := "Contains loops; may be compute-bound."
			for _, signal := range signals {
				if signal == "database" || signal == "network" {
					notes = "Touches " + signal + " calls; I/O wait likely dominates, so a rewrite may not help."
				}
			}
			if depth == 0 {
				notes = "No loops; included only for its " + strings.Join(signals, "/") + " signal."
			}
			result.Candidates = append(result.Candidates, CandidateOperation{
				Function: name, Location: fmt.Sprintf("%s:%d", rel, fset.Position(function.Pos()).Line),
				LoopDepth: depth, Signals: signals, Notes: notes,
			})
		}
		return nil
	})
	for dir := range mains {
		result.MainPackages = append(result.MainPackages, dir)
	}
	sort.Strings(result.MainPackages)
	sort.SliceStable(result.Candidates, func(i, j int) bool {
		a, b := result.Candidates[i], result.Candidates[j]
		aIO, bIO := hasIOSignal(a), hasIOSignal(b)
		if aIO != bIO {
			return !aIO
		}
		if a.LoopDepth != b.LoopDepth {
			return a.LoopDepth > b.LoopDepth
		}
		return a.Location < b.Location
	})
	if len(result.Candidates) > maxGuideCandidates {
		result.Candidates = result.Candidates[:maxGuideCandidates]
	}
}

func hasIOSignal(candidate CandidateOperation) bool {
	for _, signal := range candidate.Signals {
		if signal == "database" || signal == "network" {
			return true
		}
	}
	return false
}

func receiverName(expression ast.Expr) string {
	switch typed := expression.(type) {
	case *ast.StarExpr:
		return receiverName(typed.X)
	case *ast.Ident:
		return typed.Name
	case *ast.IndexExpr:
		return receiverName(typed.X)
	default:
		return "?"
	}
}

func maxLoopDepth(body *ast.BlockStmt) int {
	maximum := 0
	var walk func(node ast.Node, depth int)
	walk = func(node ast.Node, depth int) {
		ast.Inspect(node, func(child ast.Node) bool {
			if child == node {
				return true
			}
			switch child.(type) {
			case *ast.ForStmt, *ast.RangeStmt:
				if depth+1 > maximum {
					maximum = depth + 1
				}
				walk(child, depth+1)
				return false
			}
			return true
		})
	}
	walk(body, 0)
	return maximum
}

func functionSignals(body *ast.BlockStmt) []string {
	found := map[string]bool{}
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		method := strings.ToLower(selector.Sel.Name)
		if receiver, ok := selector.X.(*ast.Ident); ok {
			if receiver.Name == "json" && (method == "marshal" || method == "unmarshal") {
				found["json"] = true
			}
			if receiver.Name == "http" && (method == "get" || method == "post") {
				found["network"] = true
			}
		}
		if method == "query" || method == "queryrow" || method == "exec" || method == "querycontext" || method == "execcontext" {
			found["database"] = true
		}
		return true
	})
	signals := make([]string, 0, len(found))
	for signal := range found {
		signals = append(signals, signal)
	}
	sort.Strings(signals)
	return signals
}
