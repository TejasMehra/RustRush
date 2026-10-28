package rustrush

import (
	"errors"
	"fmt"
	"strings"
)

const (
	TargetSourceUser            = "user_supplied"
	TargetSourceReportEstimate  = "estimated_from_report_wall_clock_ratio"
	TargetSourceUnavailable     = "unavailable"
	ClaimNone                   = "NO_SAVINGS_CLAIM"
	ClaimSavingsModeled         = "MODELED_SAVINGS"
	ClaimNoSavingsModeled       = "MODELED_NO_SAVINGS"
	reportRatioInconclusiveNote = "No savings are modeled from the report because its verdict is INCONCLUSIVE or its measurements are invalid."
)

// ROIScenario scales user assumptions. Every factor is an assumption, not a measurement.
type ROIScenario struct {
	Name                    string  `json:"name"`
	VolumeMultiplier        float64 `json:"volume_multiplier"`
	RealizationFactor       float64 `json:"realization_factor"`
	MigrationCostMultiplier float64 `json:"migration_cost_multiplier"`
}

// DefaultROIScenarios are illustrative sensitivity assumptions, not forecasts.
func DefaultROIScenarios() []ROIScenario {
	return []ROIScenario{
		{Name: "conservative", VolumeMultiplier: 0.5, RealizationFactor: 0.5, MigrationCostMultiplier: 1.5},
		{Name: "expected", VolumeMultiplier: 1, RealizationFactor: 1, MigrationCostMultiplier: 1},
		{Name: "optimistic", VolumeMultiplier: 1.5, RealizationFactor: 1, MigrationCostMultiplier: 0.75},
	}
}

func (s ROIScenario) validate() error {
	if s.Name == "" {
		return errors.New("scenario name is required")
	}
	for name, value := range map[string]float64{
		"volume_multiplier": s.VolumeMultiplier, "realization_factor": s.RealizationFactor,
		"migration_cost_multiplier": s.MigrationCostMultiplier,
	} {
		if !isFinite(value) || value <= 0 {
			return fmt.Errorf("scenario %q: %s must be a finite number greater than zero", s.Name, name)
		}
	}
	if s.RealizationFactor > 1 {
		return fmt.Errorf("scenario %q: realization_factor must not exceed 1", s.Name)
	}
	return nil
}

// ROIReportLink describes the measured values the analysis relied on.
type ROIReportLink struct {
	Verdict             Verdict `json:"verdict"`
	Correctness         string  `json:"correctness"`
	BaselineMedianNS    uint64  `json:"baseline_median_ns,omitempty"`
	CandidateMedianNS   uint64  `json:"candidate_median_ns,omitempty"`
	WallClockRatio      float64 `json:"candidate_to_baseline_wall_clock_ratio,omitempty"`
	MeasuredValuesUsed  bool    `json:"measured_values_used"`
	ProcessWallClockMsg string  `json:"note"`
}

type ROIScenarioResult struct {
	Scenario                  ROIScenario `json:"scenario"`
	Type                      string      `json:"type"`
	InfraSavingsUSD           *float64    `json:"monthly_infrastructure_savings_usd,omitempty"`
	NetMonthlySavingsUSD      *float64    `json:"estimated_net_monthly_savings_usd,omitempty"`
	EngineeringCostUSD        *float64    `json:"estimated_engineering_cost_usd,omitempty"`
	BreakEvenMonths           *float64    `json:"break_even_months,omitempty"`
	Claim                     string      `json:"claim"`
	EffectiveTargetCPUSeconds float64     `json:"effective_target_cpu_seconds_per_operation"`
}

type ROIAnalysis struct {
	Type         string              `json:"type"`
	Claim        string              `json:"claim"`
	TargetSource string              `json:"target_cpu_source"`
	Report       *ROIReportLink      `json:"report,omitempty"`
	Result       ROIResult           `json:"result"`
	Scenarios    []ROIScenarioResult `json:"scenarios"`
	Notes        []string            `json:"notes"`
}

func reportWallClockRatio(report Report) (*ROIReportLink, float64, bool) {
	normalizeReportEvidence(&report)
	link := &ROIReportLink{
		Verdict: report.Verdict, Correctness: report.Correctness,
		ProcessWallClockMsg: "Ratio of median process durations (" + timingMode(report) + "); a proxy for CPU-time ratio, not a CPU measurement. It carries no confidence interval.",
	}
	var baseline, candidate Measurement
	for _, measurement := range report.Measurements {
		switch measurement.Name {
		case "Go baseline":
			baseline = measurement
		case "Rust candidate":
			candidate = measurement
		}
	}
	if report.Verdict == VerdictInconclusive || !validMeasurement(baseline) || !validMeasurement(candidate) {
		return link, 0, false
	}
	ratio := float64(candidate.MedianNS) / float64(baseline.MedianNS)
	if !isFinite(ratio) || ratio <= 0 {
		return link, 0, false
	}
	link.BaselineMedianNS, link.CandidateMedianNS = baseline.MedianNS, candidate.MedianNS
	link.WallClockRatio, link.MeasuredValuesUsed = ratio, true
	return link, ratio, true
}

// AnalyzeROI links optional benchmark evidence to user-supplied assumptions.
// A nil report means every value is an assumption. Savings are never claimed
// when a required input is missing or the report is not conclusive.
func AnalyzeROI(inputs ROIInputs, report *Report, scenarios []ROIScenario) (ROIAnalysis, error) {
	for _, scenario := range scenarios {
		if err := scenario.validate(); err != nil {
			return ROIAnalysis{}, err
		}
	}
	analysis := ROIAnalysis{Type: "modeled_estimate", Claim: ClaimNone, TargetSource: TargetSourceUnavailable, Scenarios: []ROIScenarioResult{}}
	analysis.Notes = append(analysis.Notes,
		"Measured benchmark values and user-supplied assumptions are kept separate; every monetary figure here is a modeled estimate.")

	effective := inputs
	reportUsable := false
	if report != nil {
		link, ratio, ok := reportWallClockRatio(*report)
		analysis.Report = link
		reportUsable = ok
		if !ok {
			analysis.Notes = append(analysis.Notes, reportRatioInconclusiveNote)
		} else if inputs.TargetCPUSecondsPerOperation == nil && inputs.BaselineCPUSecondsPerOperation != nil {
			if !isFinite(*inputs.BaselineCPUSecondsPerOperation) || *inputs.BaselineCPUSecondsPerOperation <= 0 {
				return ROIAnalysis{}, errors.New("baseline_cpu_seconds_per_operation must be greater than zero")
			}
			target := *inputs.BaselineCPUSecondsPerOperation * ratio
			effective.TargetCPUSecondsPerOperation = &target
			analysis.TargetSource = TargetSourceReportEstimate
			analysis.Notes = append(analysis.Notes,
				"Target CPU seconds per operation is an estimate: user-supplied baseline CPU seconds x the measured candidate/baseline median-duration ratio. The ratio is measured; the CPU conversion is an assumption.")
		}
	}
	if effective.TargetCPUSecondsPerOperation != nil && analysis.TargetSource == TargetSourceUnavailable {
		analysis.TargetSource = TargetSourceUser
	}
	if report != nil && !reportUsable && inputs.TargetCPUSecondsPerOperation == nil {
		result, err := CalculateROI(inputs)
		if err != nil {
			return ROIAnalysis{}, err
		}
		analysis.Result = result
		return analysis, nil
	}

	result, err := CalculateROI(effective)
	if err != nil {
		return ROIAnalysis{}, err
	}
	if analysis.TargetSource == TargetSourceReportEstimate {
		kept := result.Notes[:0:0]
		for _, note := range result.Notes {
			if !strings.Contains(note, "none is inferred from benchmark") {
				kept = append(kept, note)
			}
		}
		result.Notes = kept
	}
	analysis.Result = result
	if result.MonthlyInfrastructureSavingsUSD == nil {
		analysis.Notes = append(analysis.Notes, "Savings are not claimed because required inputs are missing.")
		return analysis, nil
	}
	analysis.Claim = claimFor(result.EstimatedNetMonthlySavingsUSD, *result.MonthlyInfrastructureSavingsUSD)
	analysis.Notes = append(analysis.Notes,
		"A MODELED_* claim is arithmetic on the stated assumptions, not a measured or guaranteed saving, and carries no statistical confidence. Scenario multipliers are illustrative, not forecasts.")

	for _, scenario := range scenarios {
		item, err := runScenario(effective, scenario)
		if err != nil {
			return ROIAnalysis{}, err
		}
		analysis.Scenarios = append(analysis.Scenarios, item)
	}
	return analysis, nil
}

func claimFor(net *float64, infrastructureSavings float64) string {
	value := infrastructureSavings
	if net != nil {
		value = *net
	}
	if value > 0 {
		return ClaimSavingsModeled
	}
	return ClaimNoSavingsModeled
}

func runScenario(inputs ROIInputs, scenario ROIScenario) (ROIScenarioResult, error) {
	base, target := *inputs.BaselineCPUSecondsPerOperation, *inputs.TargetCPUSecondsPerOperation
	effectiveTarget := base - (base-target)*scenario.RealizationFactor
	operations := *inputs.MonthlyOperations * scenario.VolumeMultiplier
	scaled := inputs
	scaled.MonthlyOperations = &operations
	scaled.TargetCPUSecondsPerOperation = &effectiveTarget
	if inputs.EngineeringHours != nil {
		hours := *inputs.EngineeringHours * scenario.MigrationCostMultiplier
		scaled.EngineeringHours = &hours
	}
	if inputs.MigrationCostUSD != nil {
		cost := *inputs.MigrationCostUSD * scenario.MigrationCostMultiplier
		scaled.MigrationCostUSD = &cost
	}
	result, err := CalculateROI(scaled)
	if err != nil {
		return ROIScenarioResult{}, fmt.Errorf("scenario %q: %w", scenario.Name, err)
	}
	item := ROIScenarioResult{
		Scenario: scenario, Type: "modeled_estimate", Claim: ClaimNone,
		InfraSavingsUSD: result.MonthlyInfrastructureSavingsUSD, NetMonthlySavingsUSD: result.EstimatedNetMonthlySavingsUSD, EngineeringCostUSD: result.EstimatedEngineeringCostUSD,
		BreakEvenMonths: result.BreakEvenMonths, EffectiveTargetCPUSeconds: effectiveTarget,
	}
	if result.MonthlyInfrastructureSavingsUSD != nil {
		item.Claim = claimFor(result.EstimatedNetMonthlySavingsUSD, *result.MonthlyInfrastructureSavingsUSD)
	}
	return item, nil
}
