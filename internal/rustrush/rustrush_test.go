package rustrush

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestDecideRequiresCorrectnessAndReliableEvidence(t *testing.T) {
	baseline := Measurement{
		Status: "measured", Iterations: 100, OpsPerSec: 100,
		Samples: []Sample{{DurationNS: 100}, {DurationNS: 101}, {DurationNS: 99}},
	}
	candidate := Measurement{
		Status: "measured", Iterations: 100, OpsPerSec: 120,
		Samples: []Sample{{DurationNS: 80}, {DurationNS: 81}, {DurationNS: 79}},
	}
	verdict, _ := Decide("NOT_RUN", baseline, candidate)
	if verdict != VerdictInconclusive {
		t.Fatalf("incorrectness did not invalidate verdict: %s", verdict)
	}
	verdict, _ = Decide("PASSED_FOR_CHECKSUM", baseline, candidate)
	if verdict != VerdictPort {
		t.Fatalf("expected PORT for reliable gain, got %s", verdict)
	}
	candidate.OpsPerSec = 80
	verdict, _ = Decide("PASSED_FOR_CHECKSUM", baseline, candidate)
	if verdict != VerdictDontPort {
		t.Fatalf("expected DONT_PORT for reliable regression, got %s", verdict)
	}
	candidate.OpsPerSec = 101
	verdict, _ = Decide("PASSED_FOR_CHECKSUM", baseline, candidate)
	if verdict != VerdictOptimizeFirst {
		t.Fatalf("expected OPTIMIZE_FIRST for marginal gain, got %s", verdict)
	}
	candidate.Samples = []Sample{{DurationNS: 20}, {DurationNS: 100}, {DurationNS: 300}}
	verdict, _ = Decide("PASSED_FOR_CHECKSUM", baseline, candidate)
	if verdict != VerdictInconclusive {
		t.Fatalf("noisy measurements did not produce an inconclusive verdict: %s", verdict)
	}
}

func TestIncorrectCandidateOutputCannotPassCorrectnessGate(t *testing.T) {
	baseline := Measurement{Samples: []Sample{{Output: "expected"}, {Output: "expected"}}}
	incorrect := Measurement{Samples: []Sample{{Output: "wrong"}, {Output: "wrong"}}}
	if outputsMatch(baseline, incorrect) {
		t.Fatal("intentionally incorrect candidate passed the correctness gate")
	}
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
	first := Report{Project: "a", Component: "same", Workload: "same", Iterations: 5, Environment: map[string]string{"os": "windows"}}
	second := Report{Project: "b", Component: "same", Workload: "same", Iterations: 5, Environment: map[string]string{"os": "linux"}}
	if !strings.Contains(CompareReports(first, second), `environment metadata differs for "os"`) {
		t.Fatal("different environments were treated as comparable")
	}
}

func TestRenderMarkdownMarksMissingMetricsAsUnavailable(t *testing.T) {
	report := Report{SchemaVersion: SchemaVersion, Measurements: []Measurement{{Name: "Rust candidate", Status: "unavailable"}}}
	rendered := RenderMarkdown(report)
	if !strings.Contains(rendered, "| Rust candidate | unavailable | n/a | n/a | n/a | n/a | n/a | n/a |") {
		t.Fatalf("missing metrics were rendered as numeric values: %s", rendered)
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
