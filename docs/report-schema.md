# Report schema

RustRush writes UTF-8 JSON with a top-level `schema_version` of `"1.0"`. The CLI validates the version before rendering or comparing a report.

| Field | Meaning |
|---|---|
| `schema_version` | Version of this report shape |
| `generated_at` | UTC generation timestamp |
| `project`, `component` | Non-sensitive identifiers for the project and tested operation |
| `workload`, `iterations` | Workload description and iteration count per process |
| `correctness` | `PASSED_FOR_CHECKSUM`, `FAILED`, or `NOT_RUN` |
| `verdict`, `reason` | Evidence-limited recommendation and explanation |
| `throughput_delta_ops_per_sec`, `throughput_delta_percent` | Rust-vs-Go delta when both implementations are validly measured |
| `environment` | OS, architecture, Go runtime, CPU count, and Rust compiler when available |
| `measurements` | Per-implementation status, configuration, raw samples, p50/p95/p99, median throughput, and coefficient of variation |
| `limitations` | Caveats that affect interpretation |
| `reproduction` | Command used to rerun the workload |
| `assumptions` | Timing scope and warm-up information |

Unavailable or failed measurements are represented by status and error fields; absent values are not replaced with invented numbers. The schema is a local experimental format and is not yet a public interchange or leaderboard contract.
