package rustrush

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func isNodeTool(tool string) bool {
	switch strings.TrimSuffix(filepath.Base(tool), ".exe") {
	case "node", "nodejs":
		return true
	}
	return false
}

// nodeOptions returns candidate options for a Node.js baseline with a fake Node.js 20 runtime.
func nodeOptions(t *testing.T, f candidateFixture, script, source string) (CandidateOptions, *[][]string) {
	t.Helper()
	path := filepath.Join(filepath.Dir(f.baseline), script)
	writeTestFile(t, path, source)
	var buildCalls [][]string
	options := f.options()
	options.BaselineLanguage = "nodejs"
	options.BaselineDir = path
	options.build = func(_ context.Context, tool string, args []string, _ string, _ []string) (string, error) {
		if isNodeTool(tool) {
			buildCalls = append(buildCalls, args)
			if len(args) == 1 && args[0] == "--version" {
				return "v20.15.1\n", nil
			}
		}
		return "", nil
	}
	return options, &buildCalls
}

func TestParseNodeVersion(t *testing.T) {
	valid := map[string]string{
		"v20.15.1\n":          "Node.js v20.15.1",
		"v18.0.0":             "Node.js v18.0.0",
		"v22.0.0-nightly2024": "Node.js v22.0.0-nightly2024",
	}
	for input, want := range valid {
		if got, err := parseNodeVersion(input); err != nil || got != want {
			t.Errorf("parseNodeVersion(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"", "\n", "Python 3.12.0", "20.1.0", "v20", "v20.x.1", "v16.20.2", "v17.9.1", "v20.1.0 extra"} {
		if got, err := parseNodeVersion(input); err == nil || got != "" {
			t.Errorf("parseNodeVersion(%q) must be rejected, got %q", input, got)
		}
	}
}

func TestFindNodeDiscovery(t *testing.T) {
	var asked []string
	found := func(available ...string) func(string) (string, error) {
		return func(name string) (string, error) {
			asked = append(asked, name)
			for _, candidate := range available {
				if candidate == name {
					return "/usr/bin/" + name, nil
				}
			}
			return "", errors.New("not found")
		}
	}
	if got, err := findNode(found("node", "nodejs")); err != nil || filepath.Base(got) != "node" || len(asked) != 1 {
		t.Fatalf("node must be preferred: %q %v %v", got, err, asked)
	}
	if runtime.GOOS != "windows" {
		if got, err := findNode(found("nodejs")); err != nil || filepath.Base(got) != "nodejs" {
			t.Fatalf("expected nodejs fallback: %q %v", got, err)
		}
	}
	if _, err := findNode(found()); err == nil || !strings.Contains(err.Error(), "Node.js 18 or newer") {
		t.Fatalf("missing Node.js must be actionable, got %v", err)
	}
}

func TestNormalizeBaselineLanguage(t *testing.T) {
	for input, want := range map[string]string{"nodejs": "nodejs", "Node": "nodejs", " node.js ": "nodejs", "GO": "go", "python": "python", "ruby": "ruby"} {
		if got := normalizeBaselineLanguage(input); got != want {
			t.Errorf("normalizeBaselineLanguage(%q) = %q, want %q", input, got, want)
		}
	}
	if baselineMeasurementName("nodejs") != "Node.js baseline" || baselineMeasurementName("") != "Go baseline" ||
		baselineMeasurementName("python") != "Python baseline" {
		t.Fatal("baseline measurement names changed")
	}
}

func TestValidateNodeBaseline(t *testing.T) {
	dir := t.TempDir()
	spaced := filepath.Join(dir, "my folder", "base line.js")
	writeTestFile(t, spaced, "console.log('1')\n")
	got, err := validateNodeBaseline(spaced)
	if err != nil || !filepath.IsAbs(got) {
		t.Fatalf("valid path with spaces rejected: %q %v", got, err)
	}
	for _, name := range []string{"a.cjs", "b.mjs", "C.JS"} {
		writeTestFile(t, filepath.Join(dir, name), "1\n")
		if _, err := validateNodeBaseline(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s should be accepted: %v", name, err)
		}
	}
	writeTestFile(t, filepath.Join(dir, "empty.js"), " \n\t\n")
	writeTestFile(t, filepath.Join(dir, "script.py"), "print(1)\n")
	writeTestFile(t, filepath.Join(dir, "script.ts"), "1\n")
	cases := map[string]string{
		"":                               "required",
		filepath.Join(dir, "missing.js"): "",
		filepath.Join(dir, "empty.js"):   "empty",
		filepath.Join(dir, "script.py"):  "single .js",
		filepath.Join(dir, "script.ts"):  "single .js",
		dir:                              "single .js",
		filepath.Join(dir, "my folder"):  "single .js",
	}
	for path, want := range cases {
		if _, err := validateNodeBaseline(path); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("validateNodeBaseline(%q) = %v, want error containing %q", path, err, want)
		}
	}
}

func TestNodeScriptNamePreservesModuleKind(t *testing.T) {
	for input, want := range map[string]string{"/x/a.js": "baseline.js", "/x/a.MJS": "baseline.mjs", "/x/a.cjs": "baseline.cjs"} {
		if got := nodeScriptName(input); got != want {
			t.Errorf("nodeScriptName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNodeEnvironmentIsSanitized(t *testing.T) {
	t.Setenv("NODE_OPTIONS", "--require /evil.js")
	t.Setenv("NODE_PATH", "/evil")
	t.Setenv("RUSTRUSH_SECRET_TOKEN", "secret")
	t.Setenv("NPM_TOKEN", "secret")
	joined := strings.Join(nodeEnvironment(), "\n")
	for _, forbidden := range []string{"NODE_OPTIONS", "NODE_PATH", "SECRET", "NPM_TOKEN"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("%s leaked into the Node.js environment: %s", forbidden, joined)
		}
	}
	if !strings.Contains(joined, "NODE_NO_WARNINGS=1") {
		t.Fatal("NODE_NO_WARNINGS must be set")
	}
}

func TestRunCandidateNodeBaselineUsesSharedPairedEngine(t *testing.T) {
	f := newCandidateFixture(t)
	options, buildCalls := nodeOptions(t, f, "my script.mjs", "console.log('42')\n")
	runs := &fakeRuns{baselineD: 2_000_000, candD: 1_000_000, candOut: "42"}
	var envs [][]string
	options.runProcess = func(ctx context.Context, exe string, args []string, dir string, env []string) (Sample, error) {
		if isNodeTool(exe) {
			envs = append(envs, env)
		}
		return runs.run(ctx, exe, args, dir, env)
	}
	report, err := RunCandidate(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if report.Environment["baseline_language"] != "nodejs" || report.Environment["node_runtime"] != "Node.js v20.15.1" ||
		report.Environment["python_runtime"] != "" {
		t.Fatalf("Node.js metadata missing or mislabelled: %+v", report.Environment)
	}
	for _, key := range []string{"os", "architecture", "timing_mode"} {
		if report.Environment[key] == "" {
			t.Errorf("metadata %q missing", key)
		}
	}
	if report.Correctness != "PASSED_FOR_CHECKSUM" || report.Verdict != VerdictPort {
		t.Fatalf("unexpected report: %s %s %s", report.Correctness, report.Verdict, report.Reason)
	}
	if report.Measurements[0].Name != "Node.js baseline" || report.Measurements[1].Name != "Rust candidate" {
		t.Fatalf("unexpected names: %+v", report.Measurements)
	}
	for _, m := range report.Measurements {
		if m.Status != "measured" || m.WarmupRuns != 1 || len(m.Samples) != 3 || m.ErrorCount != 0 {
			t.Fatalf("unexpected measurement: %+v", m)
		}
	}
	if len(runs.calls) != 8 {
		t.Fatalf("expected two correctness probes and six timed runs, got %v", runs.calls)
	}
	var syntaxChecked bool
	for _, args := range *buildCalls {
		if len(args) == 2 && args[0] == "--check" && filepath.Base(args[1]) == "baseline.mjs" {
			syntaxChecked = true
		}
	}
	if !syntaxChecked {
		t.Fatalf("syntax must be checked with node --check before timing: %v", *buildCalls)
	}
	// Order: node probe, rust probe, then alternating baseline/Rust, Rust/baseline, baseline/Rust.
	want := []bool{true, false, true, false, false, true, true, false}
	for i, call := range runs.calls {
		if (call == "node") != want[i] {
			t.Fatalf("unexpected trial order at %d: %v", i, runs.calls)
		}
		args := strings.Join(runs.args[i], " ")
		if !strings.Contains(args, "--iterations 100") || !strings.Contains(args, "--input") {
			t.Fatalf("run %d missing the shared contract flags: %v", i, runs.args[i])
		}
		if call == "node" && !strings.HasPrefix(runs.args[i][0], filepath.Join(os.TempDir())) && !strings.Contains(runs.args[i][0], "baseline.mjs") {
			t.Fatalf("Node.js run must execute the temporary copy: %v", runs.args[i])
		}
		if call != "node" && strings.Contains(args, "baseline.mjs") {
			t.Fatalf("Rust candidate received Node.js arguments: %v", runs.args[i])
		}
		if strings.Contains(args, f.input) || strings.Contains(args, "my script.mjs") {
			t.Fatalf("run %d touched original paths: %v", i, runs.args[i])
		}
	}
	if len(envs) == 0 {
		t.Fatal("expected Node.js process runs")
	}
	for _, env := range envs {
		joined := strings.Join(env, "\n")
		if strings.Contains(joined, "NODE_OPTIONS") || !strings.Contains(joined, "NODE_NO_WARNINGS=1") {
			t.Fatalf("unexpected Node.js process environment: %v", env)
		}
	}
	markdown := RenderMarkdown(report)
	if !strings.Contains(markdown, "Rust candidate vs Node.js baseline") || strings.Contains(markdown, "Python baseline") {
		t.Fatalf("Markdown must identify Node.js: %s", markdown)
	}
	if !strings.Contains(report.Reproduction, "--baseline-language nodejs") {
		t.Fatalf("reproduction command must name nodejs: %s", report.Reproduction)
	}
	if err := roundTripReport(report); err != nil {
		t.Fatal(err)
	}
}

func TestRunCandidateNodeAliasAndLanguageValidation(t *testing.T) {
	f := newCandidateFixture(t)
	options, _ := nodeOptions(t, f, "baseline.js", "console.log('42')\n")
	options.BaselineLanguage = "node"
	options.runProcess = (&fakeRuns{baselineD: 1_000_000, candD: 1_000_000, candOut: "42"}).run
	report, err := RunCandidate(context.Background(), options)
	if err != nil || report.Environment["baseline_language"] != "nodejs" {
		t.Fatalf("node alias must normalize to nodejs: %v %+v", err, report.Environment)
	}
	options.BaselineLanguage = "ruby"
	if _, err := RunCandidate(context.Background(), options); err == nil || !strings.Contains(err.Error(), "choose go, python, or nodejs") {
		t.Fatalf("expected clear unsupported language error, got %v", err)
	}
}

func TestRunCandidateNodeRejectsContainerIsolationAndMissingRuntime(t *testing.T) {
	f := newCandidateFixture(t)
	options, _ := nodeOptions(t, f, "baseline.js", "console.log('42')\n")
	options.Isolation = IsolationContainer
	if _, err := RunCandidate(context.Background(), options); err == nil ||
		!strings.Contains(err.Error(), "Node.js baselines are not supported with container isolation") {
		t.Fatalf("expected explicit Node.js container rejection, got %v", err)
	}
	options.Isolation = IsolationProcess
	options.lookPath = func(name string) (string, error) {
		if name == "go" || name == "rustc" {
			return filepath.Join("fake", name), nil
		}
		return "", errors.New("not found")
	}
	if _, err := RunCandidate(context.Background(), options); err == nil || !strings.Contains(err.Error(), "Node.js 18 or newer") {
		t.Fatalf("expected missing Node.js runtime error, got %v", err)
	}
}

func TestRunCandidateNodeRejectsBadRuntimeVersions(t *testing.T) {
	cases := map[string]struct {
		output string
		err    error
		want   string
	}{
		"malformed":   {output: "not a version\n", want: "interpreter reported"},
		"empty":       {output: "", want: "interpreter reported"},
		"unsupported": {output: "v16.20.2\n", want: "Node.js 18 or newer"},
		"exit code":   {output: "boom", err: errors.New("exit status 1"), want: "run Node.js interpreter"},
		"timeout":     {err: context.DeadlineExceeded, want: "timed out"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newCandidateFixture(t)
			options, _ := nodeOptions(t, f, "baseline.js", "console.log('42')\n")
			runs := &fakeRuns{}
			options.runProcess = runs.run
			options.build = func(ctx context.Context, tool string, args []string, _ string, _ []string) (string, error) {
				if tc.err == context.DeadlineExceeded {
					<-ctx.Done()
					return "", ctx.Err()
				}
				return tc.output, tc.err
			}
			if tc.err == context.DeadlineExceeded {
				options.Timeout = 20_000_000
			}
			_, err := RunCandidate(context.Background(), options)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
			if len(runs.calls) != 0 {
				t.Fatalf("no program may run with an invalid runtime: %v", runs.calls)
			}
		})
	}
}

func TestRunCandidateNodeSyntaxFailureBlocksExecution(t *testing.T) {
	f := newCandidateFixture(t)
	options, _ := nodeOptions(t, f, "baseline.js", "console.log(\n")
	runs := &fakeRuns{}
	options.runProcess = runs.run
	options.build = func(_ context.Context, tool string, args []string, _ string, _ []string) (string, error) {
		if len(args) == 1 && args[0] == "--version" {
			return "v20.15.1\n", nil
		}
		if len(args) > 0 && args[0] == "--check" {
			return "/tmp/rustrush/baseline.js:1\nconsole.log(\n            ^\n\nSyntaxError: Unexpected end of input\n    at wrapSafe\n\nNode.js v20.15.1\n", errors.New("exit status 1")
		}
		return "", nil
	}
	report, err := RunCandidate(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	m := report.Measurements[0]
	if m.Name != "Node.js baseline" || m.Status != "build_failed" || m.ErrorCount != 1 ||
		report.Verdict != VerdictInconclusive || len(runs.calls) != 0 {
		t.Fatalf("syntax-invalid baseline must block all program execution: %+v", report)
	}
	if !strings.Contains(m.Error, "SyntaxError: Unexpected end of input") || strings.Contains(m.Error, "console.log(") {
		t.Fatalf("diagnostic must name the syntax error without copying source: %q", m.Error)
	}
	if !strings.Contains(report.Reason, "Node.js baseline was not ready") {
		t.Fatalf("reason must name Node.js: %s", report.Reason)
	}
}

func TestRunCandidateNodeCorrectnessGate(t *testing.T) {
	cases := map[string]struct {
		baselineOut, candidateOut string
	}{
		"mismatch":     {"42", "43"},
		"empty output": {"", ""},
		"invalid json": {"{\"a\":", "{\"a\":1}"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newCandidateFixture(t)
			options, _ := nodeOptions(t, f, "baseline.js", "console.log('42')\n")
			var calls int
			options.runProcess = func(_ context.Context, exe string, _ []string, _ string, _ []string) (Sample, error) {
				calls++
				if isNodeTool(exe) {
					return Sample{DurationNS: 1_000_000, Output: tc.baselineOut}, nil
				}
				return Sample{DurationNS: 1_000_000, Output: tc.candidateOut}, nil
			}
			report, err := RunCandidate(context.Background(), options)
			if err != nil {
				t.Fatal(err)
			}
			if report.Correctness != "FAILED" || report.Verdict != VerdictInconclusive ||
				report.ThroughputDeltaPercent != nil || calls != 2 {
				t.Fatalf("correctness failure must stop before timing: calls=%d %+v", calls, report)
			}
		})
	}
}

func TestRunCandidateNodeFailuresAreInconclusive(t *testing.T) {
	cases := map[string]struct {
		node, rust error
		status     string
		index      int
	}{
		"baseline non-zero exit": {node: errors.New("exit status 1"), status: "failed", index: 0},
		"baseline timeout":       {node: context.DeadlineExceeded, status: "timed_out", index: 0},
		"candidate timeout":      {rust: context.DeadlineExceeded, status: "timed_out", index: 1},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newCandidateFixture(t)
			options, _ := nodeOptions(t, f, "baseline.js", "console.log('42')\n")
			options.runProcess = func(_ context.Context, exe string, _ []string, _ string, _ []string) (Sample, error) {
				if isNodeTool(exe) {
					if tc.node != nil {
						return Sample{}, tc.node
					}
					return Sample{DurationNS: 1_000_000, Output: "42"}, nil
				}
				if tc.rust != nil {
					return Sample{}, tc.rust
				}
				return Sample{DurationNS: 1_000_000, Output: "42"}, nil
			}
			report, err := RunCandidate(context.Background(), options)
			if err != nil {
				t.Fatal(err)
			}
			if report.Verdict != VerdictInconclusive || report.ThroughputDeltaPercent != nil ||
				report.Measurements[tc.index].Status != tc.status || report.Measurements[tc.index].ErrorCount == 0 {
				t.Fatalf("failure must be inconclusive and counted: %+v", report)
			}
		})
	}
}

func TestRunCandidateNodeMissingRustcKeepsBaselineButNoComparison(t *testing.T) {
	f := newCandidateFixture(t)
	options, _ := nodeOptions(t, f, "baseline.js", "console.log('42')\n")
	options.runProcess = (&fakeRuns{baselineD: 1_000_000}).run
	options.lookPath = func(name string) (string, error) {
		if name == "rustc" {
			return "", errors.New("not found")
		}
		return filepath.Join("fake", name), nil
	}
	report, err := RunCandidate(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != VerdictInconclusive || report.ThroughputDeltaPercent != nil ||
		report.Measurements[0].Name != "Node.js baseline" || report.Measurements[1].Status != "unavailable" {
		t.Fatalf("missing rustc must not produce a comparison: %+v", report)
	}
}

func TestCompareReportsRefusesDifferentNodeRuntimesAndLanguages(t *testing.T) {
	env := map[string]string{
		"os": "linux", "architecture": "amd64", "go_runtime": "go1.24.2", "go_compiler": "gc",
		"cpu_count": "8", "go_max_procs": "8", "baseline_language": "nodejs",
		"node_runtime": "Node.js v20.15.1", "rustc": "rustc 1.80.0", "rustc_flags": "release",
	}
	first := Report{
		Project: "a", Component: "same", Workload: "same", Iterations: 5,
		Correctness: "PASSED_FOR_CHECKSUM", Verdict: VerdictPort, Environment: env,
	}
	clone := func(mutate func(map[string]string)) Report {
		copied := make(map[string]string, len(env))
		for key, value := range env {
			copied[key] = value
		}
		mutate(copied)
		report := first
		report.Environment = copied
		return report
	}
	if out := CompareReports(first, clone(func(e map[string]string) { e["node_runtime"] = "Node.js v22.1.0" })); !strings.Contains(out, "Node.js runtime metadata differs") {
		t.Fatalf("different Node.js runtimes must not be compared: %s", out)
	}
	if out := CompareReports(first, clone(func(e map[string]string) { delete(e, "node_runtime") })); !strings.Contains(out, "Node.js runtime metadata differs") {
		t.Fatalf("missing Node.js runtime metadata must not be compared: %s", out)
	}
	if out := CompareReports(first, clone(func(e map[string]string) { e["baseline_language"] = "python" })); !strings.Contains(out, "baseline languages differ") {
		t.Fatalf("different baseline languages must not be compared: %s", out)
	}
}

func TestAnalyzeROIEstimatesFromNodeBaselineReport(t *testing.T) {
	report := roiReport([]uint64{1000, 1000, 1000}, []uint64{500, 500, 500})
	report.Environment["baseline_language"] = "nodejs"
	report.Measurements[0].Name = "Node.js baseline"
	inputs := fullInputs()
	inputs.TargetCPUSecondsPerOperation = nil
	analysis, err := AnalyzeROI(inputs, &report, nil)
	if err != nil {
		t.Fatal(err)
	}
	if analysis.Report == nil || !analysis.Report.MeasuredValuesUsed ||
		!strings.Contains(analysis.Report.ProcessWallClockMsg, "Node.js baseline / Rust candidate") {
		t.Fatalf("Node.js report should link its measured ratio accurately: %+v", analysis.Report)
	}
}

func TestNormalizeReportEvidenceUsesNodeBaseline(t *testing.T) {
	report := roiReport([]uint64{1000, 1000, 1000}, []uint64{500, 500, 500})
	report.Environment["baseline_language"] = "nodejs"
	report.Measurements[0].Name = "Node.js baseline"
	report.Verdict = VerdictPort
	normalizeReportEvidence(&report)
	if report.Verdict == VerdictInconclusive || report.ThroughputDeltaPercent == nil {
		t.Fatalf("Node.js baseline measurement was not located during normalization: %s %s", report.Verdict, report.Reason)
	}
}
