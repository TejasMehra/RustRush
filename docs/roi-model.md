# Migration ROI model

Run `rustrush roi --help` for the available assumptions. The calculator keeps inputs optional and lists missing values rather than filling them with defaults.

Example:

```powershell
go run ./cmd/rustrush roi `
  --monthly-operations 3600000 `
  --baseline-cpu-seconds 0.001 `
  --target-cpu-seconds 0.0005 `
  --cpu-hour-price 10 `
  --engineering-hours 10 `
  --engineering-hourly-cost 100 `
  --monthly-maintenance-cost-delta 1
```

For volume `V`, baseline CPU time `B`, target CPU time `T`, and CPU-hour price `P`:

- Current CPU hours per month = `V * B / 3600`
- Target CPU hours per month = `V * T / 3600`
- Infrastructure savings per month = `(current CPU hours - target CPU hours) * P`
- Engineering cost = `engineering hours * engineering cost per hour`
- Net monthly savings = infrastructure savings minus the supplied target-minus-current maintenance cost delta
- Break-even months = engineering cost divided by positive net monthly savings

All inputs are user-provided assumptions unless a benchmark report is linked. RustRush does not invent cloud pricing, workload volume, engineering cost, or maintenance cost. CPU-seconds-per-operation must represent CPU time rather than request latency. Break-even is omitted unless the required inputs, including a maintenance-cost delta, are supplied and net savings are positive.

## Extensions

- `--migration-cost USD` adds a one-off cost to the engineering cost.
- `--report results/latest.json`: if `--target-cpu-seconds` is omitted and the report is conclusive (not `INCONCLUSIVE`, valid measurements), the target is estimated as `baseline CPU seconds x (candidate median / baseline median wall-clock)`. Process wall-clock time is only a proxy for CPU time, so this is labeled `estimated_from_report_wall_clock_ratio`. An inconclusive or failed report yields no target and no savings claim. A user-supplied target always wins.
- `--scenarios` runs conservative (0.5x volume, 50% of the CPU gain realized, 1.5x migration effort), expected, and optimistic (1.5x volume, 0.75x effort) cases. These multipliers are illustrative assumptions, not forecasts. Each case reports `MODELED_SAVINGS`, `MODELED_NO_SAVINGS`, or `NO_SAVINGS_CLAIM`; scenarios are only produced when volume, baseline CPU, CPU price and a target are available.
- Zero, negative, NaN and infinite inputs are rejected (the maintenance delta may be negative); missing values are never treated as zero.

Use `--json` for machine-readable output. The result is a model, not a financial guarantee.
