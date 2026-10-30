# Implementation status

This status describes the checked-in implementation; items marked planned are not exposed as working CLI commands.

## Implemented and tested

- [x] Go CLI with `doctor`, `analyze`, `guide`, `candidate`, `demo`, `report`, `compare`, and `roi`.
- [x] `guide`: non-executing project-type detection (Go module = candidate workflow; other markers = analysis only; otherwise unsupported) and ranked Go candidate-operation hypotheses.
- [x] `candidate`: explicit-consent workflow for a user Go main package or single-file Python baseline against a single-file Rust candidate. It copies source, syntax-checks Python, applies the shared correctness gate, and reuses paired measurements and reports. Go is supported in process or container mode; Python is process-mode only.
- [x] Tests for guide classification, Go and Python baseline validation, unsupported isolation, consent, source preservation, failed/timed-out candidates, syntax failures, missing runtimes/compiler, and correctness-gate behaviour.
- [x] Markdown report keeps tables intact and surfaces measurement errors.
- [x] Local source inventory for common languages and low-confidence loop, serialization, database, and network hypotheses.
- [x] Dependency/build and credential-like path exclusions; analysis does not execute source.
- [x] Deterministic Go baseline and corresponding Rust integer-mixing candidate source.
- [x] Shared JSON integer-array fixture, Go transformation baseline, and dependency-free Rust candidate for the fixture shape.
- [x] Shared CSV filter-and-aggregate mock with Go baseline, Rust candidate source, and explicit non-database limitations.
- [x] Rust-enabled Ubuntu CI compiles all three candidates and asserts correctness, warm-up, error-count, and repeated-sample report fields; this does not claim a performance win.
- [x] Python-to-Rust example and Rust-enabled hosted CI workflow validate a successful correctness-gated report without asserting a speedup.
- [x] Untimed cross-language correctness gate before timing; one warm-up and configurable paired repetitions with alternating run order.
- [x] Raw process timings, throughput, process p50/p95/p99, process error counts, metadata, and inconclusive handling for invalid, failed, missing, or noisy evidence.
- [x] Versioned JSON plus measured-versus-estimated Markdown reporting, raw samples, and workload-compatible report comparison.
- [x] Optional ROI calculator with explicit user assumptions, missing-input reporting, and break-even estimates.
- [x] `candidate --isolation container`: fail-closed hardened Docker runner (no network, dropped capabilities, read-only filesystem, non-root, resource limits, sanitized env, no Docker socket). Failure paths and flags unit-tested; runtime behaviour probed in the hosted CI `container-isolation` job. Not a VM-grade boundary; Docker Desktop untested.
- [x] Container timing methodology: fresh hardened containers per correctness probe, warm-up, and measurement; monotonic timer helper; base64-separated workload output; empirical clock tick metadata; lifecycle/dispatch excluded from samples, invocation wall time measured, and residual overhead labeled as a derived estimate. Malformed or duplicate timer fields, failed execution/cleanup, and samples below 1,000 clock ticks cannot produce a verdict. Hosted CI validates the real Docker path; host caches/kernel state and different Go/Rust images remain measurement limitations.
- [x] ROI: migration cost input, report linkage (`roi --report`, estimate labeled, blocked for INCONCLUSIVE reports) and scenario sensitivity (`--scenarios`), with unit tests for zero/negative/NaN/Inf inputs.
- [x] Report quality: `latest.svg` shareable card generated only from measured values (well-formedness, XML-escaping and missing-data tests), Markdown Uncertainty and ROI-pointer sections. SVG layout and element bounds were inspected in the integrated browser at 760 px for passing, inconclusive, failed-candidate, and long-label examples.
- [x] Deterministic tests for benchmark aggregation, paired ordering, correctness gates, failed candidates, timeouts, report output, and invalid evidence.
- [x] Unit tests for analysis exclusions, report validation, and comparison guards.
- [x] Unit tests for ROI arithmetic, missing inputs, invalid values, and numeric overflow.
- [x] README, security notes, architecture, benchmark methodology, schema notes, and CI.
- [x] Select and add a project license.

## Planned

- [ ] Add and validate a real local database-backed example with shared fixtures and correctness tests.
- [ ] Add Python project guidance and Python container execution only with a separately verified hardened image and runtime contract.
- [ ] Add Podman/rootless-engine support and a stronger boundary (gVisor/microVM) if untrusted code must be supported.
- [ ] Add application-specific benchmark adapters beyond the `--iterations`/`--input` contract.
- [ ] Add profiling adapters and richer evidence-based analysis.
- [ ] Embed ROI assumptions inside the exported benchmark report (currently `roi --report` reads a report; reports do not contain ROI).
- [ ] Expand report validation, import/export, redaction regression tests, and confidence-interval/statistical reliability analysis.
- [ ] Document supported compiler/platform combinations based on CI evidence.
- [ ] Consider optional AI candidate generation only after explicit configuration and local workflow maturity.

No production application profiling, candidate translation, resource measurements, hosted service, or AI integration is currently implemented.
