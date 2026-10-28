# Implementation status

This status describes the checked-in implementation; items marked planned are not exposed as working CLI commands.

## Implemented and tested

- [x] Go CLI with `doctor`, `analyze`, `guide`, `candidate`, `demo`, `report`, `compare`, and `roi`.
- [x] `guide`: non-executing project-type detection (Go module = candidate workflow; other markers = analysis only; otherwise unsupported) and ranked Go candidate-operation hypotheses.
- [x] `candidate`: explicit-consent workflow that copies and builds a user Go main package and a single-file Rust candidate in a temp dir, applies the correctness gate, and reuses the paired benchmark engine and report format. Verified locally with a real Go build and injected-runner tests; Rust compilation is exercised only in the CI `rust-demo` job.
- [x] Tests for guide classification, unsupported/invalid inputs, consent, source preservation, failed/timed-out candidates, build failures, missing rustc, and correctness-gate behaviour.
- [x] Markdown report keeps tables intact and surfaces measurement errors.
- [x] Local source inventory for common languages and low-confidence loop, serialization, database, and network hypotheses.
- [x] Dependency/build and credential-like path exclusions; analysis does not execute source.
- [x] Deterministic Go baseline and corresponding Rust integer-mixing candidate source.
- [x] Shared JSON integer-array fixture, Go transformation baseline, and dependency-free Rust candidate for the fixture shape.
- [x] Shared CSV filter-and-aggregate mock with Go baseline, Rust candidate source, and explicit non-database limitations.
- [x] Rust-enabled Ubuntu CI compiles all three candidates and asserts correctness, warm-up, error-count, and repeated-sample report fields; this does not claim a performance win.
- [x] Untimed cross-language correctness gate before timing; one warm-up and configurable paired repetitions with alternating run order.
- [x] Raw process timings, throughput, process p50/p95/p99, process error counts, metadata, and inconclusive handling for invalid, failed, missing, or noisy evidence.
- [x] Versioned JSON plus measured-versus-estimated Markdown reporting, raw samples, and workload-compatible report comparison.
- [x] Optional ROI calculator with explicit user assumptions, missing-input reporting, and break-even estimates.
- [x] `candidate --isolation container`: fail-closed hardened Docker runner (no network, dropped capabilities, read-only filesystem, non-root, resource limits, sanitized env, no Docker socket). Failure paths and flags unit-tested; runtime behaviour probed in the hosted CI `container-isolation` job. Not a VM-grade boundary; Docker Desktop untested.
- [x] ROI: migration cost input, report linkage (`roi --report`, estimate labeled, blocked for INCONCLUSIVE reports) and scenario sensitivity (`--scenarios`), with unit tests for zero/negative/NaN/Inf inputs.
- [x] Deterministic tests for benchmark aggregation, paired ordering, correctness gates, failed candidates, timeouts, report output, and invalid evidence.
- [x] Unit tests for analysis exclusions, report validation, and comparison guards.
- [x] Unit tests for ROI arithmetic, missing inputs, invalid values, and numeric overflow.
- [x] README, security notes, architecture, benchmark methodology, schema notes, and CI.
- [x] Select and add a project license.

## Planned

- [ ] Add and validate a real local database-backed example with shared fixtures and correctness tests.
- [ ] Add non-Go baselines, Podman/rootless-engine support, and a stronger boundary (gVisor/microVM) if untrusted code must be supported.
- [ ] Add application-specific benchmark adapters beyond the `--iterations`/`--input` contract.
- [ ] Add profiling adapters and richer evidence-based analysis.
- [ ] Embed ROI assumptions inside the exported benchmark report (currently `roi --report` reads a report; reports do not contain ROI).
- [ ] Expand report validation, import/export, redaction regression tests, and confidence-interval/statistical reliability analysis.
- [ ] Document supported compiler/platform combinations based on CI evidence.
- [ ] Consider optional AI candidate generation only after explicit configuration and local workflow maturity.

No production application profiling, candidate translation, resource measurements, hosted service, or AI integration is currently implemented.
