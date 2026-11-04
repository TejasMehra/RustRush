package rustrush

import (
	"strings"
	"testing"
)

func summaryReport(language string, correctness string, verdict Verdict, baseline, candidate Measurement) Report {
	report := Report{
		SchemaVersion: SchemaVersion,
		Project:       "test",
		Component:     "test",
		Correctness:   correctness,
		Verdict:       verdict,
		Environment:   map[string]string{},
		Measurements:  []Measurement{baseline, candidate},
	}
	if language != "" {
		report.Environment["baseline_language"] = language
	}
	return report
}

func goodPair(baselineDurations, candidateDurations []uint64) (Measurement, Measurement) {
	return testMeasurement("Go baseline", baselineDurations, 10), testMeasurement("Rust candidate", candidateDurations, 10)
}

func assertSummaryBasics(t *testing.T, summary ResultSummary) {
	t.Helper()
	if summary.Headline == "" || summary.Explanation == "" || summary.NextStep == "" || summary.Limitation == "" {
		t.Fatalf("summary has empty fields: %+v", summary)
	}
	if !strings.Contains(summary.Limitation, "this machine") || !strings.Contains(summary.Limitation, "whole backend") {
		t.Fatalf("limitation missing: %q", summary.Limitation)
	}
	lower := strings.ToLower(summary.Text())
	for _, banned := range []string{"guarantee that rust is faster", "always faster", "p99", "coefficient"} {
		if strings.Contains(lower, banned) {
			t.Fatalf("summary contains %q: %s", banned, summary.Text())
		}
	}
}

func TestSummarizeResultMapsEveryInternalVerdict(t *testing.T) {
	cases := []struct {
		name      string
		candidate []uint64
		verdict   Verdict
		want      Conclusion
		headline  string
	}{
		{"port", []uint64{80_000_000, 81_000_000, 79_000_000}, VerdictPort, ConclusionFaster, "Rust appears faster for this workload."},
		{"optimize first", []uint64{99_000_000, 100_000_000, 101_000_000}, VerdictOptimizeFirst, ConclusionNoImprovement, "No improvement was demonstrated."},
		{"dont port", []uint64{120_000_000, 121_000_000, 119_000_000}, VerdictDontPort, ConclusionNoImprovement, "No improvement was demonstrated."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseline, candidate := goodPair([]uint64{100_000_000, 101_000_000, 99_000_000}, tc.candidate)
			report := summaryReport("", "PASSED_FOR_CHECKSUM", tc.verdict, baseline, candidate)
			summary := SummarizeResult(report)
			if summary.Conclusion != tc.want || summary.Headline != tc.headline || summary.Cause != CauseNone {
				t.Fatalf("got %+v", summary)
			}
			assertSummaryBasics(t, summary)
			if report.Verdict != tc.verdict {
				t.Fatal("SummarizeResult mutated the report verdict")
			}
		})
	}
	summary := SummarizeResult(Report{Verdict: VerdictInconclusive, Environment: map[string]string{}})
	if summary.Conclusion != ConclusionInsufficient || summary.Headline != "Not enough evidence." {
		t.Fatalf("got %+v", summary)
	}
	assertSummaryBasics(t, summary)
}

func TestSummarizeResultSuccessfulComparisonStatesMeasuredGainWithoutGuarantee(t *testing.T) {
	baseline, candidate := goodPair([]uint64{100_000_000, 101_000_000, 99_000_000}, []uint64{50_000_000, 50_500_000, 49_500_000})
	baseline.Name = "Python baseline"
	report := summaryReport("python", "PASSED_FOR_CHECKSUM", VerdictPort, baseline, candidate)
	summary := SummarizeResult(report)
	if !strings.Contains(summary.Explanation, "Python") || !strings.Contains(summary.Explanation, "%") ||
		!strings.Contains(summary.Explanation, "not a guarantee") {
		t.Fatalf("explanation lacks measured detail or caveat: %s", summary.Explanation)
	}
}

func TestSummarizeResultNegativeAndNegligibleResults(t *testing.T) {
	baseline, slower := goodPair([]uint64{100_000_000, 101_000_000, 99_000_000}, []uint64{120_000_000, 121_000_000, 119_000_000})
	summary := SummarizeResult(summaryReport("", "PASSED_FOR_CHECKSUM", VerdictDontPort, baseline, slower))
	if !strings.Contains(summary.Explanation, "slower") || !strings.Contains(summary.NextStep, "does not support a rewrite") {
		t.Fatalf("slower result not explained: %+v", summary)
	}
	baseline, similar := goodPair([]uint64{100_000_000, 101_000_000, 99_000_000}, []uint64{99_000_000, 100_000_000, 101_000_000})
	summary = SummarizeResult(summaryReport("", "PASSED_FOR_CHECKSUM", VerdictOptimizeFirst, baseline, similar))
	if !strings.Contains(summary.Explanation, "too small") {
		t.Fatalf("negligible result not explained: %+v", summary)
	}
}

func TestSummarizeResultDoesNotTrustStoredVerdictWithoutEvidence(t *testing.T) {
	baseline, candidate := goodPair([]uint64{100_000_000, 101_000_000, 99_000_000}, []uint64{80_000_000, 81_000_000, 79_000_000})

	failedGate := summaryReport("", "FAILED", VerdictPort, baseline, candidate)
	if summary := SummarizeResult(failedGate); summary.Conclusion != ConclusionInsufficient || summary.Cause != CauseCorrectnessFailed {
		t.Fatalf("stored PORT with failed correctness was presented as a win: %+v", summary)
	}

	tampered := candidate
	tampered.P95NS++
	if summary := SummarizeResult(summaryReport("", "PASSED_FOR_CHECKSUM", VerdictPort, baseline, tampered)); summary.Conclusion != ConclusionInsufficient {
		t.Fatalf("inconsistent measurements were presented as a win: %+v", summary)
	}

	incomplete := candidate
	incomplete.Samples = incomplete.Samples[:2]
	if summary := SummarizeResult(summaryReport("", "PASSED_FOR_CHECKSUM", VerdictPort, baseline, incomplete)); summary.Conclusion != ConclusionInsufficient {
		t.Fatalf("incomplete measurements were presented as a win: %+v", summary)
	}

	noMeasurements := Report{Verdict: VerdictPort, Correctness: "PASSED_FOR_CHECKSUM", Environment: map[string]string{}}
	if summary := SummarizeResult(noMeasurements); summary.Conclusion != ConclusionInsufficient || summary.Cause != CauseComparisonIncomplete {
		t.Fatalf("missing measurements were not insufficient: %+v", summary)
	}
}

func TestSummarizeResultInconclusiveCauses(t *testing.T) {
	baseline, candidate := goodPair([]uint64{100_000_000, 101_000_000, 99_000_000}, []uint64{80_000_000, 81_000_000, 79_000_000})
	unavailable := Measurement{Name: "Rust candidate", Status: "unavailable", Error: "rustc was not found on PATH"}

	t.Run("correctness failed", func(t *testing.T) {
		failedBaseline, failedCandidate := baseline, candidate
		failedBaseline.Status, failedCandidate.Status = "correctness_failed", "correctness_failed"
		summary := SummarizeResult(summaryReport("nodejs", "FAILED", VerdictInconclusive, failedBaseline, failedCandidate))
		if summary.Cause != CauseCorrectnessFailed || !strings.Contains(summary.Explanation, "did not produce the same output") ||
			!strings.Contains(summary.Explanation, "Node.js") || !strings.Contains(summary.NextStep, "identical output") {
			t.Fatalf("got %+v", summary)
		}
		assertSummaryBasics(t, summary)
	})
	t.Run("correctness failed is not hidden by other failures", func(t *testing.T) {
		timedOut := Measurement{Name: "Rust candidate", Status: "timed_out", ErrorCount: 1}
		summary := SummarizeResult(summaryReport("", "FAILED", VerdictInconclusive, baseline, timedOut))
		if summary.Cause != CauseCorrectnessFailed {
			t.Fatalf("got %+v", summary)
		}
	})
	t.Run("rust unavailable", func(t *testing.T) {
		summary := SummarizeResult(summaryReport("", "NOT_RUN", VerdictInconclusive, baseline, unavailable))
		if summary.Cause != CauseRustUnavailable || !strings.Contains(summary.Explanation, "Rust is not installed") ||
			!strings.Contains(summary.NextStep, "rustup.rs") || !strings.Contains(summary.Explanation, "Go code") {
			t.Fatalf("got %+v", summary)
		}
		assertSummaryBasics(t, summary)
	})
	t.Run("baseline failed outranks rust unavailable", func(t *testing.T) {
		failed := Measurement{Name: "Python baseline", Status: "failed", ErrorCount: 1}
		summary := SummarizeResult(summaryReport("python", "NOT_RUN", VerdictInconclusive, failed, unavailable))
		if summary.Cause != CauseExecutionFailed || !strings.Contains(summary.Explanation, "The Python program failed") {
			t.Fatalf("got %+v", summary)
		}
	})
	t.Run("timeout names the program", func(t *testing.T) {
		timedOut := Measurement{Name: "Rust candidate", Status: "timed_out", ErrorCount: 1, Repetitions: 3}
		summary := SummarizeResult(summaryReport("", "PASSED_FOR_CHECKSUM", VerdictInconclusive, baseline, timedOut))
		if summary.Cause != CauseTimeout || !strings.HasPrefix(summary.Explanation, "The Rust program timed out") ||
			!strings.Contains(summary.NextStep, "--timeout") || !strings.Contains(summary.NextStep, "--iterations") {
			t.Fatalf("got %+v", summary)
		}
		assertSummaryBasics(t, summary)
	})
	t.Run("build timeout never suggests fewer iterations", func(t *testing.T) {
		timedOut := Measurement{Name: "Rust candidate", Status: "timed_out", ErrorCount: 1, Samples: []Sample{}}
		notRun := Measurement{Name: "Go baseline", Status: "not_run"}
		summary := SummarizeResult(summaryReport("", "NOT_RUN", VerdictInconclusive, notRun, timedOut))
		if summary.Cause != CauseBuildTimeout || strings.Contains(summary.NextStep, "--iterations") ||
			!strings.Contains(summary.NextStep, "--timeout") || !strings.Contains(summary.NextStep, "compiler") {
			t.Fatalf("got %+v", summary)
		}
		assertSummaryBasics(t, summary)
	})
	t.Run("ambiguous timeout does not guess the phase", func(t *testing.T) {
		timedOut := Measurement{Name: "Rust candidate", Status: "timed_out", ErrorCount: 1, WarmupRuns: 1}
		summary := SummarizeResult(summaryReport("", "NOT_RUN", VerdictInconclusive, baseline, timedOut))
		text := strings.ToLower(summary.Explanation + summary.NextStep)
		if summary.Cause != CauseTimeoutUnknownPhase || !strings.Contains(text, "timed out") ||
			strings.Contains(summary.NextStep, "--iterations") || strings.Contains(text, "compil") {
			t.Fatalf("got %+v", summary)
		}
		assertSummaryBasics(t, summary)
	})
	t.Run("timer preparation failure names the selected baseline", func(t *testing.T) {
		for _, tc := range []struct{ language, name, program string }{
			{"go", "Go baseline", "Go"},
			{"python", "Python baseline", "Python"},
			{"nodejs", "Node.js baseline", "Node.js"},
		} {
			for _, status := range []string{"failed", "timed_out"} {
				prep := Measurement{Name: tc.name, Status: status, ErrorCount: 1, Samples: []Sample{}}
				notRun := Measurement{Name: "Rust candidate", Status: "not_run", Samples: []Sample{}}
				summary := SummarizeResult(summaryReport(tc.language, "NOT_RUN", VerdictInconclusive, prep, notRun))
				text := summary.Text()
				if summary.Conclusion != ConclusionInsufficient || !strings.Contains(text, "The "+tc.program+" program") && !strings.Contains(text, "the "+tc.program+" program") {
					t.Fatalf("%s/%s: summary must name the %s program: %+v", tc.language, status, tc.program, summary)
				}
				if tc.language != "go" && strings.Contains(text, "Go ") {
					t.Fatalf("%s/%s: summary wrongly mentions Go: %s", tc.language, status, text)
				}
			}
		}
	})
	t.Run("baseline build failure", func(t *testing.T) {
		built := Measurement{Name: "Python baseline", Status: "build_failed", ErrorCount: 1}
		notRun := Measurement{Name: "Rust candidate", Status: "not_run"}
		summary := SummarizeResult(summaryReport("python", "NOT_RUN", VerdictInconclusive, built, notRun))
		if summary.Cause != CauseBaselineBuildFailed || !strings.Contains(summary.Explanation, "Python program could not be built") ||
			strings.Contains(summary.Text(), "Rust program did not compile") || strings.Contains(summary.NextStep, "Rust") {
			t.Fatalf("got %+v", summary)
		}
		assertSummaryBasics(t, summary)
	})
	t.Run("ambiguous build failure uses neutral wording", func(t *testing.T) {
		both1 := Measurement{Name: "Go baseline", Status: "build_failed", ErrorCount: 1}
		both2 := Measurement{Name: "Rust candidate", Status: "build_failed", ErrorCount: 1}
		summary := SummarizeResult(summaryReport("", "NOT_RUN", VerdictInconclusive, both1, both2))
		if summary.Cause != CauseBuildFailed || strings.Contains(summary.Explanation, "did not compile") {
			t.Fatalf("got %+v", summary)
		}
		unknown := Measurement{Name: "mystery", Status: "build_failed", ErrorCount: 1}
		summary = SummarizeResult(summaryReport("", "NOT_RUN", VerdictInconclusive, unknown, Measurement{Name: "Rust candidate", Status: "not_run"}))
		if summary.Cause != CauseBuildFailed {
			t.Fatalf("got %+v", summary)
		}
	})
	t.Run("mismatched outputs override false correctness status", func(t *testing.T) {
		mismatched := candidate
		mismatched.CorrectnessOutput = baseline.CorrectnessOutput + "-different"
		mismatched.Samples = append([]Sample(nil), candidate.Samples...)
		for i := range mismatched.Samples {
			mismatched.Samples[i].Output = mismatched.CorrectnessOutput
		}
		for _, stored := range []Verdict{VerdictInconclusive, VerdictPort} {
			summary := SummarizeResult(summaryReport("", "PASSED_FOR_CHECKSUM", stored, baseline, mismatched))
			if summary.Conclusion != ConclusionInsufficient || summary.Cause != CauseCorrectnessFailed ||
				!strings.Contains(summary.Explanation, "did not produce the same output") {
				t.Fatalf("stored %s: got %+v", stored, summary)
			}
			assertSummaryBasics(t, summary)
		}
	})
	t.Run("missing output evidence is not a mismatch", func(t *testing.T) {
		noOutput := candidate
		noOutput.CorrectnessOutput = ""
		noOutput.Samples = append([]Sample(nil), candidate.Samples...)
		for i := range noOutput.Samples {
			noOutput.Samples[i].Output = ""
		}
		summary := SummarizeResult(summaryReport("", "PASSED_FOR_CHECKSUM", VerdictInconclusive, baseline, noOutput))
		if summary.Cause != CauseComparisonIncomplete {
			t.Fatalf("got %+v", summary)
		}
	})
	t.Run("rust build failure", func(t *testing.T) {
		built := Measurement{Name: "Rust candidate", Status: "build_failed", ErrorCount: 1}
		notRun := Measurement{Name: "Go baseline", Status: "not_run"}
		summary := SummarizeResult(summaryReport("", "NOT_RUN", VerdictInconclusive, notRun, built))
		if summary.Cause != CauseRustBuildFailed || !strings.Contains(summary.Explanation, "did not compile") {
			t.Fatalf("got %+v", summary)
		}
	})
	t.Run("execution failure", func(t *testing.T) {
		failed := Measurement{Name: "Rust candidate", Status: "failed", ErrorCount: 1}
		summary := SummarizeResult(summaryReport("", "PASSED_FOR_CHECKSUM", VerdictInconclusive, baseline, failed))
		if summary.Cause != CauseExecutionFailed || !strings.Contains(summary.Explanation, "The Rust program failed") {
			t.Fatalf("got %+v", summary)
		}
	})
	t.Run("noisy measurements", func(t *testing.T) {
		noisy := testMeasurement("Rust candidate", []uint64{20_000_000, 100_000_000, 300_000_000}, 10)
		summary := SummarizeResult(summaryReport("", "PASSED_FOR_CHECKSUM", VerdictInconclusive, baseline, noisy))
		if summary.Cause != CauseNoisyMeasurements || !strings.Contains(summary.Explanation, "varied too much") ||
			!strings.Contains(summary.NextStep, "Rerun the benchmark") ||
			strings.Contains(strings.ToLower(summary.Text()), "other programs") {
			t.Fatalf("got %+v", summary)
		}
		assertSummaryBasics(t, summary)
	})
	t.Run("runs too short", func(t *testing.T) {
		short := testMeasurement("Go baseline", []uint64{999_000, 999_000, 999_000}, 10)
		shortRust := testMeasurement("Rust candidate", []uint64{999_000, 999_000, 999_000}, 10)
		for _, measurement := range []*Measurement{&short, &shortRust} {
			for i := range measurement.Samples {
				measurement.Samples[i].TimerResolutionNS = 1_000
			}
			if err := summarizeMeasurement(measurement); err != nil {
				t.Fatal(err)
			}
		}
		summary := SummarizeResult(summaryReport("", "PASSED_FOR_CHECKSUM", VerdictInconclusive, short, shortRust))
		if summary.Cause != CauseRunsTooShort || !strings.Contains(summary.NextStep, "--iterations") {
			t.Fatalf("got %+v", summary)
		}
	})
	t.Run("incomplete evidence falls back honestly", func(t *testing.T) {
		incomplete := candidate
		incomplete.Status = "incomplete"
		summary := SummarizeResult(summaryReport("", "PASSED_FOR_CHECKSUM", VerdictInconclusive, baseline, incomplete))
		if summary.Conclusion != ConclusionInsufficient || summary.Cause != CauseComparisonIncomplete {
			t.Fatalf("got %+v", summary)
		}
		assertSummaryBasics(t, summary)
	})
	t.Run("correctness not run", func(t *testing.T) {
		summary := SummarizeResult(summaryReport("", "NOT_RUN", VerdictInconclusive, baseline, candidate))
		if summary.Cause != CauseComparisonIncomplete {
			t.Fatalf("got %+v", summary)
		}
	})
}

func TestSummarizeResultNeverShowsWinOrLossForInconclusive(t *testing.T) {
	baseline, candidate := goodPair([]uint64{100_000_000, 101_000_000, 99_000_000}, []uint64{80_000_000, 81_000_000, 79_000_000})
	report := summaryReport("", "NOT_RUN", VerdictInconclusive, baseline, candidate)
	text := strings.ToLower(SummarizeResult(report).Text())
	for _, phrase := range []string{"appears faster", "was slower", "improvement was demonstrated"} {
		if strings.Contains(text, phrase) {
			t.Fatalf("inconclusive summary contains %q: %s", phrase, text)
		}
	}
}
