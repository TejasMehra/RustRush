package rustrush

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func wellFormed(t *testing.T, svg string) {
	t.Helper()
	decoder := xml.NewDecoder(strings.NewReader(svg))
	for {
		if _, err := decoder.Token(); err != nil {
			if err.Error() == "EOF" {
				return
			}
			t.Fatalf("SVG is not well-formed XML: %v\n%s", err, svg)
		}
	}
}

func TestRenderSVGShowsRealMeasurements(t *testing.T) {
	report := roiReport([]uint64{2_000_000, 2_000_000, 2_000_000}, []uint64{1_000_000, 1_000_000, 1_000_000})
	report.Verdict = VerdictPort
	report.Environment = map[string]string{"os": "linux", "isolation": "container"}
	svg := RenderSVG(report)
	wellFormed(t, svg)
	for _, expected := range []string{"PORT", "PASSED_FOR_CHECKSUM", "p50 2.000 ms", "p50 1.000 ms", "+100.00%", "os=linux", "isolation=container", "not per-request latency"} {
		if !strings.Contains(svg, expected) {
			t.Errorf("SVG is missing %q", expected)
		}
	}
}

func TestRenderSVGNeverDrawsMissingMeasurementsAsZero(t *testing.T) {
	report := roiReport([]uint64{1000, 1000, 1000}, []uint64{500, 500, 500})
	report.Measurements[1] = Measurement{Name: "Rust candidate", Status: "build_failed", ErrorCount: 1}
	svg := RenderSVG(report)
	wellFormed(t, svg)
	if !strings.Contains(svg, "INCONCLUSIVE") || !strings.Contains(svg, "n/a — build_failed") ||
		strings.Contains(svg, "Measured throughput delta") || strings.Contains(svg, "p50 0.000") {
		t.Fatalf("missing data was misrepresented:\n%s", svg)
	}
	if strings.Count(svg, `fill="#0969da"`) != 1 {
		t.Fatal("only the valid measurement may have a bar")
	}
	empty := RenderSVG(Report{SchemaVersion: SchemaVersion})
	wellFormed(t, empty)
	if !strings.Contains(empty, "INCONCLUSIVE") {
		t.Fatal("an empty report must be inconclusive")
	}
}

func TestRenderSVGEscapesUntrustedText(t *testing.T) {
	report := roiReport([]uint64{1000, 1000, 1000}, []uint64{500, 500, 500})
	report.Project = `</text><script>alert(1)</script>`
	report.Workload = "a & b \x00 \x1b <x>"
	report.Environment["os"] = `"><svg onload=x>`
	svg := RenderSVG(report)
	wellFormed(t, svg)
	if strings.Contains(svg, "<script") || strings.Contains(svg, "<svg onload") || strings.ContainsRune(svg, 0) {
		t.Fatalf("unescaped content:\n%s", svg)
	}
}

func TestWriteReportWritesSVGAndMarkdownSections(t *testing.T) {
	dir := t.TempDir()
	report := roiReport([]uint64{2000, 2100, 1900}, []uint64{1000, 1100, 900})
	if err := WriteReport(dir, report); err != nil {
		t.Fatal(err)
	}
	svg, err := os.ReadFile(filepath.Join(dir, "latest.svg"))
	if err != nil {
		t.Fatal(err)
	}
	wellFormed(t, string(svg))
	markdown, _ := os.ReadFile(filepath.Join(dir, "latest.md"))
	for _, expected := range []string{"## Uncertainty", "coefficient of variation", "No confidence intervals", "## ROI", "No ROI is embedded", "latest.svg", "Measured ratio of median durations"} {
		if !strings.Contains(string(markdown), expected) {
			t.Errorf("Markdown missing %q", expected)
		}
	}
}

func TestMarkdownUncertaintyWithoutMeasurementsInfersNothing(t *testing.T) {
	report := roiReport([]uint64{1000, 1000, 1000}, []uint64{500, 500, 500})
	report.Measurements = []Measurement{{Name: "Go baseline", Status: "failed", ErrorCount: 3}}
	markdown := RenderMarkdown(report)
	if !strings.Contains(markdown, "No valid measurements are available") || strings.Contains(markdown, "Measured ratio") {
		t.Fatalf("unexpected:\n%s", markdown)
	}
}

func TestRenderSVGCaptionMatchesTimingMode(t *testing.T) {
	report := roiReport([]uint64{2_000_000, 2_000_000, 2_000_000}, []uint64{1_000_000, 1_000_000, 1_000_000})
	report.Environment = map[string]string{"timing_mode": TimingContainerRun}
	fresh := RenderSVG(report)
	if !strings.Contains(fresh, "fresh-container process time") || strings.Contains(fresh, "whole-process") {
		t.Fatal("fresh-container caption missing or mixed with end-to-end wording")
	}

	report.Environment = map[string]string{"timing_mode": TimingSteadyState}
	legacy := RenderSVG(report)
	if !strings.Contains(legacy, "legacy EPOCHREALTIME process time") || strings.Contains(legacy, "whole-process") {
		t.Fatal("legacy timing caption missing or mixed with end-to-end wording")
	}

	report.Environment = map[string]string{}
	if end := RenderSVG(report); !strings.Contains(end, "whole-process wall-clock") || strings.Contains(end, "in-container") {
		t.Fatal("end-to-end caption missing or mixed with steady-state wording")
	}
}

func TestRenderSVGTruncatesCombinedLongTitle(t *testing.T) {
	report := roiReport([]uint64{2_000_000, 2_000_000, 2_000_000}, []uint64{1_000_000, 1_000_000, 1_000_000})
	report.Project = strings.Repeat("LongProjectName", 8)
	report.Component = strings.Repeat("LongComponentLabel", 8)
	svg := RenderSVG(report)
	wellFormed(t, svg)
	titleStart := strings.Index(svg, `class="title">`)
	if titleStart < 0 {
		t.Fatal("title missing")
	}

	titleStart += len(`class="title">`)
	titleEnd := strings.Index(svg[titleStart:], "</text>")
	if titleEnd < 0 {
		t.Fatal("title not closed")
	}
	title := svg[titleStart : titleStart+titleEnd]
	if !strings.HasSuffix(title, "…") || len([]rune(title)) > 62 {
		t.Fatalf("combined title should be visibly ellipsized to a bounded width: %q", title)
	}
}

func TestRenderSVGShowsMeasurementQualityWarning(t *testing.T) {
	report := roiReport([]uint64{900_000, 900_000, 900_000}, []uint64{900_000, 900_000, 900_000})
	for i := range report.Measurements {
		for sample := range report.Measurements[i].Samples {
			report.Measurements[i].Samples[sample].TimerResolutionNS = 1_000
		}
		if err := summarizeMeasurement(&report.Measurements[i]); err != nil {
			t.Fatal(err)
		}
	}
	svg := RenderSVG(report)
	wellFormed(t, svg)
	if !strings.Contains(svg, "1,000-tick quality screen") ||
		!strings.Contains(svg, "n/a — invalid") ||
		strings.Contains(svg, "Measured throughput delta") {
		t.Fatalf("SVG must surface quality warnings and omit invalid comparison values:\n%s", svg)
	}
}
