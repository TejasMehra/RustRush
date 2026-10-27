package rustrush

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type DemoOptions struct {
	Workload    string
	Iterations  uint64
	Repetitions int
	Timeout     time.Duration
	Executable  string
	SourceRoot  string
}

func RunWorkload(iterations uint64) uint64 {
	value := uint64(0x9e3779b97f4a7c15)
	for i := uint64(0); i < iterations; i++ {
		value = value*6364136223846793005 + 1442695040888963407
		value ^= value >> 29
		value *= 0xbf58476d1ce4e5b9
		value ^= value >> 31
	}
	return value
}

func RunDemo(ctx context.Context, options DemoOptions) (Report, error) {
	if options.Iterations == 0 || options.Repetitions < 3 || options.Repetitions > 100 {
		return Report{}, errors.New("invalid demo workload settings")
	}
	if options.Timeout <= 0 {
		return Report{}, errors.New("timeout must be positive")
	}
	if options.Workload == "" {
		options.Workload = "cpu"
	}
	sourceRoot, err := filepath.Abs(options.SourceRoot)
	if err != nil {
		return Report{}, fmt.Errorf("resolve source root: %w", err)
	}
	options.SourceRoot = sourceRoot

	var (
		component      = "deterministic integer-mixing loop"
		workloadText   = "CPU-bound fixed-count uint64 recurrence; both executables receive the same iteration count and emit a checksum"
		baselinePrefix = []string{"__workload"}
		candidateDir   = "cpu-heavy"
		candidateArgs  []string
	)
	switch options.Workload {
	case "cpu":
	case "json":
		component = "JSON integer-array transform"
		workloadText = "Repeatedly decode the shared fixture, double each signed integer in its values array, and encode the result"
		fixture := filepath.Join(sourceRoot, "examples", "json-transform", "fixture.json")
		baselinePrefix = []string{"__json-workload", "--fixture", fixture}
		candidateDir = "json-transform"
		candidateArgs = []string{"--fixture", fixture}
	case "database-fixture":
		component = "mock database filter-and-aggregate"
		workloadText = "Repeatedly parse the shared CSV fixture and sum amounts for active rows; this is a file fixture, not a database"
		fixture := filepath.Join(sourceRoot, "examples", "database-fixture", "records.csv")
		baselinePrefix = []string{"__database-fixture", "--fixture", fixture}
		candidateDir = "database-fixture"
		candidateArgs = []string{"--fixture", fixture}
	default:
		return Report{}, fmt.Errorf("unsupported demo workload %q; choose cpu, json, or database-fixture", options.Workload)
	}
	reproduction := fmt.Sprintf("go run ./cmd/rustrush demo --workload %s --iterations %d --repetitions %d",
		options.Workload, options.Iterations, options.Repetitions)
	report := Report{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   time.Now().UTC(),
		Project:       "RustRush",
		Component:     component,
		Workload:      workloadText,
		Iterations:    options.Iterations,
		Correctness:   "NOT_RUN",
		Verdict:       VerdictInconclusive,
		Environment:   environment(),
		Limitations: []string{
			"CPU utilization, memory, allocation counts, and request-level latency are not measured.",
			"CPU model and background system load are not captured.",
		},
		Reproduction: reproduction,
		Assumptions:  map[string]string{"timing_scope": "process startup plus workload execution", "warmup_runs": "1 per implementation"},
	}
	if options.Workload == "json" {
		report.Limitations = append(report.Limitations, "The Rust JSON parser supports only the built-in integer-array fixture shape.")
	}
	if options.Workload == "database-fixture" {
		report.Limitations = append(report.Limitations, "This workload is a CSV mock and does not model a database, query planner, connection, or network.")
	}
	goMeasurement, err := measure(ctx, options, "Go baseline", options.Executable, baselinePrefix)
	if err != nil {
		return Report{}, err
	}
	report.Measurements = append(report.Measurements, goMeasurement)

	rustc, _ := exec.LookPath("rustc")
	if rustc == "" {
		report.Measurements = append(report.Measurements, Measurement{
			Name: "Rust candidate", Status: "unavailable", Iterations: options.Iterations,
			Samples: []Sample{}, Error: "rustc was not found on PATH",
		})
		report.Limitations = append(report.Limitations, "Rust candidate was not built or run because rustc is unavailable; no cross-language speedup is reported.")
		report.Reason = "The baseline produced real measurements, but a Rust comparison could not be completed. Install Rust and rerun the documented command."
		return report, nil
	}
	versionCtx, cancelVersion := context.WithTimeout(ctx, 3*time.Second)
	versionCommand := exec.CommandContext(versionCtx, rustc, "--version")
	versionCommand.Env = compilerEnvironment()
	version, versionErr := versionCommand.Output()
	cancelVersion()
	if versionErr == nil {
		report.Environment["rustc"] = strings.TrimSpace(string(version))
	} else {
		report.Limitations = append(report.Limitations, "The Rust compiler version could not be recorded.")
	}

	tempDir, err := os.MkdirTemp("", "rustrush-demo-*")
	if err != nil {
		return Report{}, fmt.Errorf("create temporary build directory: %w", err)
	}
	cleanup := func() error {
		if err := os.RemoveAll(tempDir); err != nil {
			return fmt.Errorf("remove temporary build directory: %w", err)
		}
		return nil
	}
	source := filepath.Join(sourceRoot, "examples", candidateDir, "candidate.rs")
	candidate := filepath.Join(tempDir, "candidate")
	if runtime.GOOS == "windows" {
		candidate += ".exe"
	}
	buildCtx, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()
	build := exec.CommandContext(buildCtx, rustc, "--edition=2021", "-C", "opt-level=3", source, "-o", candidate)
	build.Env = compilerEnvironment()
	build.Dir = tempDir
	var buildOutput bytes.Buffer
	build.Stdout = &buildOutput
	build.Stderr = &buildOutput
	if err := build.Run(); err != nil {
		buildError := redactBuildOutput(buildOutput.String(), source, tempDir, rustc)
		report.Measurements = append(report.Measurements, Measurement{
			Name: "Rust candidate", Status: "build_failed", Iterations: options.Iterations,
			Samples: []Sample{}, Error: buildError,
		})
		if report.Measurements[len(report.Measurements)-1].Error == "" {
			report.Measurements[len(report.Measurements)-1].Error = redactPath(err.Error(), rustc, "[rustc]")
		}
		report.Correctness = "NOT_RUN"
		report.Reason = "The Rust candidate did not build; a performance comparison was not made."
		report.Limitations = append(report.Limitations, "Candidate compilation failed; no Rust performance measurements are available.")
		if cleanupErr := cleanup(); cleanupErr != nil {
			return Report{}, cleanupErr
		}
		return report, nil
	}

	rustMeasurement, err := measure(ctx, options, "Rust candidate", candidate, candidateArgs)
	if err != nil {
		failure := redactPath(err.Error(), candidate, "[candidate]")
		failure = redactPath(failure, tempDir, "[temporary build directory]")
		rustMeasurement = Measurement{Name: "Rust candidate", Status: "failed", Iterations: options.Iterations, Samples: []Sample{}, Error: failure}
		report.Measurements = append(report.Measurements, rustMeasurement)
		report.Correctness = "FAILED"
		report.Reason = "The Rust candidate failed to execute reliably; its performance must not be used."
		report.Limitations = append(report.Limitations, "A failed candidate execution invalidates performance comparison.")
		if cleanupErr := cleanup(); cleanupErr != nil {
			return Report{}, cleanupErr
		}
		return report, nil
	}
	report.Measurements = append(report.Measurements, rustMeasurement)
	if !outputsMatch(goMeasurement, rustMeasurement) {
		report.Correctness = "FAILED"
		report.Reason = "The implementations returned different checksums; performance comparison is invalid."
		report.Limitations = append(report.Limitations, "Correctness mismatch detected before interpreting performance.")
		if cleanupErr := cleanup(); cleanupErr != nil {
			return Report{}, cleanupErr
		}
		return report, nil
	}
	report.Correctness = "PASSED_FOR_CHECKSUM"
	report.Verdict, report.Reason = Decide(report.Correctness, goMeasurement, rustMeasurement)
	deltaOps := rustMeasurement.OpsPerSec - goMeasurement.OpsPerSec
	deltaPercent := deltaOps / goMeasurement.OpsPerSec * 100
	report.ThroughputDeltaOpsPerSec = &deltaOps
	report.ThroughputDeltaPercent = &deltaPercent
	report.Limitations = append(report.Limitations,
		"Process startup is included in each sample and can dominate short workloads.",
		"A checksum match does not establish full semantic equivalence.",
		"One synthetic CPU workload is not representative evidence for an application or backend.",
	)
	if err := cleanup(); err != nil {
		return Report{}, err
	}
	return report, nil
}

func outputsMatch(baseline, candidate Measurement) bool {
	if len(baseline.Samples) == 0 || len(candidate.Samples) == 0 {
		return false
	}
	expected := baseline.Samples[0].Output
	for _, sample := range baseline.Samples {
		if sample.Output != expected {
			return false
		}
	}
	for _, sample := range candidate.Samples {
		if sample.Output != expected {
			return false
		}
	}
	return true
}

func redactBuildOutput(output, source, tempDir, rustc string) string {
	result := strings.TrimSpace(output)
	result = redactPath(result, source, "[candidate source]")
	result = redactPath(result, filepath.ToSlash(source), "[candidate source]")
	result = redactPath(result, tempDir, "[temporary build directory]")
	result = redactPath(result, filepath.ToSlash(tempDir), "[temporary build directory]")
	result = redactPath(result, rustc, "[rustc]")
	return result
}

func redactPath(text, path, replacement string) string {
	if path == "" {
		return text
	}
	for {
		index := strings.Index(strings.ToLower(text), strings.ToLower(path))
		if index < 0 {
			return text
		}
		text = text[:index] + replacement + text[index+len(path):]
	}
}

func measure(ctx context.Context, options DemoOptions, name, executable string, prefix []string) (Measurement, error) {
	run := func() (Sample, error) {
		runCtx, cancel := context.WithTimeout(ctx, options.Timeout)
		defer cancel()
		args := append(append([]string(nil), prefix...), "--iterations", strconv.FormatUint(options.Iterations, 10))
		command := exec.CommandContext(runCtx, executable, args...)
		command.Env = minimalEnvironment()
		command.Dir = options.SourceRoot
		start := time.Now()
		output, err := command.Output()
		duration := time.Since(start)
		if err != nil {
			if runCtx.Err() != nil {
				return Sample{}, fmt.Errorf("workload timed out after %s", options.Timeout)
			}
			return Sample{}, fmt.Errorf("run workload: %w", err)
		}
		return Sample{DurationNS: uint64(duration.Nanoseconds()), Output: strings.TrimSpace(string(output))}, nil
	}
	warmup, err := run()
	if err != nil {
		return Measurement{}, fmt.Errorf("%s warm-up: %w", name, err)
	}
	result := Measurement{Name: name, Status: "measured", Iterations: options.Iterations, Repetitions: options.Repetitions}
	for i := 0; i < options.Repetitions; i++ {
		sample, err := run()
		if err != nil {
			return Measurement{}, fmt.Errorf("%s trial %d: %w", name, i+1, err)
		}
		if sample.Output != warmup.Output {
			return Measurement{}, fmt.Errorf("%s trial %d returned a different checksum than warm-up", name, i+1)
		}
		result.Samples = append(result.Samples, sample)
	}
	summary, err := Summarize(result.Samples, options.Iterations)
	if err != nil {
		return Measurement{}, err
	}
	result.MedianNS = summary.MedianNS
	result.P50NS = summary.P50NS
	result.P95NS = summary.P95NS
	result.P99NS = summary.P99NS
	result.OpsPerSec = summary.OpsPerSec
	result.CoefficientOfVariation = summary.CoefficientOfVariation
	return result, nil
}

func minimalEnvironment() []string {
	var env []string
	for _, key := range []string{"PATH", "SystemRoot", "WINDIR", "TEMP", "TMP", "HOME", "USERPROFILE"} {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	return env
}

func compilerEnvironment() []string {
	env := minimalEnvironment()
	for _, key := range []string{"RUSTUP_HOME", "CARGO_HOME", "INCLUDE", "LIB", "LIBPATH", "VCINSTALLDIR", "VCToolsInstallDir", "WindowsSdkDir", "WindowsSDKVersion"} {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	return env
}
