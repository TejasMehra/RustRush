# Architecture

RustRush currently uses Go's standard library and has no third-party runtime dependencies.

## Modules

- `cmd/rustrush` parses CLI commands and formats user-facing output.
- `internal/rustrush/analyze.go` walks supported source files, applies exclusions, and emits explicitly low-confidence static hypotheses. It never executes scanned source.
- `internal/rustrush/workload.go` owns the deterministic Go workload, Rust candidate build/run sequence, timing, correctness gate, and report creation.
- `internal/rustrush/json_workload.go` and `internal/rustrush/database_fixture.go` implement the Go baselines for the JSON fixture and CSV mock.
- `internal/rustrush/models.go` defines the versioned report, percentile aggregation, validation, Markdown rendering, and comparison behavior.
- `internal/rustrush/roi.go` calculates optional migration economics only from explicitly supplied assumptions; `roi_report.go` links a saved report (labeled estimate, blocked when inconclusive) and runs scenario sensitivity.
- `internal/rustrush/container.go` is the optional fail-closed Docker runner for `candidate`; it builds both toolchains and a standalone timer helper in containers. `container_runner.go` launches a fresh hardened `docker run --rm` for each correctness probe, warm-up, and measured invocation. `container_timer/main.go` times only the workload child with Go's monotonic clock and transports stdout separately as base64 JSON. Container lifecycle and timer overhead are outside samples; full invocation wall time is measured and the residual overhead is reported as an estimate because clocks are not cross-calibrated. Fresh containers prevent process-created files and caches from carrying between repetitions, but host page cache/kernel state and differences between the Go and Rust images remain. Reports record the timing mode, and `compare` refuses to mix timing modes.
- `internal/rustrush/toolchain.go` discovers local Go and Rust compiler availability.
- `examples/cpu-heavy/candidate.rs` is the Rust implementation used by the local demo.

## Data flow

`analyze` discovers source files and produces an inventory plus static pattern findings. The findings cannot be used as measurement evidence.

`demo` compiles the selected built-in Rust candidate when available, then runs one warm-up per implementation; these warm-ups also form the correctness gate, and their durations are discarded. Only after matching outputs does RustRush start paired measured repetitions in alternating order. Both implementations receive the same fixture and iteration count. A mismatch prevents measured comparison; failed, missing, invalid, or noisy samples produce `INCONCLUSIVE`. Container timing responses use strict JSON and base64 output transport; timing fields cannot be spoofed through workload stdout. Container samples below 1,000 empirically observed clock ticks are retained but cannot produce a verdict. The CLI records raw process durations, process-level error counts, aggregates, and environment metadata in schema version `1.1` (and reads `1.0` reports), then writes JSON and Markdown locally. `roi` separately calculates estimates from explicit user-supplied assumptions; it does not infer ROI from benchmark wall time.

Reports do not include source contents or absolute project paths. The report schema and fields are documented in [docs/report-schema.md](docs/report-schema.md).

## Deliberate scope limits

Candidate translation, arbitrary benchmark adapters, profiling, ROI embedded in benchmark reports, a real database workload, AI integrations, and remote sharing are not implemented. They should be introduced as separate modules only when their correctness and safety contracts can be tested.
