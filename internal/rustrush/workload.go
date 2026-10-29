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
	runProcess  processRunner
}

type processRunner func(context.Context, string, []string, string, []string) (Sample, error)

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
	reproduction := fmt.Sprintf("go run ./cmd/rustrush demo --workload %s --iterations %d --repetitions %d --timeout %s",
		options.Workload, options.Iterations, options.Repetitions, options.Timeout)
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
		Assumptions: map[string]string{
			"timing_scope":  "process startup plus workload execution",
			"latency_scope": "one process wall-clock duration per repetition",
			"warmup_runs":   "1 per implementation",
			"trial_order":   "alternating Go/Rust then Rust/Go, starting with Go",
		},
	}
	if options.Workload == "json" {
		report.Limitations = append(report.Limitations, "The Rust JSON parser supports only the built-in integer-array fixture shape.")
	}
	if options.Workload == "database-fixture" {
		report.Limitations = append(report.Limitations, "This workload is a CSV mock and does not model a database, query planner, connection, or network.")
	}
	rustc, _ := exec.LookPath("rustc")
	if rustc == "" {
		goMeasurement, measureErr := measure(ctx, options, "Go baseline", options.Executable, baselinePrefix)
		redactMeasurementError(&goMeasurement, options.Executable, sourceRoot)
		report.Measurements = append(report.Measurements, goMeasurement)
		if measureErr != nil {
			report.Reason = "The Go baseline failed or timed out; a performance comparison was not made."
			report.Limitations = append(report.Limitations, "Baseline execution failed; no cross-implementation measurements are accepted.")
			report.Measurements = append(report.Measurements, Measurement{
				Name: "Rust candidate", Status: "unavailable", Iterations: options.Iterations,
				Samples: []Sample{}, Error: "rustc was not found on PATH",
			})
			return report, nil
		}
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
	report.Environment["rustc_flags"] = "--edition=2021 -C opt-level=3"
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
		goMeasurement, measureErr := measure(ctx, options, "Go baseline", options.Executable, baselinePrefix)
		redactMeasurementError(&goMeasurement, options.Executable, sourceRoot)
		report.Measurements = append(report.Measurements, goMeasurement)
		buildError := redactBuildOutput(buildOutput.String(), source, tempDir, rustc)
		buildStatus := "build_failed"
		if errors.Is(buildCtx.Err(), context.DeadlineExceeded) {
			buildStatus = "timed_out"
		}
		report.Measurements = append(report.Measurements, Measurement{
			Name: "Rust candidate", Status: buildStatus, Iterations: options.Iterations,
			Samples: []Sample{}, ErrorCount: 1, Error: buildError,
		})
		if report.Measurements[len(report.Measurements)-1].Error == "" {
			report.Measurements[len(report.Measurements)-1].Error = redactPath(err.Error(), rustc, "[rustc]")
		}
		report.Correctness = "NOT_RUN"
		report.Reason = "The Rust candidate did not build; a performance comparison was not made."
		report.Limitations = append(report.Limitations, "Candidate compilation failed; no Rust performance measurements are available.")
		if measureErr != nil {
			report.Limitations = append(report.Limitations, "The Go baseline also failed or timed out.")
		}
		if cleanupErr := cleanup(); cleanupErr != nil {
			return Report{}, cleanupErr
		}
		return report, nil
	}

	goMeasurement, rustMeasurement, correctnessPassed, pairErr := measurePair(
		ctx, options, options.Executable, baselinePrefix, candidate, candidateArgs,
	)
	redactMeasurementError(&goMeasurement, options.Executable, sourceRoot)
	if rustMeasurement.Error != "" {
		rustMeasurement.Error = redactPath(rustMeasurement.Error, candidate, "[candidate]")
		rustMeasurement.Error = redactPath(rustMeasurement.Error, tempDir, "[temporary build directory]")
	}
	report.Measurements = append(report.Measurements, goMeasurement, rustMeasurement)
	if pairErr != nil {
		if correctnessPassed {
			report.Correctness = "PASSED_FOR_CHECKSUM"
		} else {
			report.Correctness = "NOT_RUN"
		}
		report.Reason = "One or both implementations failed or timed out; their performance cannot be compared."
		report.Limitations = append(report.Limitations, "Failed or timed-out execution invalidates the comparison.")
		if cleanupErr := cleanup(); cleanupErr != nil {
			return Report{}, cleanupErr
		}
		return report, nil
	}
	if !correctnessPassed {
		report.Correctness = "FAILED"
		report.Reason = "The correctness probes returned different outputs; timed performance runs were not started."
		report.Limitations = append(report.Limitations, "Correctness mismatch detected before any timed comparison.")
		if cleanupErr := cleanup(); cleanupErr != nil {
			return Report{}, cleanupErr
		}
		return report, nil
	}
	report.Correctness = "PASSED_FOR_CHECKSUM"
	report.Verdict, report.Reason = Decide(report.Correctness, goMeasurement, rustMeasurement)
	if report.Verdict != VerdictInconclusive {
		deltaOps := rustMeasurement.OpsPerSec - goMeasurement.OpsPerSec
		deltaPercent := deltaOps / goMeasurement.OpsPerSec * 100
		report.ThroughputDeltaOpsPerSec = &deltaOps
		report.ThroughputDeltaPercent = &deltaPercent
	}
	report.Limitations = append(report.Limitations,
		"Process startup is included in each sample and can dominate short workloads.",
		"A checksum match does not establish full semantic equivalence.",
		"One synthetic CPU workload is not representative evidence for an application or backend.",
		"Paired trials alternate implementation order to reduce systematic order bias; this does not eliminate machine noise.",
	)
	if err := cleanup(); err != nil {
		return Report{}, err
	}
	return report, nil
}

func redactMeasurementError(measurement *Measurement, executable, sourceRoot string) {
	measurement.Error = redactPath(measurement.Error, executable, "[Go executable]")
	measurement.Error = redactPath(measurement.Error, sourceRoot, "[project]")
	measurement.Error = redactPath(measurement.Error, filepath.ToSlash(sourceRoot), "[project]")
}

func measurePair(
	ctx context.Context,
	options DemoOptions,
	baselineExecutable string,
	baselinePrefix []string,
	candidateExecutable string,
	candidatePrefix []string,
) (Measurement, Measurement, bool, error) {
	baseline := newMeasurement("Go baseline", options)
	candidate := newMeasurement("Rust candidate", options)
	run := func(executable string, prefix []string) (Sample, error) {
		runCtx, cancel := context.WithTimeout(ctx, options.Timeout)
		defer cancel()
		args := append(append([]string(nil), prefix...), "--iterations", strconv.FormatUint(options.Iterations, 10))
		if options.runProcess != nil {
			return options.runProcess(runCtx, executable, args, options.SourceRoot, minimalEnvironment())
		}
		return runProcess(runCtx, executable, args, options.SourceRoot, minimalEnvironment())
	}
	baseline.Status = "measured"
	goWarmup, err := run(baselineExecutable, baselinePrefix)
	if err != nil {
		recordMeasurementFailure(&baseline, err)
		candidate.Status = "not_run"
		return baseline, candidate, false, err
	}
	baseline.WarmupRuns = 1
	candidate.Status = "measured"
	rustWarmup, err := run(candidateExecutable, candidatePrefix)
	if err != nil {
		recordMeasurementFailure(&candidate, err)
		baseline.Status = "incomplete"
		return baseline, candidate, false, err
	}
	candidate.WarmupRuns = 1
	if goWarmup.Output == "" || rustWarmup.Output == "" {
		baseline.Status = "correctness_failed"
		candidate.Status = "correctness_failed"
		candidate.ErrorCount = 1
		candidate.Error = "correctness probe returned empty output"
		return baseline, candidate, false, nil
	}
	if goWarmup.Output != rustWarmup.Output {
		baseline.Status = "correctness_failed"
		candidate.Status = "correctness_failed"
		candidate.ErrorCount = 1
		candidate.Error = "correctness probe output did not match the Go baseline"
		return baseline, candidate, false, nil
	}
	baseline.CorrectnessOutput = goWarmup.Output
	candidate.CorrectnessOutput = rustWarmup.Output
	for i := 0; i < options.Repetitions; i++ {
		order := []struct {
			measurement *Measurement
			executable  string
			prefix      []string
			expected    string
		}{
			{&baseline, baselineExecutable, baselinePrefix, goWarmup.Output},
			{&candidate, candidateExecutable, candidatePrefix, rustWarmup.Output},
		}
		if i%2 == 1 {
			order[0], order[1] = order[1], order[0]
		}
		for _, trial := range order {
			sample, err := run(trial.executable, trial.prefix)
			if err != nil {
				recordMeasurementFailure(trial.measurement, err)
				if baseline.Status == "measured" {
					baseline.Status = "incomplete"
				}
				if candidate.Status == "measured" {
					candidate.Status = "incomplete"
				}
				return baseline, candidate, true, err
			}
			if sample.Output != trial.expected {
				baseline.Samples = nil
				candidate.Samples = nil
				baseline.Status = "correctness_failed"
				candidate.Status = "correctness_failed"
				trial.measurement.ErrorCount = 1
				trial.measurement.Error = fmt.Sprintf("trial %d output differed from its correctness probe", i+1)
				return baseline, candidate, false, nil
			}
			trial.measurement.Samples = append(trial.measurement.Samples, sample)
		}
	}
	if err := summarizeMeasurement(&baseline); err != nil {
		recordMeasurementFailure(&baseline, err)
		candidate.Status = "incomplete"
		return baseline, candidate, true, err
	}
	if err := summarizeMeasurement(&candidate); err != nil {
		recordMeasurementFailure(&candidate, err)
		baseline.Status = "incomplete"
		return baseline, candidate, true, err
	}
	return baseline, candidate, true, nil
}

func newMeasurement(name string, options DemoOptions) Measurement {
	return Measurement{
		Name: name, Status: "not_run", Iterations: options.Iterations,
		Repetitions: options.Repetitions,
		Samples:     make([]Sample, 0, options.Repetitions),
	}
}

func recordMeasurementFailure(measurement *Measurement, err error) {
	measurement.ErrorCount++
	measurement.Error = err.Error()
	if errors.Is(err, context.DeadlineExceeded) {
		measurement.Status = "timed_out"
	} else {
		measurement.Status = "failed"
	}
}

func summarizeMeasurement(measurement *Measurement) error {
	summary, err := Summarize(measurement.Samples, measurement.Iterations)
	if err != nil {
		return err
	}
	for _, sample := range measurement.Samples {
		if sample.TimerResolutionNS > 0 && sample.DurationNS/sample.TimerResolutionNS < 1000 {
			measurement.Warnings = append(measurement.Warnings,
				"sample below the 1,000-tick quality screen; nominal clock quantization may exceed 0.1%, and accuracy is not calibrated")
			break
		}
	}
	measurement.MedianNS = summary.MedianNS
	measurement.P50NS = summary.P50NS
	measurement.P95NS = summary.P95NS
	measurement.P99NS = summary.P99NS
	measurement.OpsPerSec = summary.OpsPerSec
	measurement.CoefficientOfVariation = summary.CoefficientOfVariation
	return nil
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
		if options.runProcess != nil {
			return options.runProcess(runCtx, executable, args, options.SourceRoot, minimalEnvironment())
		}
		return runProcess(runCtx, executable, args, options.SourceRoot, minimalEnvironment())
	}
	result := newMeasurement(name, options)
	result.Status = "measured"
	failed := func(err error) (Measurement, error) {
		recordMeasurementFailure(&result, err)
		return result, fmt.Errorf("%s: %w", name, err)
	}
	warmup, err := run()
	if err != nil {
		return failed(fmt.Errorf("warm-up: %w", err))
	}
	result.WarmupRuns = 1
	if warmup.Output == "" {
		result.Status = "correctness_failed"
		result.ErrorCount = 1
		result.Error = "warm-up returned empty correctness output"
		return result, fmt.Errorf("%s warm-up returned empty correctness output", name)
	}
	result.CorrectnessOutput = warmup.Output
	for i := 0; i < options.Repetitions; i++ {
		sample, err := run()
		if err != nil {
			return failed(fmt.Errorf("trial %d: %w", i+1, err))
		}
		if sample.Output != warmup.Output {
			return failed(fmt.Errorf("trial %d returned a different checksum than warm-up", i+1))
		}
		result.Samples = append(result.Samples, sample)
	}
	if err := summarizeMeasurement(&result); err != nil {
		return failed(err)
	}
	return result, nil
}

func runProcess(ctx context.Context, executable string, args []string, directory string, env []string) (Sample, error) {
	command := exec.CommandContext(ctx, executable, args...)
	command.Env = env
	command.Dir = directory
	start := time.Now()
	output, err := command.Output()
	duration := time.Since(start)
	if err != nil {
		if ctx.Err() != nil {
			return Sample{}, ctx.Err()
		}
		return Sample{}, fmt.Errorf("run workload: %w", err)
	}
	return Sample{DurationNS: uint64(duration.Nanoseconds()), Output: strings.TrimSpace(string(output))}, nil
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
