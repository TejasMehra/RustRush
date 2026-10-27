package rustrush

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"
)

const (
	SchemaVersion      = "1.0"
	DefaultIterations  = uint64(2_000_000)
	DefaultRepetitions = 5
)

type Verdict string

const (
	VerdictPort          Verdict = "PORT"
	VerdictOptimizeFirst Verdict = "OPTIMIZE_FIRST"
	VerdictDontPort      Verdict = "DONT_PORT"
	VerdictInconclusive  Verdict = "INCONCLUSIVE"
)

type Toolchain struct {
	Go    string `json:"go,omitempty"`
	Rustc string `json:"rustc,omitempty"`
}

type Analysis struct {
	Project        string    `json:"project"`
	FilesInspected int       `json:"files_inspected"`
	Languages      []string  `json:"languages"`
	Findings       []Finding `json:"findings"`
}

type Finding struct {
	Category    string `json:"category"`
	Description string `json:"description"`
	Location    string `json:"location"`
	Evidence    string `json:"evidence"`
	Confidence  string `json:"confidence"`
	NextStep    string `json:"next_step"`
}

type Sample struct {
	DurationNS uint64 `json:"duration_ns"`
	Output     string `json:"output,omitempty"`
}

type Measurement struct {
	Name                   string   `json:"name"`
	Status                 string   `json:"status"`
	Repetitions            int      `json:"repetitions"`
	Iterations             uint64   `json:"iterations"`
	Samples                []Sample `json:"samples"`
	MedianNS               uint64   `json:"median_ns,omitempty"`
	P50NS                  uint64   `json:"p50_ns,omitempty"`
	P95NS                  uint64   `json:"p95_ns,omitempty"`
	P99NS                  uint64   `json:"p99_ns,omitempty"`
	OpsPerSec              float64  `json:"ops_per_sec,omitempty"`
	CoefficientOfVariation float64  `json:"coefficient_of_variation,omitempty"`
	Error                  string   `json:"error,omitempty"`
}

type Report struct {
	SchemaVersion            string            `json:"schema_version"`
	GeneratedAt              time.Time         `json:"generated_at"`
	Project                  string            `json:"project"`
	Component                string            `json:"component"`
	Workload                 string            `json:"workload"`
	Iterations               uint64            `json:"iterations"`
	Correctness              string            `json:"correctness"`
	Verdict                  Verdict           `json:"verdict"`
	Reason                   string            `json:"reason"`
	ThroughputDeltaOpsPerSec *float64          `json:"throughput_delta_ops_per_sec,omitempty"`
	ThroughputDeltaPercent   *float64          `json:"throughput_delta_percent,omitempty"`
	Environment              map[string]string `json:"environment"`
	Measurements             []Measurement     `json:"measurements"`
	Limitations              []string          `json:"limitations"`
	Reproduction             string            `json:"reproduction"`
	Assumptions              map[string]string `json:"assumptions,omitempty"`
}

type Summary struct {
	MedianNS               uint64
	P50NS                  uint64
	P95NS                  uint64
	P99NS                  uint64
	OpsPerSec              float64
	CoefficientOfVariation float64
}

func WriteJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func ReadReport(path string) (Report, error) {
	var report Report
	file, err := os.Open(path)
	if err != nil {
		return report, fmt.Errorf("open report: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return report, fmt.Errorf("inspect report file: %w", err)
	}
	if info.Size() > 4<<20 {
		return report, errors.New("report exceeds the 4 MiB size limit")
	}

	decoder := json.NewDecoder(io.LimitReader(file, 4<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return report, fmt.Errorf("decode report: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return report, errors.New("report contains trailing JSON data")
		}
		return report, fmt.Errorf("read end of report: %w", err)
	}
	if report.SchemaVersion != SchemaVersion {
		return report, fmt.Errorf("unsupported report schema %q", report.SchemaVersion)
	}
	if report.Project == "" || report.Component == "" {
		return report, errors.New("report is missing required project or component fields")
	}
	switch report.Verdict {
	case VerdictPort, VerdictOptimizeFirst, VerdictDontPort, VerdictInconclusive:
	default:
		return report, fmt.Errorf("report has invalid verdict %q", report.Verdict)
	}
	return report, nil
}

func WriteReport(directory string, report Report) error {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create report directory: %w", err)
	}
	jsonPath := filepath.Join(directory, "latest.json")
	file, err := os.Create(jsonPath)
	if err != nil {
		return fmt.Errorf("create JSON report: %w", err)
	}
	writeErr := WriteJSON(file, report)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("write JSON report: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close JSON report: %w", closeErr)
	}
	markdownPath := filepath.Join(directory, "latest.md")
	if err := os.WriteFile(markdownPath, []byte(RenderMarkdown(report)), 0o644); err != nil {
		return fmt.Errorf("write Markdown report: %w", err)
	}
	return nil
}

func RenderMarkdown(report Report) string {
	out := fmt.Sprintf("# RustRush benchmark report\n\n- **Schema:** %s\n- **Generated:** %s\n- **Project:** %s\n- **Component:** %s\n- **Verdict:** `%s`\n- **Correctness:** %s\n\n## Workload\n\n%s\n\nIterations per process: %d. Reproduction: `%s`.\n\n## Measurements\n\n| Implementation | Status | Trials | Median/p50 (ms) | p95 (ms) | p99 (ms) | Ops/sec | CV |\n|---|---:|---:|---:|---:|---:|---:|---:|\n",
		report.SchemaVersion, report.GeneratedAt.UTC().Format(time.RFC3339), report.Project, report.Component, report.Verdict, report.Correctness, report.Workload, report.Iterations, report.Reproduction)
	for _, measurement := range report.Measurements {
		if measurement.Status != "measured" {
			out += fmt.Sprintf("| %s | %s | n/a | n/a | n/a | n/a | n/a | n/a |\n", measurement.Name, measurement.Status)
			continue
		}
		out += fmt.Sprintf("| %s | %s | %d | %.3f | %.3f | %.3f | %.2f | %.3f |\n",
			measurement.Name, measurement.Status, measurement.Repetitions,
			float64(measurement.MedianNS)/1e6, float64(measurement.P95NS)/1e6,
			float64(measurement.P99NS)/1e6, measurement.OpsPerSec, measurement.CoefficientOfVariation)
	}
	if report.ThroughputDeltaPercent != nil && report.ThroughputDeltaOpsPerSec != nil {
		out += fmt.Sprintf("\nThroughput delta (Rust candidate vs Go baseline): %+.2f ops/s (%+.2f%%).\n",
			*report.ThroughputDeltaOpsPerSec, *report.ThroughputDeltaPercent)
	}
	out += "\n## Rationale\n\n" + report.Reason + "\n"
	if len(report.Limitations) > 0 {
		out += "\n## Limitations\n"
		for _, limitation := range report.Limitations {
			out += "- " + limitation + "\n"
		}
	}
	return out
}

func CompareReports(first, second Report) string {
	out := fmt.Sprintf("Report A: %s — %s\nReport B: %s — %s\n", first.Project, first.Component, second.Project, second.Component)
	if first.Component != second.Component || first.Workload != second.Workload || first.Iterations != second.Iterations {
		return out + "Comparison unavailable: component or workload definitions differ.\n"
	}
	if first.Correctness == "FAILED" || second.Correctness == "FAILED" {
		return out + "Comparison unavailable: a report failed its correctness check.\n"
	}
	for _, key := range []string{"os", "architecture", "go_runtime", "cpu_count"} {
		if first.Environment[key] == "" || second.Environment[key] == "" || first.Environment[key] != second.Environment[key] {
			return out + fmt.Sprintf("Comparison unavailable: environment metadata differs for %q.\n", key)
		}
	}
	if first.Environment["rustc"] != second.Environment["rustc"] {
		return out + "Comparison unavailable: Rust compiler metadata differs.\n"
	}
	secondByName := make(map[string]Measurement, len(second.Measurements))
	for _, measurement := range second.Measurements {
		secondByName[measurement.Name] = measurement
	}
	comparisons := 0
	for _, aMeasurement := range first.Measurements {
		bMeasurement, ok := secondByName[aMeasurement.Name]
		if !ok || aMeasurement.Status != "measured" || bMeasurement.Status != "measured" ||
			aMeasurement.MedianNS == 0 || bMeasurement.MedianNS == 0 ||
			aMeasurement.OpsPerSec <= 0 || bMeasurement.OpsPerSec <= 0 {
			continue
		}
		change := (bMeasurement.OpsPerSec - aMeasurement.OpsPerSec) / aMeasurement.OpsPerSec * 100
		out += fmt.Sprintf("%s throughput change: %.2f%% (A %.2f ops/s, B %.2f ops/s).\n",
			aMeasurement.Name, change, aMeasurement.OpsPerSec, bMeasurement.OpsPerSec)
		comparisons++
	}
	if comparisons == 0 {
		return out + "Comparison unavailable: no matching implementation has valid measurements in both reports.\n"
	}
	out += "Results still depend on hardware not fully described by the report; use this as a local comparison, not a controlled cross-machine result.\n"
	return out
}

func Summarize(samples []Sample, iterations uint64) (Summary, error) {
	if len(samples) == 0 || iterations == 0 {
		return Summary{}, errors.New("at least one sample and a positive iteration count are required")
	}
	values := make([]uint64, len(samples))
	for i, sample := range samples {
		if sample.DurationNS == 0 {
			return Summary{}, errors.New("sample durations must be positive")
		}
		values[i] = sample.DurationNS
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	median := percentile(values, 0.5)
	return Summary{
		MedianNS:               median,
		P50NS:                  percentile(values, 0.5),
		P95NS:                  percentile(values, 0.95),
		P99NS:                  percentile(values, 0.99),
		OpsPerSec:              float64(iterations) * 1e9 / float64(median),
		CoefficientOfVariation: coefficientOfVariation(samples),
	}, nil
}

func Decide(correctness string, baseline, candidate Measurement) (Verdict, string) {
	if correctness != "PASSED_FOR_CHECKSUM" {
		return VerdictInconclusive, "Correctness was not established; performance evidence is invalid."
	}
	if baseline.Status != "measured" || candidate.Status != "measured" ||
		baseline.Iterations == 0 || baseline.Iterations != candidate.Iterations ||
		len(baseline.Samples) < 3 || len(candidate.Samples) < 3 ||
		baseline.OpsPerSec <= 0 || candidate.OpsPerSec <= 0 {
		return VerdictInconclusive, "Valid repeated measurements for the same workload are unavailable."
	}
	if coefficientOfVariation(baseline.Samples) > 0.10 || coefficientOfVariation(candidate.Samples) > 0.10 {
		return VerdictInconclusive, "Run-to-run variability exceeded 10%; repeat under quieter conditions before deciding."
	}
	gain := (candidate.OpsPerSec - baseline.OpsPerSec) / baseline.OpsPerSec * 100
	switch {
	case gain >= 10:
		return VerdictPort, fmt.Sprintf("The tested Rust candidate delivered %.1f%% higher throughput in this local microbenchmark. Validate with representative application workloads before migrating.", gain)
	case gain <= -10:
		return VerdictDontPort, fmt.Sprintf("The tested Rust candidate delivered %.1f%% lower throughput in this local microbenchmark. This workload alone does not support a port.", -gain)
	default:
		return VerdictOptimizeFirst, fmt.Sprintf("The measured throughput difference was %.1f%%, below the 10%% investigation threshold. Optimize the current implementation and remeasure before considering a migration.", gain)
	}
}

func coefficientOfVariation(samples []Sample) float64 {
	if len(samples) < 2 {
		return math.Inf(1)
	}
	var mean float64
	for _, sample := range samples {
		mean += float64(sample.DurationNS)
	}
	mean /= float64(len(samples))
	if mean == 0 {
		return math.Inf(1)
	}
	var squaredDiffs float64
	for _, sample := range samples {
		difference := float64(sample.DurationNS) - mean
		squaredDiffs += difference * difference
	}
	return math.Sqrt(squaredDiffs/float64(len(samples))) / mean
}

func percentile(values []uint64, p float64) uint64 {
	index := int(math.Ceil(p*float64(len(values)))) - 1
	if index < 0 {
		index = 0
	}
	return values[index]
}

func environment() map[string]string {
	return map[string]string{
		"os":           runtime.GOOS,
		"architecture": runtime.GOARCH,
		"go_runtime":   runtime.Version(),
		"cpu_count":    fmt.Sprint(runtime.NumCPU()),
	}
}
