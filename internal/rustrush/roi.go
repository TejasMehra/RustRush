package rustrush

import (
	"errors"
	"fmt"
	"math"
)

type ROIInputs struct {
	MonthlyOperations              *float64 `json:"monthly_operations,omitempty"`
	BaselineCPUSecondsPerOperation *float64 `json:"baseline_cpu_seconds_per_operation,omitempty"`
	TargetCPUSecondsPerOperation   *float64 `json:"target_cpu_seconds_per_operation,omitempty"`
	CPUCostPerHourUSD              *float64 `json:"cpu_cost_per_hour_usd,omitempty"`
	EngineeringHours               *float64 `json:"engineering_hours,omitempty"`
	EngineeringCostPerHourUSD      *float64 `json:"engineering_cost_per_hour_usd,omitempty"`
	MonthlyMaintenanceCostDeltaUSD *float64 `json:"monthly_maintenance_cost_delta_usd,omitempty"`
	MigrationCostUSD               *float64 `json:"migration_cost_usd,omitempty"`
}

type ROIResult struct {
	Type                            string    `json:"type"`
	Inputs                          ROIInputs `json:"inputs"`
	CurrentMonthlyCPUHours          *float64  `json:"current_monthly_cpu_hours,omitempty"`
	TargetMonthlyCPUHours           *float64  `json:"target_monthly_cpu_hours,omitempty"`
	CurrentMonthlyCostUSD           *float64  `json:"current_monthly_cost_usd,omitempty"`
	TargetMonthlyCostUSD            *float64  `json:"target_monthly_cost_usd,omitempty"`
	MonthlyInfrastructureSavingsUSD *float64  `json:"monthly_infrastructure_savings_usd,omitempty"`
	EstimatedEngineeringCostUSD     *float64  `json:"estimated_engineering_cost_usd,omitempty"`
	EstimatedNetMonthlySavingsUSD   *float64  `json:"estimated_net_monthly_savings_usd,omitempty"`
	BreakEvenMonths                 *float64  `json:"break_even_months,omitempty"`
	MissingInputs                   []string  `json:"missing_inputs"`
	Notes                           []string  `json:"notes"`
}

func CalculateROI(inputs ROIInputs) (ROIResult, error) {
	namedInputs := []struct {
		name  string
		value *float64
	}{
		{"monthly_operations", inputs.MonthlyOperations},
		{"baseline_cpu_seconds_per_operation", inputs.BaselineCPUSecondsPerOperation},
		{"target_cpu_seconds_per_operation", inputs.TargetCPUSecondsPerOperation},
		{"cpu_cost_per_hour_usd", inputs.CPUCostPerHourUSD},
		{"engineering_hours", inputs.EngineeringHours},
		{"engineering_cost_per_hour_usd", inputs.EngineeringCostPerHourUSD},
		{"monthly_maintenance_cost_delta_usd", inputs.MonthlyMaintenanceCostDeltaUSD},
	}
	if inputs.MigrationCostUSD != nil && (!isFinite(*inputs.MigrationCostUSD) || *inputs.MigrationCostUSD <= 0) {
		return ROIResult{}, errors.New("migration_cost_usd must be a finite number greater than zero")
	}
	for _, input := range namedInputs {
		if input.value == nil {
			continue
		}
		if !isFinite(*input.value) {
			return ROIResult{}, fmt.Errorf("%s must be a finite number", input.name)
		}
		if input.name != "monthly_maintenance_cost_delta_usd" && *input.value <= 0 {
			return ROIResult{}, fmt.Errorf("%s must be greater than zero", input.name)
		}
	}

	result := ROIResult{
		Type:          "modeled_estimate",
		Inputs:        inputs,
		MissingInputs: []string{},
		Notes: []string{
			"Every input is user-provided; none is inferred from benchmark wall-clock timings.",
			"CPU seconds per operation must represent CPU time, not end-to-end wall time.",
			"Break-even is not calculated unless a monthly maintenance cost delta is supplied.",
		},
	}
	for _, input := range namedInputs {
		if input.value == nil {
			result.MissingInputs = append(result.MissingInputs, input.name)
		}
	}

	if inputs.MonthlyOperations != nil && inputs.BaselineCPUSecondsPerOperation != nil {
		hours := *inputs.MonthlyOperations * *inputs.BaselineCPUSecondsPerOperation / 3600
		if !isFinite(hours) {
			return ROIResult{}, errors.New("modeled current CPU hours exceed the supported numeric range")
		}
		result.CurrentMonthlyCPUHours = &hours
	}
	if inputs.MonthlyOperations != nil && inputs.TargetCPUSecondsPerOperation != nil {
		hours := *inputs.MonthlyOperations * *inputs.TargetCPUSecondsPerOperation / 3600
		if !isFinite(hours) {
			return ROIResult{}, errors.New("modeled target CPU hours exceed the supported numeric range")
		}
		result.TargetMonthlyCPUHours = &hours
	}
	if result.CurrentMonthlyCPUHours != nil && inputs.CPUCostPerHourUSD != nil {
		cost := *result.CurrentMonthlyCPUHours * *inputs.CPUCostPerHourUSD
		if !isFinite(cost) {
			return ROIResult{}, errors.New("modeled current infrastructure cost exceeds the supported numeric range")
		}
		result.CurrentMonthlyCostUSD = &cost
	}
	if result.TargetMonthlyCPUHours != nil && inputs.CPUCostPerHourUSD != nil {
		cost := *result.TargetMonthlyCPUHours * *inputs.CPUCostPerHourUSD
		if !isFinite(cost) {
			return ROIResult{}, errors.New("modeled target infrastructure cost exceeds the supported numeric range")
		}
		result.TargetMonthlyCostUSD = &cost
	}
	if result.CurrentMonthlyCostUSD != nil && result.TargetMonthlyCostUSD != nil {
		savings := *result.CurrentMonthlyCostUSD - *result.TargetMonthlyCostUSD
		result.MonthlyInfrastructureSavingsUSD = &savings
	}
	if (inputs.EngineeringHours != nil && inputs.EngineeringCostPerHourUSD != nil) || inputs.MigrationCostUSD != nil {
		cost := 0.0
		if inputs.EngineeringHours != nil && inputs.EngineeringCostPerHourUSD != nil {
			cost = *inputs.EngineeringHours * *inputs.EngineeringCostPerHourUSD
		}
		if inputs.MigrationCostUSD != nil {
			cost += *inputs.MigrationCostUSD
		}
		if !isFinite(cost) {
			return ROIResult{}, errors.New("modeled engineering cost exceeds the supported numeric range")
		}
		result.EstimatedEngineeringCostUSD = &cost
	}
	if result.MonthlyInfrastructureSavingsUSD != nil && inputs.MonthlyMaintenanceCostDeltaUSD != nil {
		net := *result.MonthlyInfrastructureSavingsUSD - *inputs.MonthlyMaintenanceCostDeltaUSD
		if !isFinite(net) {
			return ROIResult{}, errors.New("modeled net monthly savings exceeds the supported numeric range")
		}
		result.EstimatedNetMonthlySavingsUSD = &net
	}
	if result.EstimatedEngineeringCostUSD != nil && result.EstimatedNetMonthlySavingsUSD != nil &&
		*result.EstimatedNetMonthlySavingsUSD > 0 {
		months := *result.EstimatedEngineeringCostUSD / *result.EstimatedNetMonthlySavingsUSD
		if !isFinite(months) {
			return ROIResult{}, errors.New("modeled break-even period exceeds the supported numeric range")
		}
		result.BreakEvenMonths = &months
	}
	return result, nil
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
