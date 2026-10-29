# Report schema

RustRush writes UTF-8 JSON with a top-level `schema_version` of `"1.1"`. The CLI continues to read the earlier `"1.0"` report shape.

| Field | Meaning |
|---|---|
| `schema_version` | Version of this report shape |
| `generated_at` | UTC generation timestamp |
| `project`, `component` | Non-sensitive identifiers for the project and tested operation |
| `workload`, `iterations` | Workload description and iteration count per process |
| `correctness` | `PASSED_FOR_CHECKSUM`, `FAILED`, or `NOT_RUN` |
| `verdict`, `reason` | Evidence-limited recommendation and explanation |
| `throughput_delta_ops_per_sec`, `throughput_delta_percent` | Rust-vs-Go delta when both implementations are validly measured |
| `environment` | OS, architecture, Go runtime/compiler, CPU count, Go `GOMAXPROCS`, and Rust compiler/version flags when available |
| `measurements` | Per-implementation status, repetitions, warm-up count, process error count, correctness-probe output, raw nanosecond samples, process-duration p50/p95/p99, median throughput, coefficient of variation, and any measurement-quality warnings |
| `limitations` | Caveats that affect interpretation |
| `reproduction` | Command used to rerun the workload |
| `assumptions` | Process timing scope and warm-up information |

The latency percentiles are process-level observations, not request-level percentiles. Error counts describe failed or correctness-invalid benchmark attempts. Unavailable, not-run, incomplete, failed, timed-out, and correctness-failed measurements are represented by status and error fields; absent values are not replaced with invented numbers. For legacy `1.0` reports, the reader infers the prior implementation's single warm-up and correctness output for successful measurements and one error for failed/build-failed measurements. The schema is a local experimental format and is not yet a public interchange or leaderboard contract.

Each sample may include `timer_resolution_ns`. Container reports use a monotonic Go helper whose stdout protocol is structured JSON with base64-encoded workload output; the helper response is not mixed with workload stdout. The empirical minimum clock tick is metadata, not an accuracy claim. Container samples below 1,000 observed ticks carry a warning and cannot support an aggregate verdict. Older reports without timer fields remain readable; a missing `environment.timing_mode` means `process_end_to_end`.

`environment.timing_mode` (additive, schema stays 1.1) is `process_end_to_end` or `container_process_isolated`; absent means `process_end_to_end`. Legacy reports may contain `steady_state_in_container`; it remains readable but is never produced by current runs, and comparisons still require identical modes. Container reports may carry `container_invocation_wall_ms_median` (full Docker invocation duration across correctness, warm-up, and trial runs; not a workload sample), `container_run_overhead_ms_estimate_median` (a derived residual after subtracting child duration across successful invocations; it includes timer/dispatch/lifecycle costs, and clocks are not cross-calibrated), `container_failed_invocation_wall_ms_median` (failed invocations, not valid samples), `timing_clock`, and `timing_resolution_ns`. The resolution field is an empirical minimum monotonic-clock tick, not an accuracy guarantee.
