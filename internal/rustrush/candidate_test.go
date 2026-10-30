package rustrush

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

type candidateFixture struct {
	baseline, candidate, input string
}

func newCandidateFixture(t *testing.T) candidateFixture {
	t.Helper()
	root := t.TempDir()
	f := candidateFixture{
		baseline:  filepath.Join(root, "baseline"),
		candidate: filepath.Join(root, "candidate.rs"),
		input:     filepath.Join(root, "input.txt"),
	}
	writeTestFile(t, filepath.Join(f.baseline, "go.mod"), "module example.com/b\n\ngo 1.22\n")
	writeTestFile(t, filepath.Join(f.baseline, "main.go"), "package main\n\nfunc main() {}\n")
	writeTestFile(t, filepath.Join(f.baseline, ".env"), "TOKEN=do-not-copy\n")
	writeTestFile(t, filepath.Join(f.baseline, "node_modules", "x.js"), "x")
	writeTestFile(t, f.candidate, "fn main() { println!(\"1\"); }\n")
	writeTestFile(t, f.input, "1\n2\n")
	return f
}

func (f candidateFixture) options() CandidateOptions {
	return CandidateOptions{
		BaselineDir: f.baseline, CandidateFile: f.candidate, InputFile: f.input,
		Iterations: 100, Repetitions: 3, Timeout: time.Second, AllowExecute: true,
		build:    func(context.Context, string, []string, string, []string) (string, error) { return "", nil },
		lookPath: func(name string) (string, error) { return filepath.Join("fake", name), nil },
	}
}

func hashTree(t *testing.T, root string) map[string][32]byte {
	t.Helper()
	result := map[string][32]byte{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		result[path] = sha256.Sum256(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

type fakeRuns struct {
	calls     []string
	dirs      []string
	args      [][]string
	baselineD uint64
	candD     uint64
	candOut   string
	candErr   error
}

func (r *fakeRuns) run(_ context.Context, exe string, args []string, dir string, _ []string) (Sample, error) {
	name := strings.TrimSuffix(filepath.Base(exe), ".exe")
	r.calls = append(r.calls, name)
	r.dirs = append(r.dirs, dir)
	r.args = append(r.args, args)
	if name == "candidate" {
		if r.candErr != nil {
			return Sample{}, r.candErr
		}
		return Sample{DurationNS: r.candD, Output: r.candOut}, nil
	}
	return Sample{DurationNS: r.baselineD, Output: "42"}, nil
}

func TestRunCandidateRequiresExplicitConsent(t *testing.T) {
	f := newCandidateFixture(t)
	options := f.options()
	options.AllowExecute = false
	if _, err := RunCandidate(context.Background(), options); err == nil || !strings.Contains(err.Error(), "--allow-execute") {
		t.Fatalf("expected consent error, got %v", err)
	}
}

func TestRunCandidatePairedMeasurementPreservesSource(t *testing.T) {
	f := newCandidateFixture(t)
	before := hashTree(t, filepath.Dir(f.candidate))
	runs := &fakeRuns{baselineD: 2_000_000, candD: 1_000_000, candOut: "42"}
	options := f.options()
	options.runProcess = runs.run
	report, err := RunCandidate(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if report.Correctness != "PASSED_FOR_CHECKSUM" || report.Verdict != VerdictPort {
		t.Fatalf("unexpected report: %s %s %s", report.Correctness, report.Verdict, report.Reason)
	}
	if report.ThroughputDeltaPercent == nil || *report.ThroughputDeltaPercent < 99 || *report.ThroughputDeltaPercent > 101 {
		t.Fatalf("delta must be derived from the measured samples: %v", report.ThroughputDeltaPercent)
	}
	for _, m := range report.Measurements {
		if m.Status != "measured" || m.WarmupRuns != 1 || len(m.Samples) != 3 || m.ErrorCount != 0 {
			t.Fatalf("unexpected measurement: %+v", m)
		}
	}
	if len(runs.calls) != 2+2*3 {
		t.Fatalf("expected 2 warm-ups plus 6 timed runs, got %v", runs.calls)
	}
	for i, args := range runs.args {
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "--input") || !strings.Contains(joined, "--iterations 100") {
			t.Fatalf("run %d missing contract flags: %v", i, args)
		}
		if strings.Contains(joined, f.input) || runs.dirs[i] == f.baseline || strings.HasPrefix(runs.dirs[i], filepath.Dir(f.candidate)) {
			t.Fatalf("run %d touched original paths: dir=%s args=%v", i, runs.dirs[i], args)
		}
	}
	after := hashTree(t, filepath.Dir(f.candidate))
	if len(before) != len(after) {
		t.Fatalf("file set changed: %d vs %d", len(before), len(after))
	}
	for path, sum := range before {
		if after[path] != sum {
			t.Fatalf("%s was modified", path)
		}
	}
	if err := roundTripReport(report); err != nil {
		t.Fatal(err)
	}
}

func TestRunCandidatePythonBaselineUsesSharedCorrectnessGateAndReport(t *testing.T) {
	f := newCandidateFixture(t)
	python := filepath.Join(filepath.Dir(f.baseline), "baseline.py")
	writeTestFile(t, python, "print('42')\n")
	runs := &fakeRuns{baselineD: 2_000_000, candD: 1_000_000, candOut: "42"}
	var syntaxChecked bool
	options := f.options()
	options.BaselineLanguage = "python"
	options.BaselineDir = python
	options.runProcess = runs.run
	options.build = func(_ context.Context, tool string, args []string, _ string, _ []string) (string, error) {
		if filepath.Base(tool) == "python" || filepath.Base(tool) == "python.exe" {
			if len(args) == 1 && args[0] == "--version" {
				return "Python 3.12.0\n", nil
			}
			if strings.Contains(strings.Join(args, " "), "compile(pathlib.Path") {
				syntaxChecked = true
				return "", nil
			}
		}
		return "", nil
	}
	report, err := RunCandidate(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if !syntaxChecked {
		t.Fatal("Python source must be syntax-checked before benchmarking")
	}
	if report.Correctness != "PASSED_FOR_CHECKSUM" || report.Verdict != VerdictPort {
		t.Fatalf("unexpected report: %s %s %s", report.Correctness, report.Verdict, report.Reason)
	}
	if report.Environment["baseline_language"] != "python" || report.Environment["python_runtime"] != "Python 3.12.0" {
		t.Fatalf("Python runtime metadata missing: %+v", report.Environment)
	}
	if report.Measurements[0].Name != "Python baseline" || report.Measurements[1].Name != "Rust candidate" {
		t.Fatalf("unexpected measurement names: %+v", report.Measurements)
	}
	if !strings.Contains(RenderMarkdown(report), "Rust candidate vs Python baseline") {
		t.Fatal("Markdown must identify the actual baseline language")
	}
	if len(runs.calls) != 8 {
		t.Fatalf("expected two correctness probes and six timed runs, got %d", len(runs.calls))
	}
	for i, args := range runs.args {
		if !strings.Contains(strings.Join(args, " "), "--iterations 100") {
			t.Fatalf("run %d missing iteration contract: %v", i, args)
		}
		if runs.calls[i] == "python" {
			joined := strings.Join(args, " ")
			for _, required := range []string{"-s", "-S", "-B", "baseline.py", "--input"} {
				if !strings.Contains(joined, required) {
					t.Fatalf("Python baseline run missing %q: %v", required, args)
				}
			}
		} else if strings.Contains(strings.Join(args, " "), "baseline.py") {
			t.Fatalf("Rust candidate received Python interpreter arguments: %v", args)
		}
	}
	if err := roundTripReport(report); err != nil {
		t.Fatal(err)
	}
}

func TestRunCandidatePythonBaselineSyntaxFailureBlocksExecution(t *testing.T) {
	f := newCandidateFixture(t)
	python := filepath.Join(filepath.Dir(f.baseline), "baseline.py")
	writeTestFile(t, python, "print(\n")
	runs := &fakeRuns{}
	options := f.options()
	options.BaselineLanguage = "python"
	options.BaselineDir = python
	options.runProcess = runs.run
	options.build = func(_ context.Context, tool string, args []string, _ string, _ []string) (string, error) {
		if filepath.Base(tool) == "python" && len(args) == 1 && args[0] == "--version" {
			return "Python 3.12.0\n", nil
		}
		if strings.Contains(strings.Join(args, " "), "compile(pathlib.Path") {
			return "SyntaxError: invalid syntax\nprint(\n", errors.New("exit status 1")
		}
		return "", nil
	}
	report, err := RunCandidate(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if report.Measurements[0].Status != "build_failed" || report.Measurements[0].ErrorCount != 1 ||
		report.Verdict != VerdictInconclusive || len(runs.calls) != 0 {
		t.Fatalf("syntax-invalid baseline must block all program execution: %+v", report)
	}
	if !strings.Contains(report.Measurements[0].Error, "SyntaxError") {
		t.Fatalf("syntax error should be reported: %+v", report.Measurements[0])
	}
	if strings.Contains(report.Measurements[0].Error, "print(") {
		t.Fatal("syntax diagnostics must not copy source lines into the report")
	}
}

func TestRunCandidatePythonBaselineFailureIsInconclusive(t *testing.T) {
	f := newCandidateFixture(t)
	python := filepath.Join(filepath.Dir(f.baseline), "baseline.py")
	writeTestFile(t, python, "print('42')\n")
	runs := &fakeRuns{candErr: context.DeadlineExceeded}
	options := f.options()
	options.BaselineLanguage = "python"
	options.BaselineDir = python
	options.runProcess = runs.run
	options.build = func(_ context.Context, tool string, args []string, _ string, _ []string) (string, error) {
		if filepath.Base(tool) == "python" && len(args) == 1 && args[0] == "--version" {
			return "Python 3.12.0\n", nil
		}
		return "", nil
	}
	report, err := RunCandidate(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if report.Measurements[0].Name != "Python baseline" || report.Measurements[1].Status != "timed_out" ||
		report.Verdict != VerdictInconclusive || report.ThroughputDeltaPercent != nil {
		t.Fatalf("Python comparison failure must be inconclusive: %+v", report)
	}
}

func TestRunCandidatePythonCorrectnessMismatchPreventsTimedTrials(t *testing.T) {
	f := newCandidateFixture(t)
	python := filepath.Join(filepath.Dir(f.baseline), "baseline.py")
	writeTestFile(t, python, "print('42')\n")
	runs := &fakeRuns{baselineD: 1_000_000, candOut: "different"}
	options := f.options()
	options.BaselineLanguage = "python"
	options.BaselineDir = python
	options.runProcess = runs.run
	options.build = func(_ context.Context, tool string, args []string, _ string, _ []string) (string, error) {
		if filepath.Base(tool) == "python" && len(args) == 1 && args[0] == "--version" {
			return "Python 3.12.0\n", nil
		}
		return "", nil
	}
	report, err := RunCandidate(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if report.Correctness != "FAILED" || report.Verdict != VerdictInconclusive ||
		report.ThroughputDeltaPercent != nil || len(runs.calls) != 2 {
		t.Fatalf("Python output mismatch must prevent timed trials: %+v calls=%v", report, runs.calls)
	}
}

func TestRunCandidatePythonBaselineRejectsContainerAndMissingRuntime(t *testing.T) {
	f := newCandidateFixture(t)
	python := filepath.Join(filepath.Dir(f.baseline), "baseline.py")
	writeTestFile(t, python, "print('42')\n")

	options := f.options()
	options.BaselineLanguage = "python"
	options.BaselineDir = python
	options.Isolation = IsolationContainer
	if _, err := RunCandidate(context.Background(), options); err == nil || !strings.Contains(err.Error(), "not supported with container") {
		t.Fatalf("expected explicit Python container rejection, got %v", err)
	}

	options.Isolation = IsolationProcess
	options.lookPath = func(name string) (string, error) {
		if name == "go" || name == "rustc" {
			return filepath.Join("fake", name), nil
		}
		return "", errors.New("not found")
	}
	if _, err := RunCandidate(context.Background(), options); err == nil || !strings.Contains(err.Error(), "Python 3 interpreter") {
		t.Fatalf("expected missing Python runtime error, got %v", err)
	}
}

// roundTripReport round-trips a report through the public writer/reader.
func roundTripReport(report Report) error {
	dir, err := os.MkdirTemp("", "rr-report-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := WriteReport(dir, report); err != nil {
		return err
	}
	loaded, err := ReadReport(filepath.Join(dir, "latest.json"))
	if err != nil {
		return err
	}
	if loaded.SchemaVersion != SchemaVersion || loaded.Verdict != report.Verdict {
		return errors.New("report did not round-trip")
	}
	return nil
}

func TestRunCandidateCorrectnessMismatchBlocksTiming(t *testing.T) {
	f := newCandidateFixture(t)
	runs := &fakeRuns{baselineD: 1_000_000, candD: 1, candOut: "43"}
	options := f.options()
	options.runProcess = runs.run
	report, err := RunCandidate(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if report.Correctness != "FAILED" || report.Verdict != VerdictInconclusive || report.ThroughputDeltaPercent != nil {
		t.Fatalf("mismatch must be inconclusive without a delta: %+v", report)
	}
	if len(runs.calls) != 2 {
		t.Fatalf("no timed runs may start after a mismatch, got %v", runs.calls)
	}
}

func TestRunCandidateEmptyOutputFailsGate(t *testing.T) {
	f := newCandidateFixture(t)
	runs := &fakeRuns{baselineD: 1, candD: 1, candOut: ""}
	options := f.options()
	options.runProcess = runs.run
	report, err := RunCandidate(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if report.Correctness != "FAILED" || report.Verdict != VerdictInconclusive {
		t.Fatalf("unexpected: %+v", report)
	}
}

func TestRunCandidateTimeoutAndFailureAreInconclusive(t *testing.T) {
	for name, runErr := range map[string]error{
		"timeout": context.DeadlineExceeded,
		"failure": errors.New("exit status 1"),
	} {
		t.Run(name, func(t *testing.T) {
			f := newCandidateFixture(t)
			runs := &fakeRuns{baselineD: 1_000_000, candErr: runErr}
			options := f.options()
			options.runProcess = runs.run
			report, err := RunCandidate(context.Background(), options)
			if err != nil {
				t.Fatal(err)
			}
			candidate := report.Measurements[1]
			want := "failed"
			if name == "timeout" {
				want = "timed_out"
			}
			if candidate.Status != want || candidate.ErrorCount != 1 || report.Verdict != VerdictInconclusive || report.ThroughputDeltaPercent != nil {
				t.Fatalf("unexpected: %+v verdict=%s", candidate, report.Verdict)
			}
		})
	}
}

func TestRunCandidateBuildFailuresAreReportedAndRedacted(t *testing.T) {
	f := newCandidateFixture(t)
	options := f.options()
	options.build = func(_ context.Context, tool string, args []string, dir string, _ []string) (string, error) {
		if strings.HasSuffix(tool, "rustc") || strings.HasSuffix(tool, "rustc.exe") {
			return "error in " + dir + " at " + args[3], errors.New("exit status 1")
		}
		return "", nil
	}
	options.runProcess = (&fakeRuns{}).run
	report, err := RunCandidate(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	candidate := report.Measurements[1]
	if candidate.Status != "build_failed" || candidate.ErrorCount != 1 || report.Verdict != VerdictInconclusive {
		t.Fatalf("unexpected: %+v", candidate)
	}
	if strings.Contains(candidate.Error, os.TempDir()) || !strings.Contains(candidate.Error, "[") {
		t.Fatalf("build output was not redacted: %q", candidate.Error)
	}
	if !strings.Contains(RenderMarkdown(report), "**Rust candidate error:**") {
		t.Fatal("markdown must surface the build error")
	}

	options = f.options()
	options.build = func(context.Context, string, []string, string, []string) (string, error) {
		return "", errors.New("boom")
	}
	report, err = RunCandidate(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if report.Measurements[0].Status != "build_failed" || report.Measurements[1].Status != "not_run" || report.Verdict != VerdictInconclusive {
		t.Fatalf("unexpected: %+v", report.Measurements)
	}
}

func TestRunCandidateWithoutRustcMeasuresOnlyBaseline(t *testing.T) {
	f := newCandidateFixture(t)
	options := f.options()
	options.lookPath = func(name string) (string, error) {
		if name == "rustc" {
			return "", errors.New("not found")
		}
		return "go", nil
	}
	options.runProcess = (&fakeRuns{baselineD: 1_000_000}).run
	report, err := RunCandidate(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != VerdictInconclusive || report.Measurements[0].Status != "measured" || report.Measurements[1].Status != "unavailable" {
		t.Fatalf("unexpected: %+v", report.Measurements)
	}
	if report.ThroughputDeltaPercent != nil {
		t.Fatal("no delta may be reported without a candidate")
	}
}

func TestRunCandidateRejectsUnsupportedInputs(t *testing.T) {
	f := newCandidateFixture(t)
	cases := map[string]func(*CandidateOptions){
		"not a rust file": func(o *CandidateOptions) {
			p := filepath.Join(filepath.Dir(f.candidate), "candidate.py")
			writeTestFile(t, p, "print(1)")
			o.CandidateFile = p
		},
		"include_str": func(o *CandidateOptions) {
			p := filepath.Join(filepath.Dir(f.candidate), "evil.rs")
			writeTestFile(t, p, "fn main(){ let _ = include_str!(\"/etc/passwd\"); }")
			o.CandidateFile = p
		},
		"extern crate": func(o *CandidateOptions) {
			p := filepath.Join(filepath.Dir(f.candidate), "dep.rs")
			writeTestFile(t, p, "extern crate serde;\nfn main(){}")
			o.CandidateFile = p
		},
		"missing candidate": func(o *CandidateOptions) { o.CandidateFile = filepath.Join(filepath.Dir(f.candidate), "none.rs") },
		"empty candidate":   func(o *CandidateOptions) { o.CandidateFile = "" },
		"missing input":     func(o *CandidateOptions) { o.InputFile = filepath.Join(filepath.Dir(f.candidate), "none.txt") },
		"baseline is file":  func(o *CandidateOptions) { o.BaselineDir = f.candidate },
		"empty baseline":    func(o *CandidateOptions) { o.BaselineDir = "" },
		"no go.mod": func(o *CandidateOptions) {
			d := t.TempDir()
			writeTestFile(t, filepath.Join(d, "main.go"), "package main\nfunc main(){}")
			o.BaselineDir = d
		},
		"no main package": func(o *CandidateOptions) {
			d := t.TempDir()
			writeTestFile(t, filepath.Join(d, "go.mod"), "module x\n\ngo 1.22\n")
			writeTestFile(t, filepath.Join(d, "lib.go"), "package lib\n")
			o.BaselineDir = d
		},
		"zero iterations": func(o *CandidateOptions) { o.Iterations = 0 },
		"too few trials":  func(o *CandidateOptions) { o.Repetitions = 2 },
		"invalid timeout": func(o *CandidateOptions) { o.Timeout = 0 },
		"nodejs directory": func(o *CandidateOptions) {
			d := t.TempDir()
			writeTestFile(t, filepath.Join(d, "package.json"), "{}")
			o.BaselineDir = d
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			options := f.options()
			options.runProcess = func(context.Context, string, []string, string, []string) (Sample, error) {
				t.Fatal("nothing may execute for rejected input")
				return Sample{}, nil
			}
			mutate(&options)
			if _, err := RunCandidate(context.Background(), options); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestCopyTreeSkipsSensitiveAndDependencyFiles(t *testing.T) {
	f := newCandidateFixture(t)
	destination := filepath.Join(t.TempDir(), "copy")
	if err := copyTree(f.baseline, destination); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination, "main.go")); err != nil {
		t.Fatal("main.go should be copied")
	}
	for _, skipped := range []string{".env", "node_modules"} {
		if _, err := os.Stat(filepath.Join(destination, skipped)); err == nil {
			t.Fatalf("%s must not be copied", skipped)
		}
	}
}

func TestGuideClassifiesProjects(t *testing.T) {
	goDir := t.TempDir()
	writeTestFile(t, filepath.Join(goDir, "go.mod"), "module x\n\ngo 1.22\n")
	writeTestFile(t, filepath.Join(goDir, "main.go"), `package main

import ("database/sql"; "encoding/json")

func hot(values []int) int {
	total := 0
	for _, a := range values {
		for _, b := range values {
			total += a * b
		}
	}
	return total
}

func fetch(db *sql.DB) {
	for i := 0; i < 3; i++ {
		db.Query("SELECT 1")
	}
	json.Marshal(1)
}

func main() { hot(nil); fetch(nil) }
`)
	writeTestFile(t, filepath.Join(goDir, "broken.go"), "package main\nfunc (")
	result, err := Guide(goDir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Support != SupportCandidateWorkflow || len(result.MainPackages) != 1 || result.MainPackages[0] != "." {
		t.Fatalf("unexpected: %+v", result)
	}
	if len(result.Candidates) != 2 || result.Candidates[0].Function != "hot" || result.Candidates[0].LoopDepth != 2 {
		t.Fatalf("compute-bound candidate must rank first: %+v", result.Candidates)
	}
	if result.Candidates[1].Function != "fetch" || !strings.Contains(result.Candidates[1].Notes, "I/O wait") {
		t.Fatalf("I/O candidate must carry a warning: %+v", result.Candidates[1])
	}
	if result.FilesNotParsed != 1 {
		t.Fatalf("unparseable files must be counted, got %d", result.FilesNotParsed)
	}
	if len(result.Limitations) == 0 || len(result.NextSteps) == 0 {
		t.Fatal("guide must explain limits and next steps")
	}
}

func TestGuideHandlesUnsupportedAndInvalidProjects(t *testing.T) {
	node := t.TempDir()
	writeTestFile(t, filepath.Join(node, "package.json"), "{}")
	writeTestFile(t, filepath.Join(node, "index.js"), "for (;;) {}")
	result, err := Guide(node)
	if err != nil {
		t.Fatal(err)
	}
	if result.Support != SupportAnalysisOnly || len(result.Candidates) != 0 {
		t.Fatalf("node must be analysis-only: %+v", result)
	}

	empty := t.TempDir()
	result, err = Guide(empty)
	if err != nil {
		t.Fatal(err)
	}
	if result.Support != SupportUnsupported || len(result.ProjectTypes) != 0 {
		t.Fatalf("empty dir must be unsupported: %+v", result)
	}
	if _, err := Guide(filepath.Join(empty, "missing")); err == nil {
		t.Fatal("missing path must error")
	}
}

func TestGuideNeverReadsCredentialFiles(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "go.mod"), "module x\n\ngo 1.22\n")
	writeTestFile(t, filepath.Join(dir, "secret_token.go"), "package main\nfunc leaked() { for {} }\n")
	result, err := Guide(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range result.Candidates {
		if candidate.Function == "leaked" {
			t.Fatal("credential-like files must be skipped")
		}
	}
}
