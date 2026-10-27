# RustRush

RustRush is a local-first experiment for measuring whether a small CPU-heavy operation is worth rewriting in Rust. It records real measurements, checks that the built-in Go and Rust workloads produce the same checksum, and reports `INCONCLUSIVE` rather than inventing a comparison when Rust is unavailable.

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
```

The demo writes `results/latest.json` and `results/latest.md`. To adjust its workload:

```powershell
go run ./cmd/rustrush demo --iterations 5000000 --repetitions 7 --timeout 30s
```

If `rustc` is installed, RustRush compiles the checked-in candidate into a temporary directory, verifies its checksum against the Go baseline, performs one warm-up and the requested trials, then calculates statistics from the raw samples. Otherwise it still measures the baseline and writes a report marking the candidate unavailable and the verdict inconclusive.

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
- Gates performance interpretation on a matching output checksum.
- Records raw process timings, nearest-rank p50/p95/p99, median throughput, environment metadata, assumptions, and limitations in a versioned JSON report.
- Renders and compares saved reports locally.

## Interpreting results

The benchmark is intentionally narrow. Timing includes process startup as well as workload execution, so small iteration counts are especially sensitive to launch overhead. The workload is synthetic, and matching checksums do not prove complete semantic equivalence. A measured `PORT` result is a reason to investigate the tested operation further, not evidence that an entire backend should be rewritten.

Static analysis results are hypotheses, not profiled hotspots. Database and network findings explicitly recommend measuring I/O before considering a language migration. The verdict requires at least three measured trials, a checksum match, and no more than 10% run-to-run coefficient of variation. A throughput difference within 10% yields `OPTIMIZE_FIRST`; a noisy or invalid experiment yields `INCONCLUSIVE`. RustRush does not currently collect CPU utilization, memory, allocation counts, or request latency distributions.

## Security and privacy

Analysis is local and does not execute scanned project code. It skips common dependency/build directories and credential-like files, limits source reads, and emits relative source locations rather than absolute paths. The built-in demo only compiles and runs the checked-in Rust example, uses a temporary build directory, avoids a shell, and applies a timeout. Temporary compilation is not a general-purpose sandbox; do not treat it as isolation for arbitrary generated or third-party code.

Reports contain no source contents or environment-variable values. The demo records OS, architecture, Go runtime, CPU count, and Rust compiler version when available. It makes no uploads or remote model calls.

## Current limitations

This is an initial vertical slice, not the full product vision. It does not yet generate candidates from user code, profile arbitrary applications, run JSON/database examples, calculate migration ROI, collect resource metrics, or support AI integrations. Static findings are simple pattern matches. The CLI intentionally does not advertise commands for these unfinished features.

See [TODO.md](TODO.md) for implementation status, [ARCHITECTURE.md](ARCHITECTURE.md) for module boundaries, [benchmark methodology](docs/benchmark-methodology.md), and [report schema](docs/report-schema.md).

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
