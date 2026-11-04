package rustrush

import (
	"fmt"
	"strings"
)

// Conclusion is the beginner-facing outcome of a report. It is presentation
// only: it never changes a report's internal verdict, thresholds, or schema.
type Conclusion string

const (
	ConclusionFaster        Conclusion = "RUST_APPEARS_FASTER"
	ConclusionNoImprovement Conclusion = "NO_IMPROVEMENT_DEMONSTRATED"
	ConclusionInsufficient  Conclusion = "NOT_ENOUGH_EVIDENCE"
)

// InsufficientCause names why a report could not support a comparison.
type InsufficientCause string

const (
	CauseNone                 InsufficientCause = ""
	CauseCorrectnessFailed    InsufficientCause = "correctness_failed"
	CauseTimeout              InsufficientCause = "timeout"
	CauseBuildTimeout         InsufficientCause = "build_timeout"
	CauseTimeoutUnknownPhase  InsufficientCause = "timeout_unknown_phase"
	CauseRustBuildFailed      InsufficientCause = "rust_build_failed"
	CauseBaselineBuildFailed  InsufficientCause = "baseline_build_failed"
	CauseBuildFailed          InsufficientCause = "build_failed"
	CauseExecutionFailed      InsufficientCause = "execution_failed"
	CauseRustUnavailable      InsufficientCause = "rust_unavailable"
	CauseRunsTooShort         InsufficientCause = "runs_too_short"
	CauseNoisyMeasurements    InsufficientCause = "noisy_measurements"
	CauseComparisonIncomplete InsufficientCause = "comparison_incomplete"
)

const summaryLimitation = "This measures only the tested workload on this machine. It does not measure your whole backend."

// ResultSummary is a plain-English reading of a Report, suitable for terminal
// output, Markdown, and other front ends.
type ResultSummary struct {
	Conclusion  Conclusion
	Cause       InsufficientCause
	Headline    string
	Explanation string
	NextStep    string
	Limitation  string
}

// Text renders the summary as plain lines.
func (s ResultSummary) Text() string {
	return fmt.Sprintf("%s\n%s\nNext step: %s\nKeep in mind: %s\n", s.Headline, s.Explanation, s.NextStep, s.Limitation)
}

// SummarizeResult interprets a report in plain English. The verdict is
// re-derived from the raw measurements with the existing Decide logic, so a
// stored PORT, OPTIMIZE_FIRST, or DONT_PORT verdict is only reported when the
// evidence still supports it. Anything that cannot be mapped reliably is
// reported as insufficient evidence.
func SummarizeResult(report Report) ResultSummary {
	checked := report
	if checked.Verdict != VerdictInconclusive {
		normalizeReportEvidence(&checked)
	}
	switch checked.Verdict {
	case VerdictPort:
		return summarizeFaster(checked)
	case VerdictOptimizeFirst:
		return summarizeNoImprovement(checked, false)
	case VerdictDontPort:
		return summarizeNoImprovement(checked, true)
	default:
		return summarizeInsufficient(checked)
	}
}

func baselineDisplayName(report Report) string {
	return baselineLabel(normalizeBaselineLanguage(report.Environment["baseline_language"]))
}

func summarizeFaster(report Report) ResultSummary {
	explanation := "Rust produced the same output as your " + baselineDisplayName(report) + " code and ran more work per second in repeated, consistent measurements."
	if report.ThroughputDeltaPercent != nil {
		explanation = fmt.Sprintf("Rust produced the same output as your %s code and completed about %.1f%% more runs per second in repeated, consistent measurements.",
			baselineDisplayName(report), *report.ThroughputDeltaPercent)
	}
	return ResultSummary{
		Conclusion:  ConclusionFaster,
		Headline:    "Rust appears faster for this workload.",
		Explanation: explanation + " This is not a guarantee that a rewrite will be faster.",
		NextStep:    "Repeat the run on the hardware you deploy on, and test a realistic version of your workload before deciding to rewrite.",
		Limitation:  summaryLimitation,
	}
}

func summarizeNoImprovement(report Report, slower bool) ResultSummary {
	summary := ResultSummary{
		Conclusion: ConclusionNoImprovement,
		Headline:   "No improvement was demonstrated.",
		Limitation: summaryLimitation,
	}
	if slower {
		summary.Explanation = "Rust produced the same output as your " + baselineDisplayName(report) + " code but was slower in repeated, consistent measurements"
		if report.ThroughputDeltaPercent != nil {
			summary.Explanation += fmt.Sprintf(" (about %.1f%% fewer runs per second)", -*report.ThroughputDeltaPercent)
		}
		summary.Explanation += "."
		summary.NextStep = "This workload does not support a rewrite. If you still suspect Rust could help, try measuring a different part of your code."
		return summary
	}
	summary.Explanation = "Rust produced the same output as your " + baselineDisplayName(report) + " code, but the speed difference was too small to count as an improvement."
	if report.ThroughputDeltaPercent != nil {
		summary.Explanation = fmt.Sprintf("Rust produced the same output as your %s code, but the speed difference (%+.1f%%) was too small to count as an improvement.",
			baselineDisplayName(report), *report.ThroughputDeltaPercent)
	}
	summary.NextStep = "Look for improvements in your current code and measure again before considering a rewrite."
	return summary
}

func summarizeInsufficient(report Report) ResultSummary {
	cause, subject := insufficientCause(report)
	summary := ResultSummary{
		Conclusion: ConclusionInsufficient,
		Cause:      cause,
		Headline:   "Not enough evidence.",
		Limitation: summaryLimitation,
	}
	language := baselineDisplayName(report)
	switch cause {
	case CauseCorrectnessFailed:
		summary.Explanation = "The Rust program and your " + language + " program did not produce the same output, so no speed comparison was made. A faster program that gives different answers is not a valid comparison."
		summary.NextStep = "Make both programs print identical output for the same input, then rerun."
	case CauseTimeout:
		summary.Explanation = subject + " timed out while running, so the comparison could not be completed."
		summary.NextStep = "Check that the program finishes, or rerun with a smaller --iterations value or a longer --timeout."
	case CauseBuildTimeout:
		summary.Explanation = "Building or preparing " + strings.ToLower(subject[:1]) + subject[1:] + " timed out, so the comparison could not be completed."
		summary.NextStep = "Check that the compiler or interpreter is set up correctly and that the program builds on its own, or rerun with a longer --timeout."
	case CauseTimeoutUnknownPhase:
		summary.Explanation = "An operation timed out (" + strings.ToLower(subject[:1]) + subject[1:] + "), and the report does not show whether it was building or running, so the comparison could not be completed."
		summary.NextStep = "Rerun with a longer --timeout and read the error details in the report."
	case CauseRustBuildFailed:
		summary.Explanation = "The Rust program did not compile, so there was nothing to compare."
		summary.NextStep = "Fix the compiler errors shown in the report, then rerun."
	case CauseBaselineBuildFailed:
		summary.Explanation = "Your " + language + " program could not be built or prepared, so there was nothing to compare."
		summary.NextStep = "Fix the " + language + " errors shown in the report, then rerun."
	case CauseBuildFailed:
		summary.Explanation = "A program could not be built or prepared, and the report does not show which one, so there was nothing to compare."
		summary.NextStep = "Read the error details in the report, fix the program that failed to build, then rerun."
	case CauseExecutionFailed:
		summary.Explanation = subject + " failed while running, so the comparison could not be completed."
		summary.NextStep = "Run the failing program by itself to see its error, fix it, then rerun."
	case CauseRustUnavailable:
		summary.Explanation = "Rust is not installed, so only your " + language + " code was measured and nothing could be compared."
		summary.NextStep = "Install Rust from https://rustup.rs/ and rerun."
	case CauseRunsTooShort:
		summary.Explanation = "Each run was too short to time reliably, so the numbers cannot support a conclusion."
		summary.NextStep = "Rerun with a larger --iterations value so each run takes longer."
	case CauseNoisyMeasurements:
		summary.Explanation = "Timings varied too much from run to run to trust a comparison."
		summary.NextStep = "Rerun the benchmark, and consider a larger --iterations or --repetitions value. Only trust a result once repeated runs agree."
	default:
		summary.Cause = CauseComparisonIncomplete
		summary.Explanation = "The report does not contain complete, valid measurements for both programs, so no comparison can be made."
		summary.NextStep = "Rerun the benchmark and read the limitations section of the report for details."
	}
	return summary
}

// insufficientCause picks the most specific reason from the report's evidence.
// The returned subject names the measurement that caused timeout and failure causes.
func insufficientCause(report Report) (InsufficientCause, string) {
	if report.Correctness == "FAILED" {
		return CauseCorrectnessFailed, ""
	}
	if name := firstMeasurementWithStatus(report, "correctness_failed"); name != "" {
		return CauseCorrectnessFailed, ""
	}
	if outputsDisagree(report) {
		return CauseCorrectnessFailed, ""
	}
	if cause, subject, ok := timeoutCause(report); ok {
		return cause, subject
	}
	if cause, ok := buildFailureCause(report); ok {
		return cause, ""
	}
	if name := firstMeasurementWithStatus(report, "failed"); name != "" {
		return CauseExecutionFailed, subjectName(name)
	}
	for _, measurement := range report.Measurements {
		if measurement.ErrorCount > 0 {
			return CauseExecutionFailed, subjectName(measurement.Name)
		}
	}
	if firstMeasurementWithStatus(report, "unavailable") != "" {
		return CauseRustUnavailable, ""
	}
	if report.Correctness != "PASSED_FOR_CHECKSUM" {
		return CauseComparisonIncomplete, ""
	}
	measured := 0
	warned := false
	noisy := false
	for _, measurement := range report.Measurements {
		if measurement.Status != "measured" {
			continue
		}
		measured++
		if len(measurement.Warnings) > 0 {
			warned = true
		}
		if len(measurement.Samples) >= 2 && coefficientOfVariation(measurement.Samples) > 0.10 {
			noisy = true
		}
	}
	if measured >= 2 {
		if warned {
			return CauseRunsTooShort, ""
		}
		if noisy {
			return CauseNoisyMeasurements, ""
		}
	}
	return CauseComparisonIncomplete, ""
}

// outputsDisagree reports whether the baseline and Rust measurements both
// recorded output and those outputs differ. Absent output proves nothing.
func outputsDisagree(report Report) bool {
	var baseline, candidate *Measurement
	baselineName := baselineDisplayName(report) + " baseline"
	for i := range report.Measurements {
		switch report.Measurements[i].Name {
		case baselineName:
			baseline = &report.Measurements[i]
		case "Rust candidate":
			candidate = &report.Measurements[i]
		}
	}
	if baseline == nil || candidate == nil {
		return false
	}
	baselineOutputs, candidateOutputs := recordedOutputs(*baseline), recordedOutputs(*candidate)
	if len(baselineOutputs) == 0 || len(candidateOutputs) == 0 {
		return false
	}
	if len(baselineOutputs) != len(candidateOutputs) {
		return true
	}
	for output := range baselineOutputs {
		if !candidateOutputs[output] {
			return true
		}
	}
	return false
}

func recordedOutputs(measurement Measurement) map[string]bool {
	outputs := map[string]bool{}
	if measurement.CorrectnessOutput != "" {
		outputs[measurement.CorrectnessOutput] = true
	}
	for _, sample := range measurement.Samples {
		if sample.Output != "" {
			outputs[sample.Output] = true
		}
	}
	return outputs
}

type measurementSide int

const (
	sideUnknown measurementSide = iota
	sideBaseline
	sideRust
)

func sideOf(name string) measurementSide {
	switch {
	case name == "Rust candidate":
		return sideRust
	case strings.HasSuffix(name, " baseline") && strings.TrimSpace(strings.TrimSuffix(name, " baseline")) != "":
		return sideBaseline
	default:
		return sideUnknown
	}
}

// buildFailureCause attributes a build failure to a side only when exactly one
// side recorded it.
func buildFailureCause(report Report) (InsufficientCause, bool) {
	var sides []measurementSide
	for _, measurement := range report.Measurements {
		if measurement.Status == "build_failed" {
			sides = append(sides, sideOf(measurement.Name))
		}
	}
	if len(sides) == 0 {
		return CauseNone, false
	}
	for _, side := range sides[1:] {
		if side != sides[0] {
			return CauseBuildFailed, true
		}
	}
	switch sides[0] {
	case sideRust:
		return CauseRustBuildFailed, true
	case sideBaseline:
		return CauseBaselineBuildFailed, true
	}
	return CauseBuildFailed, true
}

// timeoutCause distinguishes build/preparation timeouts from workload timeouts
// using existing report fields: a measurement that never reached the trial
// phase has no repetitions, warm-up, or samples, while a run-phase timeout
// comes from a measurement configured with repetitions.
func timeoutCause(report Report) (InsufficientCause, string, bool) {
	var timedOut []Measurement
	for _, measurement := range report.Measurements {
		if measurement.Status == "timed_out" {
			timedOut = append(timedOut, measurement)
		}
	}
	if len(timedOut) == 0 {
		return CauseNone, "", false
	}
	phase := func(m Measurement) InsufficientCause {
		switch {
		case m.Repetitions > 0:
			return CauseTimeout
		case len(m.Samples) == 0 && m.WarmupRuns == 0:
			return CauseBuildTimeout
		}
		return CauseTimeoutUnknownPhase
	}
	cause := phase(timedOut[0])
	subject := subjectName(timedOut[0].Name)
	for _, measurement := range timedOut[1:] {
		if phase(measurement) != cause {
			cause = CauseTimeoutUnknownPhase
		}
		if measurement.Name != timedOut[0].Name {
			subject = "One of the programs"
		}
	}
	return cause, subject, true
}

func firstMeasurementWithStatus(report Report, status string) string {
	for _, measurement := range report.Measurements {
		if measurement.Status == status {
			return measurement.Name
		}
	}
	return ""
}

func subjectName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "One of the programs"
	}
	if name == "Rust candidate" {
		return "The Rust program"
	}
	return "The " + strings.TrimSuffix(name, " baseline") + " program"
}
