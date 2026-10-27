# RustRush

RustRush is a local-first experiment for measuring whether a small backend operation is worth rewriting in Rust. When Rust is available, it records real measurements and checks that built-in Go and Rust workloads produce matching outputs; otherwise it reports `INCONCLUSIVE` rather than inventing a comparison.

## Requirements

- Go 1.22 or newer
- Rust `rustc` on `PATH` to build and measure the Rust candidate

The CLI, source scanner, and baseline run without Rust. No API key, telemetry, or network connection is required.

For the Rust comparison, install Rust with [rustup](https://rustup.rs/). On Windows, the MSVC Rust toolchain also requires the Visual C++ build tools and Windows SDK.

## Quick start

From the repository root:

```powershell
go test ./...
go run ./cmd/rustrush doctor
go run ./cmd/rustrush analyze .
go run ./cmd/rustrush demo
go run ./cmd/rustrush demo --workload json --iterations 10000
go run ./cmd/rustrush demo --workload database-fixture --iterations 1000
go run ./cmd/rustrush roi --monthly-operations 3600000 --baseline-cpu-seconds 0.001 --target-cpu-seconds 0.0005 --cpu-hour-price 10 --engineering-hours 10 --engineering-hourly-cost 100 --monthly-maintenance-cost-delta 1
```

The demo writes `results/latest.json` and `results/latest.md`. To adjust its workload:

```powershell
go run ./cmd/rustrush demo --iterations 5000000 --repetitions 7 --timeout 30s
```

If `rustc` is installed, RustRush compiles the checked-in candidate into a temporary directory and runs untimed correctness probes first. Timed runs start only when both outputs match. It then performs one warm-up per implementation and the requested number of paired repetitions, alternating which implementation runs first. Otherwise it still measures the Go baseline and writes a report marking the candidate unavailable and the verdict inconclusive.

Other useful commands:

```powershell
go run ./cmd/rustrush analyze --json .
go run ./cmd/rustrush report results/latest.json
go run ./cmd/rustrush compare results/latest.json results/another-run.json
```

`compare` refuses to compare different workload definitions and warns that results from different machines or toolchains are not directly comparable.

## What this version does

- Checks Go and Rust toolchain availability with `doctor`.
- Scans source files without executing them. It detects a small set of languages and reports low-confidence static hypotheses for loops, JSON, database, and network patterns.
- Runs one deterministic integer-mixing CPU workload implemented in Go and Rust.
- Runs a fixture-based JSON integer-array transformation implemented in Go and Rust; the Rust parser is deliberately limited to the built-in fixture shape.
- Runs a CSV-backed filter-and-aggregate mock implemented in Go and Rust; it is explicitly not a real database benchmark.
- Requires matching correctness probes before timing; output changes during trials invalidate the comparison.
- Records raw process timings, nearest-rank p50/p95/p99 process latency, median throughput, process failure counts, and OS/runtime metadata in versioned JSON plus a Markdown comparison report.
- Reports `INCONCLUSIVE` when measurements are missing, malformed, failed, or too variable. Markdown labels observed benchmark results as measured, not estimated.
- Renders and compares saved reports locally.
- Calculates optional migration ROI from explicit user assumptions; it does not infer CPU costs or engineering effort from benchmark timings.

## Interpreting results

The benchmark is intentionally narrow. Reported p50/p95/p99 values describe whole-process wall-clock durations across repetitions; they are not per-request latency distributions. Timing includes process startup as well as workload execution, so small iteration counts are especially sensitive to launch overhead. The workload is synthetic, and matching checksums do not prove complete semantic equivalence. A measured `PORT` result is a reason to investigate the tested operation further, not evidence that an entire backend should be rewritten.

Static analysis results are hypotheses, not profiled hotspots. Database and network findings explicitly recommend measuring I/O before considering a language migration. The verdict requires at least three matching paired trials, a passed correctness gate, valid aggregates, and no more than 10% run-to-run coefficient of variation. A throughput difference within 10% yields `OPTIMIZE_FIRST`; a noisy or invalid experiment yields `INCONCLUSIVE`. RustRush does not currently collect CPU utilization, memory, allocation counts, per-request latency distributions, or request-level error rates; its error counts describe failed benchmark process attempts.

## Security and privacy

Analysis is local and does not execute scanned project code. It skips common dependency/build directories and credential-like files, limits source reads, and emits relative source locations rather than absolute paths. The built-in demo only compiles and runs the checked-in Rust example, uses a temporary build directory, avoids a shell, and applies a timeout. Temporary compilation is not a general-purpose sandbox; do not treat it as isolation for arbitrary generated or third-party code.

Reports contain no source contents or environment-variable values. The demo records OS, architecture, Go runtime/compiler, CPU count, Go `GOMAXPROCS`, and Rust compiler version when available. It makes no uploads or remote model calls.

## Current limitations

This is an initial vertical slice, not the full product vision. It does not yet generate candidates from user code, profile arbitrary applications, benchmark a real database, collect resource metrics, or support AI integrations. Static findings are simple pattern matches. The CLI intentionally does not advertise commands for these unfinished features.

See [TODO.md](TODO.md) for implementation status, [ARCHITECTURE.md](ARCHITECTURE.md) for module boundaries, [benchmark methodology](docs/benchmark-methodology.md), [report schema](docs/report-schema.md), and the [ROI model](docs/roi-model.md).

## Troubleshooting

- **`rustc was not found on PATH`:** Install Rust with rustup, reopen the terminal, and confirm `rustc --version` works.
- **Rust candidate build fails on Windows:** Install the Visual C++ build tools and Windows SDK, then rerun the demo. RustRush reports build failures instead of reporting a speedup.
- **Verdict is `INCONCLUSIVE`:** Check the report for unavailable tools, checksum errors, or high trial variability. A report without a Rust measurement is not a Go-vs-Rust comparison.

## Development

```powershell
gofmt -w (Get-ChildItem -Recurse -Filter *.go | ForEach-Object FullName)
go test ./...
go vet ./...
go build ./cmd/rustrush
```

See [CONTRIBUTING.md](CONTRIBUTING.md). RustRush is distributed under the MIT License; see [LICENSE](LICENSE).
