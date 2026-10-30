package rustrush

import (
	"fmt"
	"sort"
	"strings"
)

// uncertaintySection describes only what the raw samples show. It never
// estimates a confidence interval; the verdict thresholds are stated verbatim.
func uncertaintySection(report Report) string {
	var out strings.Builder
	out.WriteString("\n## Uncertainty\n\n")
	var baseline, candidate Measurement
	baselineName := baselineMeasurementName(report.Environment["baseline_language"])
	count := 0
	for _, measurement := range report.Measurements {
		if measurement.Status != "measured" || !validMeasurement(measurement) {
			continue
		}
		count++
		durations := sampleDurations(measurement.Samples)
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		fmt.Fprintf(&out, "- **%s:** %d trials, min %.3f ms, median %.3f ms, max %.3f ms, coefficient of variation %.1f%% (verdict requires at most 10%%).\n",
			measurement.Name, len(durations), float64(durations[0])/1e6, float64(measurement.MedianNS)/1e6,
			float64(durations[len(durations)-1])/1e6, measurement.CoefficientOfVariation*100)
		switch measurement.Name {
		case baselineName:
			baseline = measurement
		case "Rust candidate":
			candidate = measurement
		}
	}
	if count == 0 {
		out.WriteString("No valid measurements are available, so no uncertainty can be reported and nothing is inferred.\n")
		return out.String()
	}
	if report.Verdict != VerdictInconclusive && baseline.MedianNS > 0 && candidate.MedianNS > 0 {
		fmt.Fprintf(&out, "\nMeasured ratio of median durations (%s / Rust candidate): %.3f. Values above 1 mean the candidate's median process time was shorter in this run.\n",
			baselineName,
			float64(baseline.MedianNS)/float64(candidate.MedianNS))
	}
	out.WriteString("\nNo confidence intervals are computed. A handful of trials on one machine cannot establish statistical significance; repeat the run and compare before acting.\n")
	return out.String()
}
