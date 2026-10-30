package rustrush

import (
	"math"
	"strings"
	"testing"
	"time"
)

func f(v float64) *float64 { return &v }

func roiReport(baseline, candidate []uint64) Report {
	return Report{
		SchemaVersion: SchemaVersion, GeneratedAt: time.Unix(0, 0).UTC(), Project: "p", Component: "c",
		Workload: "w", Iterations: 10, Correctness: "PASSED_FOR_CHECKSUM",
		Environment: map[string]string{"os": "x"},
		Measurements: []Measurement{
			testMeasurement("Go baseline", baseline, 10),
			testMeasurement("Rust candidate", candidate, 10),
		},
	}
}

func fullInputs() ROIInputs {
	return ROIInputs{
		MonthlyOperations: f(3_600_000), BaselineCPUSecondsPerOperation: f(1), TargetCPUSecondsPerOperation: f(0.5),
		CPUCostPerHourUSD: f(0.1), EngineeringHours: f(10), EngineeringCostPerHourUSD: f(100),
	}
}

func TestMigrationCostAddsToEngineeringCost(t *testing.T) {
	inputs := fullInputs()
	inputs.MigrationCostUSD = f(500)
	result, err := CalculateROI(inputs)
	if err != nil || result.EstimatedEngineeringCostUSD == nil || *result.EstimatedEngineeringCostUSD != 1500 {
		t.Fatalf("got %+v, %v", result.EstimatedEngineeringCostUSD, err)
	}
	only := ROIInputs{MigrationCostUSD: f(500)}
	result, _ = CalculateROI(only)
	if result.EstimatedEngineeringCostUSD == nil || *result.EstimatedEngineeringCostUSD != 500 {
		t.Fatal("migration cost alone should be reported")
	}
}

func TestROIRejectsInvalidMigrationCost(t *testing.T) {
	for _, v := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := CalculateROI(ROIInputs{MigrationCostUSD: f(v)}); err == nil {
			t.Errorf("migration cost %v must be rejected", v)
		}
	}
}

func TestAnalyzeROIWithoutReportUsesOnlyAssumptions(t *testing.T) {
	analysis, err := AnalyzeROI(fullInputs(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if analysis.Claim != ClaimSavingsModeled || analysis.TargetSource != TargetSourceUser || analysis.Report != nil {
		t.Fatalf("unexpected: %+v", analysis)
	}
	// 1000 CPU hours * $0.1 * 50% = $50/month
	if got := *analysis.Result.MonthlyInfrastructureSavingsUSD; math.Abs(got-50) > 1e-9 {
		t.Fatalf("savings %v", got)
	}
	if analysis.Result.EstimatedNetMonthlySavingsUSD != nil || analysis.Result.BreakEvenMonths != nil {
		t.Fatal("net savings and break-even need a maintenance delta")
	}
}

func TestAnalyzeROIMissingInputsNeverClaimSavings(t *testing.T) {
	inputs := fullInputs()
	inputs.CPUCostPerHourUSD = nil
	analysis, err := AnalyzeROI(inputs, nil, DefaultROIScenarios())
	if err != nil {
		t.Fatal(err)
	}
	if analysis.Claim != ClaimNone || len(analysis.Scenarios) != 0 || analysis.Result.MonthlyInfrastructureSavingsUSD != nil {
		t.Fatalf("must not claim savings: %+v", analysis)
	}
	empty, err := AnalyzeROI(ROIInputs{}, nil, DefaultROIScenarios())
	if err != nil || empty.Claim != ClaimNone || len(empty.Scenarios) != 0 {
		t.Fatalf("%+v %v", empty, err)
	}
}

func TestAnalyzeROIEstimatesTargetFromMeasuredReport(t *testing.T) {
	report := roiReport([]uint64{1000, 1000, 1000}, []uint64{500, 500, 500})
	inputs := fullInputs()
	inputs.TargetCPUSecondsPerOperation = nil
	analysis, err := AnalyzeROI(inputs, &report, nil)
	if err != nil {
		t.Fatal(err)
	}
	if analysis.TargetSource != TargetSourceReportEstimate || analysis.Report == nil || !analysis.Report.MeasuredValuesUsed ||
		math.Abs(analysis.Report.WallClockRatio-0.5) > 1e-9 {
		t.Fatalf("%+v", analysis)
	}
	if got := *analysis.Result.TargetMonthlyCPUHours; math.Abs(got-500) > 1e-6 {
		t.Fatalf("target hours %v", got)
	}
	if analysis.Claim != ClaimSavingsModeled {
		t.Fatalf("claim %s", analysis.Claim)
	}
}

func TestAnalyzeROIEstimatesFromPythonBaselineReport(t *testing.T) {
	report := roiReport([]uint64{1000, 1000, 1000}, []uint64{500, 500, 500})
	report.Environment["baseline_language"] = "python"
	report.Measurements[0].Name = "Python baseline"
	inputs := fullInputs()
	inputs.TargetCPUSecondsPerOperation = nil
	analysis, err := AnalyzeROI(inputs, &report, nil)
	if err != nil {
		t.Fatal(err)
	}
	if analysis.Report == nil || !analysis.Report.MeasuredValuesUsed ||
		!strings.Contains(analysis.Report.ProcessWallClockMsg, "Python baseline / Rust candidate") {
		t.Fatalf("Python report should link its measured ratio accurately: %+v", analysis.Report)
	}
}

func TestAnalyzeROIUserTargetWinsOverReport(t *testing.T) {
	report := roiReport([]uint64{1000, 1000, 1000}, []uint64{500, 500, 500})
	analysis, err := AnalyzeROI(fullInputs(), &report, nil)
	if err != nil || analysis.TargetSource != TargetSourceUser {
		t.Fatalf("%+v %v", analysis, err)
	}
}

func TestAnalyzeROIInconclusiveReportBlocksEstimate(t *testing.T) {
	noisy := roiReport([]uint64{100, 1000, 100}, []uint64{50, 500, 50})
	inputs := fullInputs()
	inputs.TargetCPUSecondsPerOperation = nil
	analysis, err := AnalyzeROI(inputs, &noisy, DefaultROIScenarios())
	if err != nil {
		t.Fatal(err)
	}
	if analysis.Report.Verdict != VerdictInconclusive || analysis.Report.MeasuredValuesUsed ||
		analysis.Claim != ClaimNone || analysis.TargetSource != TargetSourceUnavailable || len(analysis.Scenarios) != 0 ||
		analysis.Result.MonthlyInfrastructureSavingsUSD != nil {
		t.Fatalf("inconclusive evidence must not produce savings: %+v", analysis)
	}
	failed := roiReport([]uint64{1000, 1000, 1000}, []uint64{500, 500, 500})
	failed.Measurements[1] = Measurement{Name: "Rust candidate", Status: "build_failed"}
	analysis, err = AnalyzeROI(inputs, &failed, nil)
	if err != nil || analysis.Claim != ClaimNone || analysis.TargetSource != TargetSourceUnavailable {
		t.Fatalf("%+v %v", analysis, err)
	}
}

func TestAnalyzeROISlowerCandidateModelsNoSavings(t *testing.T) {
	report := roiReport([]uint64{1000, 1000, 1000}, []uint64{2000, 2000, 2000})
	inputs := fullInputs()
	inputs.TargetCPUSecondsPerOperation = nil
	analysis, err := AnalyzeROI(inputs, &report, DefaultROIScenarios())
	if err != nil {
		t.Fatal(err)
	}
	if analysis.Claim != ClaimNoSavingsModeled || analysis.Result.BreakEvenMonths != nil {
		t.Fatalf("%+v", analysis)
	}
}

func TestROIScenarios(t *testing.T) {
	inputs := fullInputs()
	inputs.MonthlyMaintenanceCostDeltaUSD = f(0)
	analysis, err := AnalyzeROI(inputs, nil, DefaultROIScenarios())
	if err != nil || len(analysis.Scenarios) != 3 {
		t.Fatalf("%+v %v", analysis, err)
	}
	byName := map[string]ROIScenarioResult{}
	for _, s := range analysis.Scenarios {
		byName[s.Scenario.Name] = s
	}
	// expected: $50/month, engineering $1000 => 20 months
	expected := byName["expected"]
	if math.Abs(*expected.NetMonthlySavingsUSD-50) > 1e-9 || math.Abs(*expected.BreakEvenMonths-20) > 1e-9 {
		t.Fatalf("%+v", expected)
	}
	// conservative: 0.5 volume, 0.5 realization => $12.50/month, cost $1500 => 120 months
	conservative := byName["conservative"]
	if math.Abs(*conservative.NetMonthlySavingsUSD-12.5) > 1e-9 || math.Abs(*conservative.BreakEvenMonths-120) > 1e-9 ||
		math.Abs(conservative.EffectiveTargetCPUSeconds-0.75) > 1e-12 {
		t.Fatalf("%+v", conservative)
	}
	if *byName["optimistic"].NetMonthlySavingsUSD <= *expected.NetMonthlySavingsUSD {
		t.Fatal("optimistic must exceed expected")
	}
	// maintenance cost can flip a scenario to no savings
	inputs.MonthlyMaintenanceCostDeltaUSD = f(40)
	analysis, err = AnalyzeROI(inputs, nil, DefaultROIScenarios())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range analysis.Scenarios {
		if s.Scenario.Name == "conservative" && (s.Claim != ClaimNoSavingsModeled || s.BreakEvenMonths != nil) {
			t.Fatalf("%+v", s)
		}
	}
}

func TestROIScenarioValidation(t *testing.T) {
	bad := []ROIScenario{
		{Name: "", VolumeMultiplier: 1, RealizationFactor: 1, MigrationCostMultiplier: 1},
		{Name: "zero", VolumeMultiplier: 0, RealizationFactor: 1, MigrationCostMultiplier: 1},
		{Name: "neg", VolumeMultiplier: 1, RealizationFactor: -1, MigrationCostMultiplier: 1},
		{Name: "nan", VolumeMultiplier: 1, RealizationFactor: 1, MigrationCostMultiplier: math.NaN()},
		{Name: "over", VolumeMultiplier: 1, RealizationFactor: 1.5, MigrationCostMultiplier: 1},
	}
	for _, s := range bad {
		if _, err := AnalyzeROI(fullInputs(), nil, []ROIScenario{s}); err == nil {
			t.Errorf("scenario %q must be rejected", s.Name)
		}
	}
}

func TestAnalyzeROIRejectsInvalidInputs(t *testing.T) {
	for name, mutate := range map[string]func(*ROIInputs){
		"zero ops":    func(i *ROIInputs) { i.MonthlyOperations = f(0) },
		"negative":    func(i *ROIInputs) { i.CPUCostPerHourUSD = f(-1) },
		"nan":         func(i *ROIInputs) { i.BaselineCPUSecondsPerOperation = f(math.NaN()) },
		"inf":         func(i *ROIInputs) { i.EngineeringHours = f(math.Inf(1)) },
		"zero target": func(i *ROIInputs) { i.TargetCPUSecondsPerOperation = f(0) },
	} {
		inputs := fullInputs()
		mutate(&inputs)
		if _, err := AnalyzeROI(inputs, nil, nil); err == nil {
			t.Errorf("%s must be rejected", name)
		}
	}
	report := roiReport([]uint64{1000, 1000, 1000}, []uint64{500, 500, 500})
	inputs := fullInputs()
	inputs.TargetCPUSecondsPerOperation = nil
	inputs.BaselineCPUSecondsPerOperation = f(0)
	if _, err := AnalyzeROI(inputs, &report, nil); err == nil || !strings.Contains(err.Error(), "greater than zero") {
		t.Fatalf("%v", err)
	}
}

func TestROIReportEstimateNotesAreConsistent(t *testing.T) {
	report := roiReport([]uint64{2_000_000, 2_000_000, 2_000_000}, []uint64{1_000_000, 1_000_000, 1_000_000})
	report.Verdict = VerdictPort
	baseline, price, volume := 0.01, 0.05, 1_000_000.0
	analysis, err := AnalyzeROI(ROIInputs{BaselineCPUSecondsPerOperation: &baseline, CPUCostPerHourUSD: &price, MonthlyOperations: &volume}, &report, nil)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(append(append([]string{}, analysis.Notes...), analysis.Result.Notes...), "\n")
	if strings.Contains(joined, "none is inferred from benchmark") || strings.Contains(joined, "overrides the note above") {
		t.Fatalf("contradictory notes:\n%s", joined)
	}
	if !strings.Contains(joined, "no statistical confidence") {
		t.Fatalf("missing confidence caveat:\n%s", joined)
	}
}
