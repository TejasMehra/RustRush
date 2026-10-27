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

All inputs are user-provided assumptions. RustRush does not infer CPU time from process wall-clock benchmark results, and it does not invent cloud pricing, workload volume, engineering cost, or maintenance cost. CPU-seconds-per-operation must represent CPU time rather than request latency. Break-even is omitted unless the required inputs, including a maintenance-cost delta, are supplied and net savings are positive.

Use `--json` for machine-readable output. The result is a model, not a financial guarantee.
