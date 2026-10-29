package rustrush

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

const svgWidth = 760

func svgText(value string) string {
	var buffer bytes.Buffer
	if err := xml.EscapeText(&buffer, []byte(sanitizeSVGText(value))); err != nil {
		return ""
	}
	return buffer.String()
}

// sanitizeSVGText drops control characters that are invalid in XML 1.0.
func sanitizeSVGText(value string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' || r < 0x20 || r == 0xFFFE || r == 0xFFFF {
			return ' '
		}
		return r
	}, value)
}

func truncateText(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit-1]) + "…"
}

func verdictColor(verdict Verdict) string {
	switch verdict {
	case VerdictPort:
		return "#1a7f37"
	case VerdictDontPort:
		return "#cf222e"
	case VerdictOptimizeFirst:
		return "#9a6700"
	default:
		return "#57606a"
	}
}

// RenderSVG renders a self-contained report card. Every number comes from the
// report's measurements; missing or invalid measurements are labelled n/a and
// never drawn as zero-length bars.
func RenderSVG(report Report) string {
	normalizeReportEvidence(&report)
	var rows []string
	y := 150
	maxP50 := uint64(0)
	valid := map[string]bool{}
	for _, measurement := range report.Measurements {
		if measurement.Status == "measured" && validMeasurement(measurement) {
			valid[measurement.Name] = true
			if measurement.MedianNS > maxP50 {
				maxP50 = measurement.MedianNS
			}
		}
	}
	for _, measurement := range report.Measurements {
		rows = append(rows, fmt.Sprintf(`<text x="24" y="%d" class="label">%s</text>`, y, svgText(measurement.Name)))
		if !valid[measurement.Name] {
			status := measurement.Status
			if status == "measured" || status == "" {
				status = "invalid"
			}
			detail := fmt.Sprintf("n/a — %s (%d errors)", status, measurement.ErrorCount)
			if len(measurement.Warnings) > 0 {
				detail += ": " + strings.Join(measurement.Warnings, "; ")
			}
			rows = append(rows, fmt.Sprintf(`<text x="190" y="%d" class="muted">%s</text>`, y, svgText(truncateText(detail, 80))))
			y += 56
			continue
		}
		width := 1.0
		if maxP50 > 0 {
			width = float64(measurement.MedianNS) / float64(maxP50) * 380
		}
		if width < 1 {
			width = 1
		}
		rows = append(rows,
			fmt.Sprintf(`<rect x="190" y="%d" width="%.1f" height="18" fill="#0969da"/>`, y-14, width),
			fmt.Sprintf(`<text x="%.1f" y="%d" class="value">p50 %.3f ms</text>`, 190+width+8, y, float64(measurement.MedianNS)/1e6),
			fmt.Sprintf(`<text x="190" y="%d" class="muted">p95 %.3f ms · p99 %.3f ms · %.2f ops/s · CV %.3f · %d trials · %d errors</text>`,
				y+20, float64(measurement.P95NS)/1e6, float64(measurement.P99NS)/1e6, measurement.OpsPerSec,
				measurement.CoefficientOfVariation, measurement.Repetitions, measurement.ErrorCount))
		y += 56
	}

	delta := "Throughput delta: n/a (no conclusive comparison)"
	if report.ThroughputDeltaPercent != nil {
		delta = fmt.Sprintf("Measured throughput delta (Rust vs Go): %+.2f%%", *report.ThroughputDeltaPercent)
	}
	meta := []string{}
	for _, key := range []string{"os", "architecture", "cpu_count", "go_runtime", "rustc", "isolation"} {
		if value := report.Environment[key]; value != "" {
			meta = append(meta, key+"="+value)
		}
	}
	height := y + 150
	var out strings.Builder
	fmt.Fprintf(&out, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="RustRush benchmark report">`+"\n", svgWidth, height, svgWidth, height)
	out.WriteString(`<style>text{font-family:Segoe UI,Helvetica,Arial,sans-serif;fill:#24292f}.title{font-size:20px;font-weight:600}.label{font-size:14px;font-weight:600}.value{font-size:13px}.muted{font-size:12px;fill:#57606a}.badge{font-size:14px;font-weight:700;fill:#fff}</style>` + "\n")
	fmt.Fprintf(&out, `<rect width="%d" height="%d" fill="#ffffff" stroke="#d0d7de"/>`+"\n", svgWidth, height)
	title := truncateText(fmt.Sprintf("RustRush: %s / %s", report.Project, report.Component), 62)
	fmt.Fprintf(&out, `<text x="24" y="38" class="title">%s</text>`+"\n", svgText(title))
	fmt.Fprintf(&out, `<rect x="24" y="52" width="190" height="28" rx="6" fill="%s"/><text x="36" y="71" class="badge">%s</text>`+"\n", verdictColor(report.Verdict), svgText(string(report.Verdict)))
	fmt.Fprintf(&out, `<text x="230" y="71" class="value">Correctness gate: %s</text>`+"\n", svgText(report.Correctness))
	fmt.Fprintf(&out, `<text x="24" y="104" class="muted">Workload: %s · %d iterations per process</text>`+"\n", svgText(truncateText(report.Workload, 80)), report.Iterations)
	scope := "Median whole-process wall-clock time per run (includes process start-up)"
	if timingMode(report) == TimingContainerRun {
		scope = "Median fresh-container process time (excludes container lifecycle)"
	} else if timingMode(report) == TimingSteadyState {
		scope = "Median legacy EPOCHREALTIME process time (includes workload startup)"
	}
	fmt.Fprintf(&out, `<text x="24" y="124" class="muted">%s (shorter is faster); measured in this run.</text>`+"\n", svgText(scope))
	for _, row := range rows {
		out.WriteString(row + "\n")
	}
	fmt.Fprintf(&out, `<text x="24" y="%d" class="value">%s</text>`+"\n", y+10, svgText(delta))
	fmt.Fprintf(&out, `<text x="24" y="%d" class="muted">Reason: %s</text>`+"\n", y+34, svgText(truncateText(report.Reason, 110)))
	fmt.Fprintf(&out, `<text x="24" y="%d" class="muted">%s</text>`+"\n", y+58, svgText(truncateText(strings.Join(meta, " · "), 120)))
	fmt.Fprintf(&out, `<text x="24" y="%d" class="muted">Process-level timings, not per-request latency. Schema %s, generated %s.</text>`+"\n", y+82, svgText(report.SchemaVersion), svgText(report.GeneratedAt.UTC().Format("2006-01-02T15:04:05Z")))
	out.WriteString("</svg>\n")
	return out.String()
}
