# RustRush

RustRush is a local-first experiment for measuring whether a small backend operation is worth rewriting in Rust. When Rust is available, it records real measurements and checks that built-in Go and Rust workloads produce matching outputs; otherwise it reports `INCONCLUSIVE` rather than inventing a comparison.

## Requirements

- Go 1.22 or newer
- Rust `rustc` on `PATH` to build and measure the Rust candidate

The CLI, source scanner, and baseline run without Rust. No API key, telemetry, or network connection is required.

For the Rust comparison, install Rust with [rustup](https://rustup.rs/). On Windows, the MSVC Rust toolchain also requires the Visual C++ build tools and Windows SDK.

## First run (no API key, no Rust required)

Prerequisites: Go 1.22+ and a clone of this repository. Nothing is uploaded and no account is needed.

```powershell
git clone https://github.com/TejasMehra/RustRush.git
cd RustRush
go run ./cmd/rustrush demo
```

What you should see: the Markdown report printed to the terminal, then `Reports written to results/latest.json and results/latest.md`. Both files are regenerated from the current run; no performance number is hardcoded. Open `results/latest.md` to read it.

- **Without `rustc`** (verified locally on Windows): the Go baseline is measured for real, the Rust candidate shows `unavailable`, the verdict is `INCONCLUSIVE`, and the command exits 0. This is not a Go-vs-Rust comparison.
- **With `rustc`**: the checked-in Rust candidate is compiled, and each implementation's first warm-up doubles as the correctness probe. Its duration is discarded; timed repetitions begin only after outputs match. The verdict can then be `PORT`, `OPTIMIZE_FIRST`, `DONT_PORT`, or `INCONCLUSIVE` (too noisy). Rust compilation and comparison are verified only in the hosted CI `rust-demo` job, not on the author's machine.

Look at these report fields first: `verdict` and `reason`, `correctness` (`PASSED_FOR_CHECKSUM` means the gate passed), each measurement's `status`, `error_count`, and `samples`, and `environment`. Use `--workload json` or `--workload database-fixture` for the other built-in workloads, and `--out DIR` to keep several runs.

## Try it on your own backend

`rustrush guide` checks, without executing anything, whether your project can use the guided workflow:

```powershell
go run ./cmd/rustrush guide path\to\your\project
```

Supported project types:

| Project | Level | What you get |
|---|---|---|
| Go module (`go.mod`) | `candidate_workflow` | Candidate operations (functions with loops, JSON, DB or network calls, ranked as static hypotheses), main packages found, next steps |
| Node, Python, Java/Kotlin, Rust, Ruby, PHP markers | `analysis_only` | Static findings only; the paired benchmark needs a Go baseline |
| Anything else | `unsupported` | An explanation and pointers; no support for arbitrary repositories is claimed |

Then benchmark one operation you have isolated into a small Go `main` package against a hand-written, dependency-free, single-file Rust port. A working template lives in [examples/candidate-template](examples/candidate-template):

```powershell
go run ./cmd/rustrush candidate `
  --baseline examples/candidate-template/baseline `
  --candidate examples/candidate-template/candidate.rs `
  --input examples/candidate-template/input.txt `
  --allow-execute
```

Contract: both programs accept `--iterations N` (and `--input FILE` when you pass one) and print one deterministic result to stdout. RustRush then:

1. Refuses to run unless you pass `--allow-execute`, since it builds and runs the programs you point it at (review them first).
2. Copies the baseline module and candidate into a temporary directory (originals are never modified; `.env`, credential-like files, symlinks, and dependency/build directories are not copied) and builds there. Go builds use `GOPROXY=off` and `-mod=readonly`, so no network fetches; a baseline that needs uncached dependencies reports `build_failed`.
3. Rejects candidates using `include!`/`include_str!`/`include_bytes!`, `extern crate`, external `mod x;`, or linker attributes.
4. Uses each implementation's warm-up as its correctness probe and discards that duration. Paired measured repetitions start only after outputs are identical; reports use the same schema and paired benchmark engine as the demo.

By default this is process-level isolation, not a sandbox: programs run with a timeout, a minimal environment, no shell, and an empty working directory, but with your user's permissions, filesystem and network. Do not run code you do not trust. RustRush does not generate or translate code and does not upload anything.

### Optional container isolation

```powershell
docker pull golang:1.24; docker pull rust:1
go run ./cmd/rustrush candidate --isolation container --allow-execute `
  --baseline examples/candidate-template/baseline `
  --candidate examples/candidate-template/candidate.rs `
  --input examples/candidate-template/input.txt
```

`--isolation container` builds and runs both programs in hardened Docker containers: no network (`--network none`), all capabilities dropped, `no-new-privileges`, read-only root filesystem, read-only source/binary/input mounts, a small `noexec` tmpfs (executable only for builds), non-root user, memory/CPU/PID limits (`--container-memory`, `--container-cpus`, `--container-pids`), and a sanitized environment. The Docker socket is never mounted and nothing runs privileged. Images are never pulled implicitly.

It fails closed: if the Docker CLI or daemon is missing, an image is not present locally, the host user is root, or a setting is invalid, the command exits with an error and does **not** fall back to local execution (no report is written).

Verification status: the hardening flags and failure paths are unit-tested with an injected Docker executor, and the hosted CI `container-isolation` job runs a probe inside a real container (non-root, no network interfaces beyond loopback, zero effective capabilities, no-new-privileges, read-only root and mounts, no host environment or Docker socket visible) plus a full `candidate --isolation container` run. Limitations: containers share the host kernel and are not a VM-grade boundary; Docker on Linux is the supported engine. Docker Desktop on Windows/macOS is untested. Each correctness, warm-up, and measured invocation uses a fresh `docker run --rm` container. The Go timer helper uses a monotonic clock around the workload child; helper startup, container lifecycle, compilation, and Docker dispatch are excluded from samples. Full container invocation time is measured; residual overhead is a derived estimate because clocks are not cross-calibrated. Timer resolution is an empirical minimum clock tick, not an accuracy guarantee. Samples spanning fewer than 1,000 observed ticks are retained but make the verdict `INCONCLUSIVE`; this is a nominal quantization screen, not an accuracy bound. Baseline and candidate use different images; baselines build offline (`GOPROXY=off`). Fresh container filesystems prevent process-created files from persisting between trials, but host caches, kernel state, and external machine load are not reset. Reports record isolation, image IDs, timing mode, and timer limitations.

## Quick start

From the repository root:

```powershell
go test ./...
go run ./cmd/rustrush doctor
go run ./cmd/rustrush analyze .
go run ./cmd/rustrush guide .
go run ./cmd/rustrush demo
go run ./cmd/rustrush demo --workload json --iterations 10000
go run ./cmd/rustrush demo --workload database-fixture --iterations 1000
go run ./cmd/rustrush roi --monthly-operations 3600000 --baseline-cpu-seconds 0.001 --target-cpu-seconds 0.0005 --cpu-hour-price 10 --engineering-hours 10 --engineering-hourly-cost 100 --monthly-maintenance-cost-delta 1
```

The demo writes `results/latest.json`, `results/latest.md` and a shareable `results/latest.svg` card (verdict, correctness status, measured p50/p95/p99/throughput/CV per implementation, hardware and runtime metadata; implementations that could not be measured show `n/a`, never zero). The Markdown report also has an Uncertainty section (trials, min/median/max, CV against the 10% threshold; no confidence intervals are computed) and points to `roi --report` for cost assumptions. The SVG is tested for valid XML, escaping, missing-data handling, and timing-mode labels. Its rendered layout has also been checked in the integrated browser at 760 px for passing, inconclusive, failed-candidate, and long-label examples. To adjust its workload:

```powershell
go run ./cmd/rustrush demo --iterations 5000000 --repetitions 7 --timeout 30s
```

If `rustc` is installed, RustRush compiles the checked-in candidate into a temporary directory. Each implementation's first warm-up also acts as its correctness probe; the duration is discarded, and the requested paired repetitions start only when both outputs match. Trial order alternates which implementation runs first. Otherwise it still measures the Go baseline and writes a report marking the candidate unavailable and the verdict inconclusive.

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
- Calculates optional migration ROI from explicit user assumptions (plus an optional one-off migration cost). With `--report`, it links a saved benchmark report: the target CPU time is then an estimate derived from the measured wall-clock ratio, and only when the report is conclusive. `--scenarios` adds illustrative conservative/expected/optimistic sensitivity cases. Savings are never claimed when inputs are missing or the report is `INCONCLUSIVE`.

## Release status and interpreting results

Supported environments verified by hosted CI are Go 1.22 and 1.24 on Windows and Ubuntu. Container execution is verified on Ubuntu with Docker Linux images; Windows/macOS Docker Desktop has not been tested. Local process mode does not require Docker. A local Rust compiler is required for a completed Go/Rust comparison; without `rustc`, the baseline may be measured but the verdict remains inconclusive.

The benchmark is intentionally narrow. Both timing modes report process-duration percentiles, not request-level latency distributions. Local mode (`environment.timing_mode = process_end_to_end`) uses Go's monotonic clock and includes process startup and output capture. Container mode (`container_process_isolated`) builds both implementations first, then uses a fresh hardened container for each correctness probe, warm-up, and paired measurement. A Go helper times the workload child with `time.Since`; workload process startup and output capture are included, while helper startup, container lifecycle, compilation, and Docker dispatch are excluded from sample duration. Full invocation wall time is measured; the residual overhead is a derived estimate because the child and host clocks are not cross-calibrated. Neither value is a benchmark sample. Reported clock resolution is an empirical minimum tick, not a timer-accuracy guarantee. Fresh container filesystems prevent per-process files from carrying between repetitions, but host caches and kernel state remain. Baseline and candidate images differ. Older `steady_state_in_container` reports remain readable; the two current timing modes and legacy reports are never mixed by `compare`.

`INCONCLUSIVE` means RustRush lacks valid evidence for a recommendation: correctness was not established, a build/process/timer failed, measurements are missing or invalid, CV exceeds 10%, or a container sample spans fewer than 1,000 observed clock ticks. Raw observations are preserved where available. The tick budget is a quantization screen only, not a universal noise cutoff or accuracy bound. Repeat on a quieter machine with longer workloads and representative inputs.

ROI is a model, not measured savings: workload volume, CPU time, prices, engineering effort and maintenance deltas are user assumptions. If a benchmark is linked, its measured median-duration ratio is only a labeled proxy to estimate target CPU time from user-supplied baseline CPU seconds. Scenarios are illustrative, not forecasts. No confidence intervals are computed and no modeled saving is a guarantee. Containers share the host kernel; host caches are not reset, and CPU/memory utilization is not measured. Matching checksums do not prove semantic equivalence; a measured `PORT` result is not evidence that an entire backend should be rewritten.

Static analysis results are hypotheses, not profiled hotspots. Database and network findings explicitly recommend measuring I/O before considering a language migration. The verdict requires at least three matching paired trials, a passed correctness gate, valid aggregates, and no more than 10% run-to-run coefficient of variation. A throughput difference within 10% yields `OPTIMIZE_FIRST`; a noisy or invalid experiment yields `INCONCLUSIVE`. RustRush does not currently collect CPU utilization, memory, allocation counts, per-request latency distributions, or request-level error rates; its error counts describe failed benchmark process attempts.

## Security and privacy

Analysis is local and does not execute scanned project code. It skips common dependency/build directories and credential-like files, limits source reads, and emits relative source locations rather than absolute paths. The built-in demo only compiles and runs the checked-in Rust example, uses a temporary build directory, avoids a shell, and applies a timeout. Temporary compilation is not a general-purpose sandbox; do not treat it as isolation for arbitrary generated or third-party code.

Reports contain no source contents or environment-variable values. The demo records OS, architecture, Go runtime/compiler, CPU count, Go `GOMAXPROCS`, and Rust compiler version when available. It makes no uploads or remote model calls.

## Current limitations

This is an initial vertical slice, not the full product vision. The `candidate` workflow benchmarks hand-written candidates for a single Go `main` package; it does not generate or translate code, profile applications, handle Go modules with uncached dependencies, or support non-Go baselines. It does not yet benchmark a real database, collect resource metrics, or support AI integrations. Static findings and `guide` candidate operations are simple pattern matches.

See [TODO.md](TODO.md) for implementation status, [ARCHITECTURE.md](ARCHITECTURE.md) for module boundaries, [benchmark methodology](docs/benchmark-methodology.md), [report schema](docs/report-schema.md), and the [ROI model](docs/roi-model.md).

## Troubleshooting

- **`rustc was not found on PATH`:** Install Rust with rustup, reopen the terminal, and confirm `rustc --version` works.
- **Rust candidate build fails on Windows:** Install the Visual C++ build tools and Windows SDK, then rerun the demo. RustRush reports build failures instead of reporting a speedup.
- **Verdict is `INCONCLUSIVE`:** Check the report for unavailable tools, checksum errors, or high trial variability. A report without a Rust measurement is not a Go-vs-Rust comparison.
- **`candidate` says `refusing to build and run`:** Add `--allow-execute` after reviewing the code.
- **`candidate` reports `build_failed` for the baseline:** The module must build offline from a temporary copy (`GOPROXY=off`); vendor or pre-download its dependencies, or isolate the operation in a dependency-free main package.
- **Correctness `FAILED`:** The two programs printed different (or empty) stdout for the same arguments; no timing was done. Fix the candidate first.

## Development

```powershell
gofmt -w (Get-ChildItem -Recurse -Filter *.go | ForEach-Object FullName)
go test ./...
go vet ./...
go build ./cmd/rustrush
```

See [CONTRIBUTING.md](CONTRIBUTING.md). RustRush is distributed under the MIT License; see [LICENSE](LICENSE).
