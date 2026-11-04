package rustrush

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHistoricalCompleteReportKeepsVerdictAndTechnicalDetails(t *testing.T) {
	report, err := ReadReport(filepath.Join("testdata", "report-v1.0-complete.json"))
	if err != nil {
		t.Fatalf("historical report rejected: %v", err)
	}
	if report.SchemaVersion != "1.0" || report.Verdict != VerdictPort {
		t.Fatalf("existing verdict not preserved: %s %s", report.SchemaVersion, report.Verdict)
	}
	if report.Measurements[0].WarmupRuns != 1 || report.Measurements[0].CorrectnessOutput != "4242" {
		t.Fatalf("legacy measurement was not migrated: %+v", report.Measurements[0])
	}
	summary := SummarizeResult(report)
	if summary.Conclusion != ConclusionFaster || summary.Headline != "Rust appears faster for this workload." {
		t.Fatalf("historical PORT misinterpreted: %+v", summary)
	}
	markdown := RenderMarkdown(report)
	for _, detail := range []string{"p95 (ms)", "p99 (ms)", "Ops/sec", "raw process samples", "## Uncertainty", "## Limitations", "historical limitation", "`PORT`", "PASSED_FOR_CHECKSUM"} {
		if !strings.Contains(markdown, detail) {
			t.Fatalf("technical detail %q missing from historical report:\n%s", detail, markdown)
		}
	}
}

func TestHistoricalReportWithMissingFieldsNeverClaimsAWin(t *testing.T) {
	report, err := ReadReport(filepath.Join("testdata", "report-v1.0-missing-fields.json"))
	if err != nil {
		t.Fatalf("historical report rejected: %v", err)
	}
	if report.Verdict != VerdictInconclusive {
		t.Fatalf("stored PORT without evidence survived loading: %s", report.Verdict)
	}
	summary := SummarizeResult(report)
	if summary.Conclusion != ConclusionInsufficient || summary.Headline != "Not enough evidence." {
		t.Fatalf("missing evidence produced %+v", summary)
	}
	markdown := strings.ToLower(RenderMarkdown(report))
	if strings.Contains(markdown, "appears faster") || strings.Contains(markdown, "no improvement was demonstrated") {
		t.Fatalf("missing evidence produced a performance conclusion:\n%s", markdown)
	}
}

func TestRenderMarkdownLeadsWithResultOnceAndKeepsDetails(t *testing.T) {
	baseline, candidate := goodPair([]uint64{100_000_000, 101_000_000, 99_000_000}, []uint64{80_000_000, 81_000_000, 79_000_000})
	report := summaryReport("", "PASSED_FOR_CHECKSUM", VerdictPort, baseline, candidate)
	markdown := RenderMarkdown(report)
	if !strings.HasPrefix(markdown, "# RustRush benchmark report\n\n## Result\n\n**Rust appears faster for this workload.**") {
		t.Fatalf("report does not lead with the result:\n%s", markdown)
	}
	if strings.Count(markdown, "Rust appears faster for this workload.") != 1 || strings.Count(markdown, "**Next step:**") != 1 {
		t.Fatalf("summary repeated:\n%s", markdown)
	}
	if strings.Index(markdown, "## Result") > strings.Index(markdown, "## Measurements") {
		t.Fatal("result must precede the technical tables")
	}
	for _, detail := range []string{"- **Verdict:** `PORT`", "- **Correctness gate:** PASSED_FOR_CHECKSUM", "| Implementation |", "**Measured throughput delta**"} {
		if !strings.Contains(markdown, detail) {
			t.Fatalf("existing detail %q removed:\n%s", detail, markdown)
		}
	}
}

func TestWriteReportKeepsFilenamesAndJSONSchema(t *testing.T) {
	baseline, candidate := goodPair([]uint64{100_000_000, 101_000_000, 99_000_000}, []uint64{80_000_000, 81_000_000, 79_000_000})
	report := summaryReport("", "PASSED_FOR_CHECKSUM", VerdictPort, baseline, candidate)
	report.Project, report.Component = "p", "c"
	dir := filepath.Join(t.TempDir(), "out")
	if err := WriteReport(dir, report); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"latest.json", "latest.md", "latest.svg"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s not written: %v", name, err)
		}
	}
	// ReadReport rejects unknown fields, so this also proves the JSON gained none.
	loaded, err := ReadReport(filepath.Join(dir, "latest.json"))
	if err != nil || loaded.Verdict != VerdictPort {
		t.Fatalf("written JSON not readable as the existing schema: %v %+v", err, loaded.Verdict)
	}
	data, err := os.ReadFile(filepath.Join(dir, "latest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "Rust appears faster") {
		t.Fatal("plain-English text must not be added to the JSON report")
	}
}
