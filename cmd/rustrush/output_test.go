package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TejasMehra/RustRush/internal/rustrush"
)

func cliMeasurement(name string, durations []uint64) rustrush.Measurement {
	samples := make([]rustrush.Sample, len(durations))
	for i, duration := range durations {
		samples[i] = rustrush.Sample{DurationNS: duration, Output: "same"}
	}
	summary, err := rustrush.Summarize(samples, 10)
	if err != nil {
		panic(err)
	}
	return rustrush.Measurement{
		Name: name, Status: "measured", Repetitions: len(samples), WarmupRuns: 1, CorrectnessOutput: "same",
		Iterations: 10, Samples: samples, MedianNS: summary.MedianNS, P50NS: summary.P50NS, P95NS: summary.P95NS,
		P99NS: summary.P99NS, OpsPerSec: summary.OpsPerSec, CoefficientOfVariation: summary.CoefficientOfVariation,
	}
}

func cliReport(correctness string, verdict rustrush.Verdict, baseline, candidate rustrush.Measurement) rustrush.Report {
	return rustrush.Report{
		SchemaVersion: rustrush.SchemaVersion, Project: "p", Component: "c", Correctness: correctness,
		Verdict: verdict, Environment: map[string]string{}, Measurements: []rustrush.Measurement{baseline, candidate},
		Limitations: []string{"example limitation"},
	}
}

var (
	baselineDurations = []uint64{100_000_000, 101_000_000, 99_000_000}
	technicalMarkers  = []string{"| Implementation |", "p95", "p99", "Median", "Ops/sec", "Warm-ups", "raw process samples"}
)

func TestDefaultOutputIsConciseForEveryConclusion(t *testing.T) {
	baseline := cliMeasurement("Go baseline", baselineDurations)
	failed := cliMeasurement("Rust candidate", baselineDurations)
	failed.Status = "correctness_failed"
	noisy := cliMeasurement("Rust candidate", []uint64{20_000_000, 100_000_000, 300_000_000})
	cases := []struct {
		name     string
		report   rustrush.Report
		headline string
		other    []string
	}{
		{"faster", cliReport("PASSED_FOR_CHECKSUM", rustrush.VerdictPort, baseline, cliMeasurement("Rust candidate", []uint64{80_000_000, 81_000_000, 79_000_000})),
			"Rust appears faster for this workload.", nil},
		{"negligible", cliReport("PASSED_FOR_CHECKSUM", rustrush.VerdictOptimizeFirst, baseline, cliMeasurement("Rust candidate", []uint64{99_000_000, 100_000_000, 101_000_000})),
			"No improvement was demonstrated.", nil},
		{"slower", cliReport("PASSED_FOR_CHECKSUM", rustrush.VerdictDontPort, baseline, cliMeasurement("Rust candidate", []uint64{120_000_000, 121_000_000, 119_000_000})),
			"No improvement was demonstrated.", nil},
		{"inconclusive noisy", cliReport("PASSED_FOR_CHECKSUM", rustrush.VerdictInconclusive, baseline, noisy),
			"Not enough evidence.", []string{"varied too much"}},
		{"correctness failure", cliReport("FAILED", rustrush.VerdictInconclusive, baseline, failed),
			"Not enough evidence.", []string{"did not produce the same output"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			printBenchmarkResult(&out, tc.report, "results", false)
			text := out.String()
			for _, want := range append([]string{tc.headline, "Next step:", "Keep in mind:", "this machine", "whole backend",
				filepath.Join("results", "latest.md"), filepath.Join("results", "latest.json"), "--verbose"}, tc.other...) {
				if !strings.Contains(text, want) {
					t.Fatalf("default output missing %q:\n%s", want, text)
				}
			}
			for _, marker := range technicalMarkers {
				if strings.Contains(text, marker) {
					t.Fatalf("default output contains technical detail %q:\n%s", marker, text)
				}
			}
			if strings.Count(text, tc.headline) != 1 || strings.Count(text, "Next step:") != 1 {
				t.Fatalf("summary printed more than once:\n%s", text)
			}
			if strings.Contains(strings.ToLower(text), "other programs") {
				t.Fatalf("output assumes a cause for variability:\n%s", text)
			}
		})
	}
}

func TestVerboseOutputPrintsFullReportWithSummaryOnce(t *testing.T) {
	baseline := cliMeasurement("Go baseline", baselineDurations)
	report := cliReport("PASSED_FOR_CHECKSUM", rustrush.VerdictPort, baseline, cliMeasurement("Rust candidate", []uint64{80_000_000, 81_000_000, 79_000_000}))
	var out bytes.Buffer
	printBenchmarkResult(&out, report, "results", true)
	text := out.String()
	if !strings.HasPrefix(text, rustrush.RenderMarkdown(report)) {
		t.Fatalf("verbose output must be the complete existing Markdown report:\n%s", text)
	}
	for _, want := range []string{"| Implementation |", "p95 (ms)", "raw process samples", "## Limitations", "example limitation", "Correctness gate:", "Reports written to " + filepath.Join("results", "latest.json")} {
		if !strings.Contains(text, want) {
			t.Fatalf("verbose output missing %q:\n%s", want, text)
		}
	}
	if strings.Count(text, "Rust appears faster for this workload.") != 1 || strings.Count(text, "Next step:") != 1 {
		t.Fatalf("summary duplicated in verbose output:\n%s", text)
	}
	if strings.Contains(text, "Full report:") || strings.Contains(text, "Run again with --verbose") {
		t.Fatalf("concise-mode footer leaked into verbose output:\n%s", text)
	}
}

func TestVerboseOutputKeepsCorrectnessFailureAndInconclusiveNotice(t *testing.T) {
	baseline := cliMeasurement("Go baseline", baselineDurations)
	failed := cliMeasurement("Rust candidate", baselineDurations)
	failed.Status = "correctness_failed"
	failed.Error = "outputs differ"
	report := cliReport("FAILED", rustrush.VerdictInconclusive, baseline, failed)
	report.Reason = "The correctness probes returned different or empty outputs."
	var out bytes.Buffer
	printBenchmarkResult(&out, report, "results", true)
	for _, want := range []string{"Correctness gate:** FAILED", "outputs differ", "correctness probes returned different", "The comparison is inconclusive; see limitations in the report."} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("verbose output hid %q:\n%s", want, out.String())
		}
	}
}

func TestBenchmarkCommandsAcceptVerboseFlag(t *testing.T) {
	for _, args := range [][]string{{"candidate", "--verbose"}, {"demo", "--verbose", "--iterations", "0"}} {
		err := run(args)
		if err == nil {
			t.Fatalf("%v unexpectedly succeeded", args)
		}
		if strings.Contains(err.Error(), "not defined") {
			t.Fatalf("%v does not accept --verbose: %v", args, err)
		}
	}
}
