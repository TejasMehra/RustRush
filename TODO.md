# Implementation status

This status describes the checked-in implementation; items marked planned are not exposed as working CLI commands.

## Implemented and tested

- [x] Go CLI with `doctor`, `analyze`, `demo`, `report`, `compare`, and `roi`.
- [x] Local source inventory for common languages and low-confidence loop, serialization, database, and network hypotheses.
- [x] Dependency/build and credential-like path exclusions; analysis does not execute source.
- [x] Deterministic Go baseline and corresponding Rust integer-mixing candidate source.
- [x] Shared JSON integer-array fixture, Go transformation baseline, and dependency-free Rust candidate for the fixture shape.
- [x] Shared CSV filter-and-aggregate mock with Go baseline, Rust candidate source, and explicit non-database limitations.
- [x] Checksum correctness gate, warm-up, repeated process timings, raw JSON samples, and honest missing-compiler reporting.
- [x] Versioned JSON plus local Markdown reports and workload-compatible report comparison.
- [x] Optional ROI calculator with explicit user assumptions, missing-input reporting, and break-even estimates.
- [x] Unit tests for statistics, analysis exclusions, report validation, and comparison guards.
- [x] Unit tests for ROI arithmetic, missing inputs, invalid values, and numeric overflow.
- [x] README, security notes, architecture, benchmark methodology, schema notes, and CI.
- [x] Select and add a project license.

## Planned

- [ ] Compile and checksum-verify the Rust candidate end-to-end on a machine with a supported Rust toolchain; this environment has no Rust compiler or Windows C++ linker.
- [ ] Add and validate a real local database-backed example with shared fixtures and correctness tests.
- [ ] Add isolated candidate templates and application-specific benchmark adapters.
- [ ] Add profiling adapters and richer evidence-based analysis.
- [ ] Add sensitivity analysis and connect optional ROI assumptions to exported benchmark reports.
- [ ] Expand report validation, import/export, redaction regression tests, and statistical reliability analysis.
- [ ] Document supported compiler/platform combinations based on CI evidence.
- [ ] Consider optional AI candidate generation only after explicit configuration and local workflow maturity.

No production application profiling, candidate translation, resource measurements, ROI integration in benchmark reports, hosted service, or AI integration is currently implemented.
