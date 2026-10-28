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
| `measurements` | Per-implementation status, repetitions, warm-up count, process error count, correctness-probe output, raw nanosecond samples, process-wall-clock p50/p95/p99, median throughput, and coefficient of variation |
| `limitations` | Caveats that affect interpretation |
| `reproduction` | Command used to rerun the workload |
| `assumptions` | Process timing scope and warm-up information |

The latency percentiles are process-level observations, not request-level percentiles. Error counts describe failed or correctness-invalid benchmark attempts. Unavailable, not-run, incomplete, failed, timed-out, and correctness-failed measurements are represented by status and error fields; absent values are not replaced with invented numbers. For legacy `1.0` reports, the reader infers the prior implementation's single warm-up and correctness output for successful measurements and one error for failed/build-failed measurements. The schema is a local experimental format and is not yet a public interchange or leaderboard contract.

`environment.timing_mode` (additive, schema stays 1.1) is `process_end_to_end` or `steady_state_in_container`; absent means `process_end_to_end`. Container reports also carry `container_startup_ms`, `container_exec_dispatch_ms_median` and `timing_resolution` describing overheads excluded from samples.
