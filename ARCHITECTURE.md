# Architecture

RustRush currently uses Go's standard library and has no third-party runtime dependencies.

## Modules

- `cmd/rustrush` parses CLI commands and formats user-facing output.
- `internal/rustrush/analyze.go` walks supported source files, applies exclusions, and emits explicitly low-confidence static hypotheses. It never executes scanned source.
- `internal/rustrush/workload.go` owns the deterministic Go workload, Rust candidate build/run sequence, timing, correctness gate, and report creation.
- `internal/rustrush/json_workload.go` and `internal/rustrush/database_fixture.go` implement the Go baselines for the JSON fixture and CSV mock.
- `internal/rustrush/models.go` defines the versioned report, percentile aggregation, validation, Markdown rendering, and comparison behavior.
- `internal/rustrush/roi.go` calculates optional migration economics only from explicitly supplied assumptions.
- `internal/rustrush/toolchain.go` discovers local Go and Rust compiler availability.
- `examples/cpu-heavy/candidate.rs` is the Rust implementation used by the local demo.

## Data flow

`analyze` discovers source files and produces an inventory plus static pattern findings. The findings cannot be used as measurement evidence.

`demo` launches the selected Go baseline and, when available, compiles and launches the corresponding built-in Rust candidate. Both receive the same fixture and iteration count and emit a deterministic result. A mismatch prevents a positive verdict. The CLI records each measured trial and environment metadata in schema version `1.0`, then writes JSON and Markdown locally. `roi` separately calculates estimates from explicit user-supplied assumptions; it does not infer ROI from benchmark wall time.

Reports do not include source contents or absolute project paths. The report schema and fields are documented in [docs/report-schema.md](docs/report-schema.md).

## Deliberate scope limits

Candidate translation, arbitrary benchmark adapters, profiling, ROI integration in benchmark reports, a real database workload, AI integrations, and remote sharing are not implemented. They should be introduced as separate modules only when their correctness and safety contracts can be tested.
