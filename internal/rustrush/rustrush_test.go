package rustrush

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunWorkloadIsDeterministic(t *testing.T) {
	first := RunWorkload(100_000)
	second := RunWorkload(100_000)
	if first != second {
		t.Fatalf("same workload returned different checksums: %d != %d", first, second)
	}
}

func TestTransformJSONDoublesSignedIntegers(t *testing.T) {
	output, err := TransformJSON([]byte(`{"values":[0,1,7,42,-5]}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != `{"values":[0,2,14,84,-10]}` {
		t.Fatalf("unexpected transformed JSON: %s", output)
	}
}

func TestTransformJSONHandlesEmptyArraysAndIntegerOverflow(t *testing.T) {
	empty, err := TransformJSON([]byte(`{"values":[]}`))
	if err != nil || string(empty) != `{"values":[]}` {
		t.Fatalf("empty array transform = %s, %v", empty, err)
	}
	wrapped, err := TransformJSON([]byte(`{"values":[9223372036854775807]}`))
	if err != nil || string(wrapped) != `{"values":[-2]}` {
		t.Fatalf("overflow transform = %s, %v", wrapped, err)
	}
}

func TestTransformJSONRejectsUnsupportedFixtureShapes(t *testing.T) {
	for _, input := range []string{
		`{"values":[1],"extra":true}`,
		`{"values":[1]} {}`,
		`{"values":[1.5]}`,
		`{"values":[null]}`,
		`{"values":null}`,
		`{}`,
	} {
		if _, err := TransformJSON([]byte(input)); err == nil {
			t.Fatalf("unsupported fixture was accepted: %s", input)
		}
	}
}

func TestRunJSONWorkloadUsesFixtureRepeatedly(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "fixture.json")
	if err := os.WriteFile(fixture, []byte(`{"values":[2,-3]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := RunJSONWorkload(fixture, 4)
	if err != nil {
		t.Fatal(err)
	}
	if output != `{"values":[4,-6]}` {
		t.Fatalf("unexpected workload output: %s", output)
	}
}

func TestBuiltInJSONFixtureHasExpectedTransformation(t *testing.T) {
	fixture := filepath.Join("..", "..", "examples", "json-transform", "fixture.json")
	output, err := RunJSONWorkload(fixture, 1)
	if err != nil {
		t.Fatal(err)
	}
	if output != `{"values":[0,2,14,84,-10]}` {
		t.Fatalf("unexpected built-in fixture output: %s", output)
	}
}

func TestRunDatabaseFixtureAggregatesActiveRows(t *testing.T) {
	fixture := filepath.Join("..", "..", "examples", "database-fixture", "records.csv")
	output, err := RunDatabaseFixture(fixture, 5)
	if err != nil {
		t.Fatal(err)
	}
	if output != "42" {
		t.Fatalf("unexpected fixture aggregate: %s", output)
	}
}

func TestRunDatabaseFixtureRejectsInvalidRows(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "records.csv")
	if err := os.WriteFile(fixture, []byte("id,active,amount\n1,maybe,4\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RunDatabaseFixture(fixture, 1); err == nil {
		t.Fatal("invalid active field was accepted")
	}
}

func TestSummarizeUsesNearestRankAndMedianThroughput(t *testing.T) {
	samples := []Sample{{DurationNS: 100}, {DurationNS: 300}, {DurationNS: 200}, {DurationNS: 400}, {DurationNS: 500}}
	summary, err := Summarize(samples, 10)
	if err != nil {
		t.Fatal(err)
	}
	if summary.MedianNS != 300 || summary.P95NS != 500 || summary.P99NS != 500 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if summary.OpsPerSec < 33_333_333 || summary.OpsPerSec > 33_333_334 {
		t.Fatalf("unexpected throughput: %f", summary.OpsPerSec)
	}
}

func TestSummarizeRejectsMissingAndInvalidSamples(t *testing.T) {
	if _, err := Summarize(nil, 1); err == nil {
		t.Fatal("expected empty samples to be rejected")
	}
	if _, err := Summarize([]Sample{{DurationNS: 0}}, 1); err == nil {
		t.Fatal("expected zero duration to be rejected")
	}
}

func TestMeasureAggregatesWarmupsTrialsThroughputAndLatencySamples(t *testing.T) {
	durations := []uint64{900, 100, 300, 200}
	calls := 0
	options := DemoOptions{
		Iterations: 10, Repetitions: 3, Timeout: time.Second,
		runProcess: func(context.Context, string, []string, string, []string) (Sample, error) {
			sample := Sample{DurationNS: durations[calls], Output: "stable-checksum"}
			calls++
			return sample, nil
		},
	}
	measurement, err := measure(context.Background(), options, "Go baseline", "ignored", nil)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 4 || measurement.WarmupRuns != 1 || measurement.Repetitions != 3 ||
		measurement.ErrorCount != 0 || len(measurement.Samples) != 3 {
		t.Fatalf("warm-up/trial accounting is incorrect: calls=%d measurement=%+v", calls, measurement)
	}
	if measurement.MedianNS != 200 || measurement.P50NS != 200 ||
		measurement.P95NS != 300 || measurement.P99NS != 300 ||
		measurement.OpsPerSec != 50_000_000 {
		t.Fatalf("unexpected aggregated benchmark values: %+v", measurement)
	}
}

func TestMeasureRecordsFailedCandidateAsInvalidEvidence(t *testing.T) {
	options := DemoOptions{
		Iterations: 10, Repetitions: 3, Timeout: time.Second,
		runProcess: func(context.Context, string, []string, string, []string) (Sample, error) {
			return Sample{}, errors.New("candidate exited unsuccessfully")
		},
	}
	candidate, err := measure(context.Background(), options, "Rust candidate", "ignored", nil)
	if err == nil || candidate.Status != "failed" || candidate.ErrorCount != 1 ||
		candidate.WarmupRuns != 0 || len(candidate.Samples) != 0 {
		t.Fatalf("failed candidate evidence was not retained: measurement=%+v error=%v", candidate, err)
	}
	verdict, _ := Decide("PASSED_FOR_CHECKSUM", testMeasurement("Go baseline", []uint64{10, 11, 12}, 10), candidate)
	if verdict != VerdictInconclusive {
		t.Fatalf("failed candidate received a performance verdict: %s", verdict)
	}
}

func TestMeasureRecordsTimeoutAsInconclusiveEvidence(t *testing.T) {
	options := DemoOptions{
		Iterations: 10, Repetitions: 3, Timeout: time.Nanosecond,
		runProcess: func(ctx context.Context, _ string, _ []string, _ string, _ []string) (Sample, error) {
			return Sample{}, context.DeadlineExceeded
		},
	}
	candidate, err := measure(context.Background(), options, "Rust candidate", "ignored", nil)
	if err == nil || candidate.Status != "timed_out" || candidate.ErrorCount != 1 ||
		candidate.WarmupRuns != 0 ||
		!strings.Contains(candidate.Error, "deadline exceeded") {
		t.Fatalf("timed-out candidate was not reported accurately: measurement=%+v error=%v", candidate, err)
	}
	verdict, _ := Decide("PASSED_FOR_CHECKSUM", testMeasurement("Go baseline", []uint64{10, 11, 12}, 10), candidate)
	if verdict != VerdictInconclusive {
		t.Fatalf("timed-out candidate received a performance verdict: %s", verdict)
	}
}

func TestMeasurePairAlternatesTrialOrderAfterCorrectnessGate(t *testing.T) {
	var calls []string
	durations := []uint64{900, 800, 300, 400, 500, 600, 700, 200}
	options := DemoOptions{
		Iterations: 10, Repetitions: 3, Timeout: time.Second,
		runProcess: func(_ context.Context, executable string, _ []string, _ string, _ []string) (Sample, error) {
			calls = append(calls, executable)
			return Sample{DurationNS: durations[len(calls)-1], Output: "same-checksum"}, nil
		},
	}
	baseline, candidate, correctnessPassed, err := measurePair(
		context.Background(), options, "go", nil, "rust", nil,
	)
	if err != nil || !correctnessPassed {
		t.Fatalf("valid paired benchmark failed: passed=%v error=%v", correctnessPassed, err)
	}
	expectedOrder := []string{"go", "rust", "go", "rust", "rust", "go", "go", "rust"}
	if strings.Join(calls, ",") != strings.Join(expectedOrder, ",") {
		t.Fatalf("paired trial order = %v; want %v", calls, expectedOrder)
	}
	if baseline.WarmupRuns != 1 || candidate.WarmupRuns != 1 ||
		len(baseline.Samples) != 3 || len(candidate.Samples) != 3 {
		t.Fatalf("paired measurement metadata is incomplete: Go=%+v Rust=%+v", baseline, candidate)
	}
}

func TestMeasurePairStopsBeforeTimingWhenCorrectnessProbeDiffers(t *testing.T) {
	calls := 0
	options := DemoOptions{
		Iterations: 10, Repetitions: 3, Timeout: time.Second,
		runProcess: func(_ context.Context, executable string, _ []string, _ string, _ []string) (Sample, error) {
			calls++
			output := "baseline-checksum"
			if executable == "rust" {
				output = "different-checksum"
			}
			return Sample{DurationNS: 100, Output: output}, nil
		},
	}
	baseline, candidate, correctnessPassed, err := measurePair(
		context.Background(), options, "go", nil, "rust", nil,
	)
	if err != nil || correctnessPassed || calls != 2 {
		t.Fatalf("benchmark ran despite correctness mismatch: passed=%v calls=%d error=%v", correctnessPassed, calls, err)
	}
	if baseline.Status != "correctness_failed" || candidate.Status != "correctness_failed" ||
		len(baseline.Samples) != 0 || len(candidate.Samples) != 0 {
		t.Fatalf("mismatched probes retained performance evidence: Go=%+v Rust=%+v", baseline, candidate)
	}
	if verdict, _ := Decide("FAILED", baseline, candidate); verdict != VerdictInconclusive {
		t.Fatalf("incorrect candidate received a performance verdict: %s", verdict)
	}
}

func TestMeasurePairRejectsEmptyCorrectnessOutput(t *testing.T) {
	calls := 0
	options := DemoOptions{
		Iterations: 10, Repetitions: 3, Timeout: time.Second,
		runProcess: func(context.Context, string, []string, string, []string) (Sample, error) {
			calls++
			return Sample{DurationNS: 100}, nil
		},
	}
	baseline, candidate, correctnessPassed, err := measurePair(
		context.Background(), options, "go", nil, "rust", nil,
	)
	if err != nil || correctnessPassed || calls != 2 ||
		baseline.Status != "correctness_failed" || candidate.Status != "correctness_failed" ||
		candidate.ErrorCount != 1 {
		t.Fatalf("empty output was treated as a valid checksum: pass=%v calls=%d Go=%+v Rust=%+v error=%v",
			correctnessPassed, calls, baseline, candidate, err)
	}
}

func TestMeasurePairTimeoutInvalidatesBothAggregateResults(t *testing.T) {
	calls := 0
	options := DemoOptions{
		Iterations: 10, Repetitions: 3, Timeout: time.Nanosecond,
		runProcess: func(_ context.Context, executable string, _ []string, _ string, _ []string) (Sample, error) {
			calls++
			if calls == 4 {
				return Sample{}, context.DeadlineExceeded
			}
			return Sample{DurationNS: 100, Output: "same-checksum"}, nil
		},
	}
	baseline, candidate, correctnessPassed, err := measurePair(
		context.Background(), options, "go", nil, "rust", nil,
	)
	if err == nil || !correctnessPassed || candidate.Status != "timed_out" ||
		baseline.Status != "incomplete" || candidate.ErrorCount != 1 {
		t.Fatalf("timeout was not reflected in paired results: pass=%v Go=%+v Rust=%+v error=%v",
			correctnessPassed, baseline, candidate, err)
	}
	if baseline.MedianNS != 0 || candidate.MedianNS != 0 {
		t.Fatalf("partial runs produced aggregate metrics: Go=%+v Rust=%+v", baseline, candidate)
	}
	if verdict, _ := Decide("PASSED_FOR_CHECKSUM", baseline, candidate); verdict != VerdictInconclusive {
		t.Fatalf("timed-out pair received a verdict: %s", verdict)
	}
}

func TestMeasureRejectsChangedWarmupOutput(t *testing.T) {
	calls := 0
	options := DemoOptions{
		Iterations: 10, Repetitions: 3, Timeout: time.Second,
		runProcess: func(context.Context, string, []string, string, []string) (Sample, error) {
			calls++
			return Sample{DurationNS: 100, Output: []string{"warmup", "changed"}[min(calls-1, 1)]}, nil
		},
	}
	measurement, err := measure(context.Background(), options, "Rust candidate", "ignored", nil)
	if err == nil || measurement.Status != "failed" || measurement.ErrorCount != 1 || len(measurement.Samples) != 0 {
		t.Fatalf("output mismatch was accepted as benchmark data: measurement=%+v error=%v", measurement, err)
	}
}

func TestMeasureRejectsEmptyCorrectnessOutput(t *testing.T) {
	options := DemoOptions{
		Iterations: 10, Repetitions: 3, Timeout: time.Second,
		runProcess: func(context.Context, string, []string, string, []string) (Sample, error) {
			return Sample{DurationNS: 100}, nil
		},
	}
	measurement, err := measure(context.Background(), options, "Go baseline", "ignored", nil)
	if err == nil || measurement.Status != "correctness_failed" ||
		measurement.ErrorCount != 1 || measurement.WarmupRuns != 1 {
		t.Fatalf("empty baseline correctness output was accepted: measurement=%+v error=%v", measurement, err)
	}
}

func TestDecideRequiresCorrectnessAndReliableEvidence(t *testing.T) {
	baseline := testMeasurement("Go baseline", []uint64{100_000_000, 101_000_000, 99_000_000}, 10)
	candidate := testMeasurement("Rust candidate", []uint64{80_000_000, 81_000_000, 79_000_000}, 10)
	verdict, _ := Decide("NOT_RUN", baseline, candidate)
	if verdict != VerdictInconclusive {
		t.Fatalf("incorrectness did not invalidate verdict: %s", verdict)
	}
	verdict, _ = Decide("PASSED_FOR_CHECKSUM", baseline, candidate)
	if verdict != VerdictPort {
		t.Fatalf("expected PORT for reliable gain, got %s", verdict)
	}
	candidate = testMeasurement("Rust candidate", []uint64{120_000_000, 121_000_000, 119_000_000}, 10)
	verdict, _ = Decide("PASSED_FOR_CHECKSUM", baseline, candidate)
	if verdict != VerdictDontPort {
		t.Fatalf("expected DONT_PORT for reliable regression, got %s", verdict)
	}
	candidate = testMeasurement("Rust candidate", []uint64{99_000_000, 100_000_000, 101_000_000}, 10)
	verdict, _ = Decide("PASSED_FOR_CHECKSUM", baseline, candidate)
	if verdict != VerdictOptimizeFirst {
		t.Fatalf("expected OPTIMIZE_FIRST for marginal gain, got %s", verdict)
	}
	candidate = testMeasurement("Rust candidate", []uint64{20_000_000, 100_000_000, 300_000_000}, 10)
	verdict, _ = Decide("PASSED_FOR_CHECKSUM", baseline, candidate)
	if verdict != VerdictInconclusive {
		t.Fatalf("noisy measurements did not produce an inconclusive verdict: %s", verdict)
	}
}

func TestDecideRejectsInvalidOrIncompleteMeasurements(t *testing.T) {
	baseline := testMeasurement("Go baseline", []uint64{100_000_000, 101_000_000, 99_000_000}, 10)
	candidate := testMeasurement("Rust candidate", []uint64{80_000_000, 81_000_000, 79_000_000}, 10)
	candidate.Samples = candidate.Samples[:2]
	if verdict, _ := Decide("PASSED_FOR_CHECKSUM", baseline, candidate); verdict != VerdictInconclusive {
		t.Fatalf("incomplete trial data received a verdict: %s", verdict)
	}
	candidate = testMeasurement("Rust candidate", []uint64{80_000_000, 81_000_000, 79_000_000}, 10)
	candidate.ErrorCount = 1
	if verdict, _ := Decide("PASSED_FOR_CHECKSUM", baseline, candidate); verdict != VerdictInconclusive {
		t.Fatalf("measurement errors were accepted: %s", verdict)
	}
	candidate = testMeasurement("Rust candidate", []uint64{80_000_000, 81_000_000, 79_000_000}, 10)
	candidate.P95NS++
	if verdict, _ := Decide("PASSED_FOR_CHECKSUM", baseline, candidate); verdict != VerdictInconclusive {
		t.Fatalf("inconsistent summary values were accepted: %s", verdict)
	}
	candidate = testMeasurement("Rust candidate", []uint64{80_000_000, 81_000_000, 79_000_000}, 10)
	candidate.CorrectnessOutput = "different"
	for i := range candidate.Samples {
		candidate.Samples[i].Output = "different"
	}
	if verdict, _ := Decide("PASSED_FOR_CHECKSUM", baseline, candidate); verdict != VerdictInconclusive {
		t.Fatalf("different recorded correctness outputs were accepted: %s", verdict)
	}
}

func TestIncorrectCandidateOutputCannotPassCorrectnessGate(t *testing.T) {
	baseline := testMeasurement("Go baseline", []uint64{100_000_000, 101_000_000, 99_000_000}, 10)
	incorrect := testMeasurement("Rust candidate", []uint64{80_000_000, 81_000_000, 79_000_000}, 10)
	verdict, _ := Decide("FAILED", baseline, incorrect)
	if verdict != VerdictInconclusive {
		t.Fatalf("incorrect candidate received a performance verdict: %s", verdict)
	}
}

func TestAnalyzeSkipsDependenciesAndSensitiveFiles(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"main.go":                          "package main\nfunc f() { for i := 0; i < 10; i++ {} }\n",
		".env":                             "SAMPLE=ignored-fixture",
		"credentials.json":                 `{"fixture":"ignored-fixture"}`,
		"secrets/private.go":               "package secrets\nfunc leak() { for {} }",
		"node_modules/dependency/index.js": "const x = JSON.parse('{}')",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := Analyze(root)
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesInspected != 1 || len(result.Languages) != 1 || result.Languages[0] != "Go" {
		t.Fatalf("sensitive, generated, or dependency file was inspected: %+v", result)
	}
	if len(result.Findings) != 1 || result.Findings[0].Category != "cpu_hypothesis" {
		t.Fatalf("expected one static CPU hypothesis, got %+v", result.Findings)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "ignored-fixture") {
		t.Fatal("analysis output contains sensitive file contents")
	}
}

func TestAnalyzeGoIgnoresPatternWordsInStringsAndComments(t *testing.T) {
	root := t.TempDir()
	source := `package main
// for SELECT name FROM users fetch(
const example = "for SELECT name FROM users fetch("
`
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Analyze(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("comments or string literals created static findings: %+v", result.Findings)
	}
}

func TestReadReportValidatesSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	report := Report{SchemaVersion: SchemaVersion, Project: "sample", Component: "operation", Verdict: VerdictInconclusive}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadReport(path); err != nil {
		t.Fatalf("valid report rejected: %v", err)
	}
	legacy := `{"schema_version":"1.0","project":"sample","component":"operation","verdict":"INCONCLUSIVE","measurements":[{"name":"Go baseline","status":"measured","repetitions":3,"iterations":10}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	loadedLegacy, err := ReadReport(path)
	if err != nil {
		t.Fatalf("legacy report rejected: %v", err)
	}
	if loadedLegacy.Measurements[0].WarmupRuns != 1 {
		t.Fatalf("legacy warm-up metadata was not migrated: %+v", loadedLegacy.Measurements[0])
	}
	report.SchemaVersion = "999"
	data, _ = json.Marshal(report)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadReport(path); err == nil {
		t.Fatal("unsupported schema version was accepted")
	}
	if err := os.WriteFile(path, []byte(`{"schema_version":"1.0","project":"p","component":"c","verdict":"INCONCLUSIVE"} {}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadReport(path); err == nil {
		t.Fatal("trailing report data was accepted")
	}
	report.SchemaVersion = SchemaVersion
	report.Verdict = "NOT_A_VERDICT"
	data, _ = json.Marshal(report)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadReport(path); err == nil {
		t.Fatal("invalid verdict was accepted")
	}
}

func TestCompareReportsRefusesDifferentWorkloads(t *testing.T) {
	first := Report{Project: "a", Component: "x", Workload: "one", Iterations: 5}
	second := Report{Project: "b", Component: "y", Workload: "two", Iterations: 5}
	if !strings.Contains(CompareReports(first, second), "component or workload definitions differ") {
		t.Fatal("different workload definitions were treated as comparable")
	}
}

func TestCompareReportsRefusesDifferentEnvironments(t *testing.T) {
	first := Report{Project: "a", Component: "same", Workload: "same", Iterations: 5, Correctness: "PASSED_FOR_CHECKSUM", Environment: map[string]string{"os": "windows"}}
	second := Report{Project: "b", Component: "same", Workload: "same", Iterations: 5, Correctness: "PASSED_FOR_CHECKSUM", Environment: map[string]string{"os": "linux"}}
	if !strings.Contains(CompareReports(first, second), `environment metadata differs for "os"`) {
		t.Fatal("different environments were treated as comparable")
	}
}

func TestCompareReportsRefusesInconclusiveEvidence(t *testing.T) {
	report := Report{
		Project: "sample", Component: "operation", Workload: "fixed", Iterations: 10,
		Correctness: "PASSED_FOR_CHECKSUM", Verdict: VerdictInconclusive,
	}
	if !strings.Contains(CompareReports(report, report), "inconclusive or invalid measurements") {
		t.Fatal("inconclusive reports were compared as valid evidence")
	}
}

func TestReadReportNormalizesVerdictWhenRawMeasurementsAreInvalid(t *testing.T) {
	baseline := testMeasurement("Go baseline", []uint64{100_000_000, 101_000_000, 99_000_000}, 10)
	candidate := testMeasurement("Rust candidate", []uint64{80_000_000, 81_000_000, 79_000_000}, 10)
	candidate.P95NS++
	delta := 25.0
	report := Report{
		SchemaVersion: SchemaVersion, Project: "sample", Component: "operation",
		Workload: "fixed", Iterations: 10, Correctness: "PASSED_FOR_CHECKSUM",
		Verdict: VerdictPort, ThroughputDeltaPercent: &delta,
		Measurements: []Measurement{baseline, candidate},
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := ReadReport(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Verdict != VerdictInconclusive || loaded.ThroughputDeltaPercent != nil {
		t.Fatalf("invalid raw data retained a performance verdict: %+v", loaded)
	}
}

func TestReadReportRequiresMatchingRecordedCorrectnessOutputs(t *testing.T) {
	baseline := testMeasurement("Go baseline", []uint64{100_000_000, 101_000_000, 99_000_000}, 10)
	candidate := testMeasurement("Rust candidate", []uint64{80_000_000, 81_000_000, 79_000_000}, 10)
	candidate.CorrectnessOutput = "different"
	for i := range candidate.Samples {
		candidate.Samples[i].Output = "different"
	}
	report := Report{
		SchemaVersion: SchemaVersion, Project: "sample", Component: "operation",
		Workload: "fixed", Iterations: 10, Correctness: "PASSED_FOR_CHECKSUM",
		Verdict: VerdictPort, Measurements: []Measurement{baseline, candidate},
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := ReadReport(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Verdict != VerdictInconclusive || loaded.ThroughputDeltaPercent != nil {
		t.Fatalf("mismatched output evidence retained a verdict: %+v", loaded)
	}
}

func TestRenderMarkdownMarksMissingMetricsAsUnavailable(t *testing.T) {
	report := Report{SchemaVersion: SchemaVersion, Measurements: []Measurement{{Name: "Rust candidate", Status: "unavailable"}}}
	rendered := RenderMarkdown(report)
	if !strings.Contains(rendered, "| Rust candidate | unavailable | 0 | 0 | 0 | n/a | n/a | n/a | n/a | n/a |") {
		t.Fatalf("missing metrics were rendered as numeric values: %s", rendered)
	}
}

func TestRenderMarkdownDoesNotDisplayInvalidAggregates(t *testing.T) {
	measurement := testMeasurement("Go baseline", []uint64{100, 200, 300}, 10)
	measurement.P95NS++
	rendered := RenderMarkdown(Report{
		Correctness: "PASSED_FOR_CHECKSUM",
		Verdict:     VerdictPort,
		Measurements: []Measurement{
			measurement,
		},
	})
	if !strings.Contains(rendered, "| Go baseline | invalid | 3 | 1 | 0 | n/a | n/a | n/a | n/a | n/a |") ||
		!strings.Contains(rendered, "Verdict:** `INCONCLUSIVE`") {
		t.Fatalf("invalid aggregate values were rendered as measured: %s", rendered)
	}
}

func TestWriteReportPreservesRawMeasurementsAndRendersComparison(t *testing.T) {
	dir := t.TempDir()
	report := Report{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Project:       "sample", Component: "operation", Workload: "fixed input",
		Iterations: 10, Correctness: "PASSED_FOR_CHECKSUM", Verdict: VerdictInconclusive,
		Reason: "variability exceeded threshold", Environment: map[string]string{"os": "test-os"},
		Measurements: []Measurement{
			testMeasurement("Go baseline", []uint64{100, 200, 300}, 10),
		},
		Reproduction: "rustrush demo --repetitions 3",
	}
	if err := WriteReport(dir, report); err != nil {
		t.Fatal(err)
	}
	loaded, err := ReadReport(filepath.Join(dir, "latest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Measurements) != 1 || len(loaded.Measurements[0].Samples) != 3 ||
		loaded.Measurements[0].Samples[1].DurationNS != 200 {
		t.Fatalf("raw samples were not preserved in JSON: %+v", loaded.Measurements)
	}
	markdown, err := os.ReadFile(filepath.Join(dir, "latest.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"not estimates", "not per-request latency", "Warm-ups", "Errors",
		"**Go baseline raw process samples (ns):** [100 200 300]", "Machine and runtime metadata",
		"variability exceeded threshold",
	} {
		if !strings.Contains(string(markdown), expected) {
			t.Errorf("Markdown report is missing %q:\n%s", expected, markdown)
		}
	}
}

func TestRedactBuildOutputRemovesLocalPaths(t *testing.T) {
	source := `C:\Users\private\RustRush\examples\cpu-heavy\candidate.rs`
	tempDir := `C:\Users\private\AppData\Local\Temp\rustrush-demo`
	rustc := `C:\Users\private\.cargo\bin\rustc.exe`
	output := redactBuildOutput(source+" "+tempDir+" "+rustc, source, tempDir, rustc)
	if strings.Contains(output, `C:\Users\private`) {
		t.Fatalf("local path was not redacted: %s", output)
	}
	if !strings.Contains(output, "[candidate source]") ||
		!strings.Contains(output, "[temporary build directory]") ||
		!strings.Contains(output, "[rustc]") {
		t.Fatalf("expected sanitized diagnostic placeholders, got %s", output)
	}
}

func TestCalculateROIModelsOnlySuppliedAssumptions(t *testing.T) {
	inputs := ROIInputs{
		MonthlyOperations:              floatPointer(3_600_000),
		BaselineCPUSecondsPerOperation: floatPointer(0.001),
		TargetCPUSecondsPerOperation:   floatPointer(0.0005),
		CPUCostPerHourUSD:              floatPointer(10),
		EngineeringHours:               floatPointer(10),
		EngineeringCostPerHourUSD:      floatPointer(100),
		MonthlyMaintenanceCostDeltaUSD: floatPointer(1),
	}
	result, err := CalculateROI(inputs)
	if err != nil {
		t.Fatal(err)
	}
	if result.CurrentMonthlyCPUHours == nil || *result.CurrentMonthlyCPUHours != 1 {
		t.Fatalf("unexpected current CPU-hours estimate: %+v", result.CurrentMonthlyCPUHours)
	}
	if result.TargetMonthlyCostUSD == nil || *result.TargetMonthlyCostUSD != 5 {
		t.Fatalf("unexpected target monthly cost: %+v", result.TargetMonthlyCostUSD)
	}
	if result.MonthlyInfrastructureSavingsUSD == nil || *result.MonthlyInfrastructureSavingsUSD != 5 {
		t.Fatalf("unexpected infrastructure savings: %+v", result.MonthlyInfrastructureSavingsUSD)
	}
	if result.EstimatedNetMonthlySavingsUSD == nil || *result.EstimatedNetMonthlySavingsUSD != 4 {
		t.Fatalf("unexpected net monthly savings: %+v", result.EstimatedNetMonthlySavingsUSD)
	}
	if result.BreakEvenMonths == nil || *result.BreakEvenMonths != 250 {
		t.Fatalf("unexpected break-even estimate: %+v", result.BreakEvenMonths)
	}
	if len(result.MissingInputs) != 0 || result.Type != "modeled_estimate" {
		t.Fatalf("unexpected ROI result metadata: %+v", result)
	}
}

func TestCalculateROIRetainsMissingAssumptionsInsteadOfInventingValues(t *testing.T) {
	result, err := CalculateROI(ROIInputs{MonthlyOperations: floatPointer(1000)})
	if err != nil {
		t.Fatal(err)
	}
	if result.CurrentMonthlyCostUSD != nil || result.BreakEvenMonths != nil {
		t.Fatal("incomplete inputs generated cost or break-even estimates")
	}
	if len(result.MissingInputs) != 6 {
		t.Fatalf("expected six missing inputs, got %v", result.MissingInputs)
	}
}

func TestCalculateROIRejectsInvalidInputsAndOverflow(t *testing.T) {
	if _, err := CalculateROI(ROIInputs{MonthlyOperations: floatPointer(math.NaN())}); err == nil {
		t.Fatal("NaN input was accepted")
	}
	if _, err := CalculateROI(ROIInputs{MonthlyOperations: floatPointer(0)}); err == nil {
		t.Fatal("zero operations was accepted")
	}
	if _, err := CalculateROI(ROIInputs{
		MonthlyOperations:              floatPointer(math.MaxFloat64),
		BaselineCPUSecondsPerOperation: floatPointer(math.MaxFloat64),
	}); err == nil {
		t.Fatal("overflowing modeled CPU hours were accepted")
	}
}

func floatPointer(value float64) *float64 {
	return &value
}

func testMeasurement(name string, durations []uint64, iterations uint64) Measurement {
	samples := make([]Sample, len(durations))
	for i, duration := range durations {
		samples[i] = Sample{DurationNS: duration, Output: "same"}
	}
	summary, err := Summarize(samples, iterations)
	if err != nil {
		panic(err)
	}
	return Measurement{
		Name: name, Status: "measured", Repetitions: len(samples), WarmupRuns: 1,
		CorrectnessOutput: "same", Iterations: iterations, Samples: samples, MedianNS: summary.MedianNS,
		P50NS: summary.P50NS, P95NS: summary.P95NS, P99NS: summary.P99NS,
		OpsPerSec: summary.OpsPerSec, CoefficientOfVariation: summary.CoefficientOfVariation,
	}
}

func TestMeasurementBelowClockResolutionQualityBudgetIsInconclusive(t *testing.T) {
	baseline := testMeasurement("Go baseline", []uint64{999_000, 999_000, 999_000}, 10)
	candidate := testMeasurement("Rust candidate", []uint64{999_000, 999_000, 999_000}, 10)
	for _, measurement := range []*Measurement{&baseline, &candidate} {
		for i := range measurement.Samples {
			measurement.Samples[i].TimerResolutionNS = 1_000
		}
		if err := summarizeMeasurement(measurement); err != nil {
			t.Fatal(err)
		}
	}
	verdict, reason := Decide("PASSED_FOR_CHECKSUM", baseline, candidate)
	if verdict != VerdictInconclusive || !strings.Contains(reason, "1,000-tick quality screen") {
		t.Fatalf("short samples must be inconclusive with a diagnostic, got %s: %s", verdict, reason)
	}
	if len(baseline.Samples) != 3 || baseline.Samples[0].DurationNS != 999_000 {
		t.Fatalf("raw samples must be retained: %+v", baseline.Samples)
	}
	if validMeasurement(baseline) {
		t.Fatal("quality warnings must invalidate aggregate measurements")
	}
}

func TestMeasurementAtClockQualityBudgetRemainsEligible(t *testing.T) {
	measurement := testMeasurement("Go baseline", []uint64{1_000_000, 1_000_000, 1_000_000}, 10)
	for i := range measurement.Samples {
		measurement.Samples[i].TimerResolutionNS = 1_000
	}
	if err := summarizeMeasurement(&measurement); err != nil {
		t.Fatal(err)
	}
	if len(measurement.Warnings) != 0 {
		t.Fatalf("exactly 1,000 ticks should not warn: %v", measurement.Warnings)
	}
}
