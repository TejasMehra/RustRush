package rustrush

import (
	"context"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const (
	maxCopyFiles = 5000
	maxCopyBytes = 50 << 20
)

// forbiddenRustPatterns rejects constructs that let a single-file candidate
// read other files at compile time or link external code. This is a
// best-effort guard, not a sandbox.
var forbiddenRustPatterns = regexp.MustCompile(`(?m)(\binclude(_str|_bytes)?\s*!|#!?\[\s*(link|link_args|link_name|path)\b|\bextern\s+crate\b|\bmod\s+\w+\s*;)`)

type buildRunner func(ctx context.Context, tool string, args []string, dir string, env []string) (string, error)

type CandidateOptions struct {
	BaselineDir   string
	CandidateFile string
	InputFile     string
	Iterations    uint64
	Repetitions   int
	Timeout       time.Duration
	// AllowExecute is explicit consent to build and run the supplied programs.
	AllowExecute bool
	// Isolation is "process" (default; a timeout and minimal environment, not a
	// sandbox) or "container" (hardened Docker run; fails closed if unavailable).
	Isolation string
	Container ContainerConfig

	runProcess    processRunner
	build         buildRunner
	lookPath      func(string) (string, error)
	containerExec containerExecFn
}

// RunCandidate benchmarks a user-supplied Go main package against a user-supplied
// single-file Rust candidate. Sources are copied; the originals are never modified.
func RunCandidate(ctx context.Context, options CandidateOptions) (Report, error) {
	if !options.AllowExecute {
		return Report{}, errors.New("refusing to build and run supplied programs without --allow-execute; review the code first, since the default process-level isolation is a timeout and minimal environment, not a sandbox (see --isolation container)")
	}
	if options.Iterations == 0 || options.Repetitions < 3 || options.Repetitions > 100 {
		return Report{}, errors.New("iterations must be positive and repetitions must be between 3 and 100")
	}
	if options.Timeout <= 0 {
		return Report{}, errors.New("timeout must be positive")
	}
	baselineDir, err := validateBaselineDir(options.BaselineDir)
	if err != nil {
		return Report{}, err
	}
	candidateSource, err := readCandidateSource(options.CandidateFile)
	if err != nil {
		return Report{}, err
	}
	inputName := ""
	if options.InputFile != "" {
		if err := requireRegularFile(options.InputFile, "input file"); err != nil {
			return Report{}, err
		}
		inputName = filepath.Base(options.InputFile)
	}
	lookPath := options.lookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	build := options.build
	if build == nil {
		build = runBuild
	}
	goTool := "go"
	var engine *containerEngine
	switch options.Isolation {
	case "", IsolationProcess:
		goTool, err = lookPath("go")
		if err != nil {
			return Report{}, errors.New("the go tool was not found on PATH; it is required to build the baseline")
		}
	case IsolationContainer:
		engine, err = newContainerEngine(ctx, options.Container, options.lookPath, options.containerExec)
		if err != nil {
			return Report{}, err
		}
	default:
		return Report{}, fmt.Errorf("unsupported isolation mode %q; choose process or container", options.Isolation)
	}

	tempDir, err := os.MkdirTemp("", "rustrush-candidate-*")
	if err != nil {
		return Report{}, fmt.Errorf("create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)
	baselineCopy := filepath.Join(tempDir, "baseline")
	workDir := filepath.Join(tempDir, "work")
	binDir := filepath.Join(tempDir, "bin")
	candidateCopy := filepath.Join(tempDir, "candidate", "candidate.rs")
	for _, dir := range []string{workDir, binDir, filepath.Dir(candidateCopy)} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return Report{}, fmt.Errorf("create temporary directory: %w", err)
		}
	}
	timerSourceDir := filepath.Join(tempDir, "timer-source")
	if engine != nil && options.runProcess == nil {
		if err := os.MkdirAll(timerSourceDir, 0o700); err != nil {
			return Report{}, fmt.Errorf("create timer source directory: %w", err)
		}
		if err := os.WriteFile(filepath.Join(timerSourceDir, "main.go"), timerSource, 0o600); err != nil {
			return Report{}, fmt.Errorf("write container timer source: %w", err)
		}
	}
	if err := copyTree(baselineDir, baselineCopy); err != nil {
		return Report{}, err
	}
	if err := os.WriteFile(candidateCopy, candidateSource, 0o600); err != nil {
		return Report{}, fmt.Errorf("copy candidate: %w", err)
	}
	var prefix []string
	if inputName != "" {
		inputCopy := filepath.Join(tempDir, "input", inputName)
		if err := os.MkdirAll(filepath.Dir(inputCopy), 0o700); err != nil {
			return Report{}, fmt.Errorf("create temporary directory: %w", err)
		}
		if err := copyFile(options.InputFile, inputCopy); err != nil {
			return Report{}, err
		}
		prefix = []string{"--input", inputCopy}
	}

	suffix := ""
	if runtime.GOOS == "windows" && engine == nil {
		suffix = ".exe"
	}
	baselineExe := filepath.Join(binDir, "baseline"+suffix)
	candidateExe := filepath.Join(binDir, "candidate"+suffix)
	redact := func(text string) string {
		for _, pair := range [][2]string{
			{tempDir, "[temporary directory]"}, {filepath.ToSlash(tempDir), "[temporary directory]"},
			{goTool, "[go]"},
		} {
			if pair[0] == "go" {
				continue
			}
			text = redactPath(text, pair[0], pair[1])
		}
		return strings.TrimSpace(text)
	}

	report := Report{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   time.Now().UTC(),
		Project:       filepath.Base(baselineDir),
		Component:     "user-supplied operation (Go main package vs. single-file Rust candidate)",
		Workload: fmt.Sprintf("Both programs run with --iterations %d%s; stdout must be identical before timed repetitions",
			options.Iterations, map[bool]string{true: " and the same --input file", false: ""}[inputName != ""]),
		Iterations:  options.Iterations,
		Correctness: "NOT_RUN",
		Verdict:     VerdictInconclusive,
		Environment: environment(),
		Limitations: []string{
			"Supplied programs are executed with a timeout, a minimal environment, and no shell; this is not a sandbox. Review code before running it.",
			"CPU utilization, memory, allocation counts, and request-level latency are not measured.",
			"CPU model and background system load are not captured.",
			"Equivalence is judged only by identical stdout for the supplied inputs; it does not prove semantic equivalence.",
			"Process startup and output capture are included in each sample and can dominate short workloads.",
		},
		Reproduction: fmt.Sprintf("rustrush candidate --baseline <go-main-dir> --candidate <candidate.rs>%s --iterations %d --repetitions %d --timeout %s --allow-execute",
			map[bool]string{true: " --input <file>", false: ""}[inputName != ""], options.Iterations, options.Repetitions, options.Timeout),
		Assumptions: map[string]string{
			"timing_scope":  "process startup plus workload execution",
			"latency_scope": "one process wall-clock duration per repetition",
			"warmup_runs":   "1 per implementation",
			"trial_order":   "alternating Go/Rust then Rust/Go, starting with Go",
			"go_build":      "GOFLAGS=-mod=readonly, GOPROXY=off, GOTOOLCHAIN=local, built from a temporary copy",
			"isolation":     "process-level isolation: source copies, empty working directory, minimal environment, per-run timeout; not a sandbox",
		},
	}
	runOptions := DemoOptions{
		Iterations: options.Iterations, Repetitions: options.Repetitions, Timeout: options.Timeout,
		SourceRoot: workDir, runProcess: options.runProcess,
	}
	inputDir := ""
	if inputName != "" {
		inputDir = filepath.Join(tempDir, "input")
	}
	if engine != nil {
		for key, value := range engine.metadata() {
			report.Environment[key] = value
		}
		report.Assumptions["isolation"] = "container (docker): " + report.Environment["container_limits"] + ", no network, read-only root filesystem, read-only source/binary mounts, all capabilities dropped, no-new-privileges, non-root user, empty environment; images are never pulled implicitly"
		report.Assumptions["go_build"] = "GOFLAGS=-mod=readonly, GOPROXY=off, GOTOOLCHAIN=local, built inside the Go container from a read-only copy"
		var kept []string
		for _, limitation := range report.Limitations {
			if !strings.HasPrefix(limitation, "Process startup and output capture are included") {
				kept = append(kept, limitation)
			}
		}
		report.Limitations = append(kept,
			"Container isolation shares the host kernel and depends on the container runtime; it has not been independently validated as a security boundary. Do not run untrusted code on a machine you cannot afford to compromise.",
			"Each implementation's first warm-up also serves as its correctness probe; that run's duration is discarded, and timed repetitions start only after outputs match. Every warm-up/correctness run and measured invocation uses its own hardened docker run --rm container. The Go time.Since monotonic timer wraps the workload child; child startup and stdout capture are included, while timer-helper startup, container lifecycle, compilation, and Docker dispatch are excluded. Full invocation wall time is measured; the residual overhead is a derived estimate because clocks are not cross-calibrated. Clock resolution is an empirical minimum tick, not an accuracy guarantee; instrumentation accuracy is not calibrated.",
			"Each invocation has a fresh container filesystem, writable /tmp tmpfs, working directory, HOME, and TMPDIR, so process-created files and caches do not persist into later repetitions. Host page caches, kernel state, and external machine load are not reset.",
			"The baseline runs in the Go image and the candidate in the Rust image; differences between images and their system libraries are part of the measurement. This timing mode is not comparable with process_end_to_end reports.")
		report.Environment["timing_mode"] = TimingContainerRun
		report.Assumptions["timing_scope"] = "fresh-container workload process duration from Go's monotonic clock; excludes timer helper startup, container lifecycle, compilation, and Docker dispatch"
		report.Assumptions["latency_scope"] = "one fresh-container process duration per implementation warm-up/correctness run (discarded) and each measured repetition"
		build = func(ctx context.Context, tool string, args []string, dir string, _ []string) (string, error) {
			if tool == "go" {
				return engine.buildGo(ctx, dir, binDir)
			}
			return engine.buildRust(ctx, dir, binDir)
		}
		lookPath = func(name string) (string, error) { return name, nil }
	} else {
		report.Environment["isolation"] = "process (timeout and minimal environment only; not a sandbox)"
		report.Environment["timing_mode"] = TimingProcessEndToEnd
	}
	notRun := func(name, status, message string, count int) Measurement {
		return Measurement{Name: name, Status: status, Iterations: options.Iterations, Samples: []Sample{}, ErrorCount: count, Error: message}
	}

	output, err, buildTimedOut := runBuildPhase(ctx, options.Timeout, func(buildCtx context.Context) (string, error) {
		return build(buildCtx, goTool, []string{"build", "-o", baselineExe, "."}, baselineCopy, goBuildEnvironment())
	})
	if err != nil {
		status := "build_failed"
		if buildTimedOut {
			status = "timed_out"
		}
		report.Measurements = []Measurement{
			notRun("Go baseline", status, buildMessage(redact(output), redact(err.Error())), 1),
			notRun("Rust candidate", "not_run", "", 0),
		}
		report.Reason = "The Go baseline did not build; a performance comparison was not made."
		report.Limitations = append(report.Limitations, "Baseline compilation failed; no measurements are available.")
		return report, nil
	}
	if engine != nil && options.runProcess == nil {
		output, err, buildTimedOut = runBuildPhase(ctx, options.Timeout, func(buildCtx context.Context) (string, error) {
			return engine.buildTimer(buildCtx, timerSourceDir, binDir)
		})
		if err != nil {
			message := redact(buildFailureMessage(output, err))
			status := "failed"
			if buildTimedOut {
				status = "timed_out"
			}
			report.Measurements = []Measurement{
				notRun("Go baseline", status, message, 1),
				notRun("Rust candidate", "not_run", "", 0),
			}
			report.Reason = "The monotonic container timer could not be prepared; no performance measurements were collected. Preparation error: " + message
			report.Limitations = append(report.Limitations, "The benchmark timer could not be prepared; no comparison is valid.")
			return report, nil
		}
	}

	rustc, lookErr := lookPath("rustc")
	if lookErr != nil || rustc == "" {
		baseline, measureErr := measure(ctx, runOptions, "Go baseline", baselineExe, prefix)
		baseline.Error = redact(baseline.Error)
		report.Measurements = []Measurement{baseline, notRun("Rust candidate", "unavailable", "rustc was not found on PATH", 0)}
		report.Limitations = append(report.Limitations, "Rust candidate was not built or run because rustc is unavailable; no cross-language comparison is reported.")
		report.Reason = "The baseline produced measurements, but the Rust comparison could not be completed. Install Rust and rerun."
		if measureErr != nil {
			report.Limitations = append(report.Limitations, "The Go baseline failed or timed out.")
		}
		return report, nil
	}
	report.Environment["rustc_flags"] = "--edition=2021 -C opt-level=3"
	if engine != nil {
		versionCtx, cancelVersion := context.WithTimeout(ctx, options.Timeout)
		report.Environment["rustc"] = engine.rustVersion(versionCtx)
		cancelVersion()
	} else if options.build == nil {
		if version, err := exec.CommandContext(ctx, rustc, "--version").Output(); err == nil {
			report.Environment["rustc"] = strings.TrimSpace(string(version))
		}
	}
	output, err, buildTimedOut = runBuildPhase(ctx, options.Timeout, func(buildCtx context.Context) (string, error) {
		return build(buildCtx, rustc, []string{"--edition=2021", "-C", "opt-level=3", candidateCopy, "-o", candidateExe}, filepath.Dir(candidateCopy), compilerEnvironment())
	})
	if err != nil {
		status := "build_failed"
		if buildTimedOut {
			status = "timed_out"
		}
		report.Measurements = []Measurement{
			notRun("Go baseline", "not_run", "", 0),
			notRun("Rust candidate", status, buildMessage(redact(strings.ReplaceAll(output, candidateCopy, "[candidate source]")), redact(err.Error())), 1),
		}
		report.Reason = "The Rust candidate did not build; a performance comparison was not made."
		report.Limitations = append(report.Limitations, "Candidate compilation failed; no measurements are available.")
		return report, nil
	}

	var containerRunner *containerRunner
	if engine != nil && options.runProcess == nil {
		containerRunner = engine.runner(binDir, inputDir)
		runOptions.runProcess = containerRunner.processRunner()
	}
	baseline, candidate, correctnessPassed, pairErr := measurePair(ctx, runOptions, baselineExe, prefix, candidateExe, prefix)
	if containerRunner != nil {
		for key, value := range containerRunner.metadata() {
			report.Environment[key] = value
		}
	}
	baseline.Error, candidate.Error = redact(baseline.Error), redact(candidate.Error)
	report.Measurements = []Measurement{baseline, candidate}
	switch {
	case pairErr != nil:
		if correctnessPassed {
			report.Correctness = "PASSED_FOR_CHECKSUM"
		}
		report.Reason = "One or both implementations failed or timed out; their performance cannot be compared."
		report.Limitations = append(report.Limitations, "Failed or timed-out execution invalidates the comparison.")
	case !correctnessPassed:
		report.Correctness = "FAILED"
		report.Reason = "The correctness probes returned different or empty outputs; timed performance runs were not started."
		report.Limitations = append(report.Limitations, "Correctness mismatch detected before any timed comparison.")
	default:
		report.Correctness = "PASSED_FOR_CHECKSUM"
		report.Verdict, report.Reason = Decide(report.Correctness, baseline, candidate)
		if report.Verdict != VerdictInconclusive {
			deltaOps := candidate.OpsPerSec - baseline.OpsPerSec
			deltaPercent := deltaOps / baseline.OpsPerSec * 100
			report.ThroughputDeltaOpsPerSec = &deltaOps
			report.ThroughputDeltaPercent = &deltaPercent
		}
		report.Limitations = append(report.Limitations, "Paired trials alternate implementation order to reduce systematic order bias; this does not eliminate machine noise.")
	}
	return report, nil
}

func runBuildPhase(ctx context.Context, timeout time.Duration, build func(context.Context) (string, error)) (string, error, bool) {
	buildCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	output, err := build(buildCtx)
	return output, err, errors.Is(buildCtx.Err(), context.DeadlineExceeded)
}

func buildMessage(output, fallback string) string {
	if output != "" {
		return output
	}
	return fallback
}

func buildFailureMessage(output string, err error) string {
	message := strings.TrimSpace(output)
	if err == nil || strings.Contains(message, err.Error()) {
		return message
	}
	if message != "" {
		message += "\n"
	}
	return message + err.Error()
}

func goBuildEnvironment() []string {
	env := compilerEnvironment()
	for _, key := range []string{"LOCALAPPDATA", "APPDATA", "GOCACHE", "GOPATH", "GOMODCACHE", "GOROOT"} {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	return append(env, "GOFLAGS=-mod=readonly", "GOPROXY=off", "GOTOOLCHAIN=local", "CGO_ENABLED=0", "GOWORK=off")
}

func runBuild(ctx context.Context, tool string, args []string, dir string, env []string) (string, error) {
	command := exec.CommandContext(ctx, tool, args...)
	command.Dir = dir
	command.Env = env
	output, err := command.CombinedOutput()
	return string(output), err
}

func requireRegularFile(path, label string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s %q must be a regular file (not a directory or symlink)", label, filepath.Base(path))
	}
	if info.Size() > maxSourceFileBytes {
		return fmt.Errorf("%s %q exceeds the %d byte limit", label, filepath.Base(path), maxSourceFileBytes)
	}
	return nil
}

func readCandidateSource(path string) ([]byte, error) {
	if path == "" {
		return nil, errors.New("a Rust candidate file is required (--candidate)")
	}
	if strings.ToLower(filepath.Ext(path)) != ".rs" {
		return nil, fmt.Errorf("candidate %q must be a single .rs file", filepath.Base(path))
	}
	if err := requireRegularFile(path, "candidate"); err != nil {
		return nil, err
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read candidate: %w", err)
	}
	if match := forbiddenRustPatterns.Find(contents); match != nil {
		return nil, fmt.Errorf("candidate uses %q, which is not allowed: candidates must be self-contained single files without include!, extern crate, external modules, or linker attributes", strings.TrimSpace(string(match)))
	}
	return contents, nil
}

func validateBaselineDir(path string) (string, error) {
	if path == "" {
		return "", errors.New("a Go baseline directory is required (--baseline)")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve baseline: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("baseline: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("baseline %q must be a directory containing a Go main package", filepath.Base(path))
	}
	root, err := findModuleRoot(absolute)
	if err != nil {
		return "", err
	}
	if root != absolute {
		return "", fmt.Errorf("baseline %q must be the module root containing go.mod", filepath.Base(path))
	}
	entries, err := os.ReadDir(absolute)
	if err != nil {
		return "", fmt.Errorf("read baseline: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(absolute, name), nil, parser.PackageClauseOnly)
		if err == nil && file.Name.Name == "main" {
			return absolute, nil
		}
	}
	return "", errors.New("baseline directory has no Go main package at its top level; add a small main package that accepts --iterations")
}

func findModuleRoot(dir string) (string, error) {
	if info, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil || info.IsDir() {
		return "", errors.New("baseline must be a Go module root containing go.mod")
	}
	return dir, nil
}

// copyTree copies regular files, skipping symlinks, dependency/build
// directories, and credential-like names, with file-count and size caps.
func copyTree(source, destination string) error {
	files, total := 0, int64(0)
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != source && (skippedDirectory(entry.Name()) || sensitiveFilename(entry.Name())) {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(destination, rel), 0o700)
		}
		if !entry.Type().IsRegular() || sensitiveFilename(entry.Name()) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		files++
		total += info.Size()
		if files > maxCopyFiles || total > maxCopyBytes {
			return fmt.Errorf("baseline is too large to copy (limit %d files / %d MiB); point --baseline at a small benchmark module", maxCopyFiles, maxCopyBytes>>20)
		}
		return copyFile(path, filepath.Join(destination, rel))
	})
	if err != nil {
		return fmt.Errorf("copy baseline: %w", err)
	}
	return nil
}

func copyFile(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
