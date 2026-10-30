![RustRush GitHub banner](assets/rustrush-github-banner.png)

**Find out how much faster your backend hot paths could run in Rust before committing to a rewrite.**
RustRush compares one Go operation with a Rust candidate. Results depend on the workload,
implementation, compiler, and machine; a benchmark is evidence, not a promise of speedup.

[![Go 1.22+](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go)](go.mod)
[![MIT License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Website:** [rust-rush.netlify.app](https://rust-rush.netlify.app/)

## What is RustRush?

RustRush is a local-first Go CLI for developers deciding whether to explore a Rust implementation of a specific backend operation. It checks matching program output before timing, runs paired repetitions, and reports when the available evidence is insufficient.

It is for focused experiments, not automatic rewrites. You provide the Go baseline and the Rust candidate; RustRush does not translate code or profile production traffic. Results describe the tested operation and input, not an entire service.

## Features

- **Correctness before timing:** each implementation's warm-up output is compared before measured repetitions begin. Matching output is a necessary check, not proof of full semantic equivalence.
- **Paired measurements:** configurable repetitions alternate run order and record process durations, throughput, p50/p95/p99, variability, and process failure counts.
- **Multiple timing modes:** reports identify local process timing or the optional container runner; incompatible timing modes are not compared.
- **Local reports:** each run writes machine-readable JSON, a Markdown report, and an SVG summary.
- **Optional Docker execution:** the candidate workflow can run in fail-closed, hardened containers with resource limits and no network access by default.
- **Assumption-driven ROI:** estimate cost scenarios from explicitly supplied workload, infrastructure, and engineering inputs.
- **Project guidance:** inspect Go projects for candidate operations and classify other recognized project types for analysis only.

## How it works

1. Choose one operation and a representative input. `guide` can identify Go entry points and rank source-pattern hypotheses; it does not profile or execute the project.
2. Provide a Go `main` package and a dependency-free, single-file Rust candidate that implement the same operation.
3. RustRush builds the programs and runs one warm-up for each. The same arguments and input are passed to both; mismatched output blocks timing.
4. After the correctness gate passes, RustRush collects paired repetitions in alternating order and aggregates the observations.
5. Review the raw samples, warnings, environment, assumptions, and verdict in the generated reports.

## Quick start

**Prerequisite:** Go 1.22 or newer. Rust is optional for the built-in demo.

```sh
git clone https://github.com/TejasMehra/RustRush.git
cd RustRush
go run ./cmd/rustrush demo
```

The default demo runs a deterministic CPU workload with 2,000,000 iterations and five measured repetitions. It writes `results/latest.json`, `results/latest.md`, and `results/latest.svg` from the current run.

If `rustc` is unavailable, RustRush still measures the Go baseline, marks Rust unavailable, and returns an `INCONCLUSIVE` verdict. It does not substitute sample or hardcoded performance numbers. The demo needs no API key and does not upload source code.

Try the checked-in fixture workloads:

```sh
go run ./cmd/rustrush demo --workload json --iterations 10000
go run ./cmd/rustrush demo --workload database-fixture --iterations 1000
go run ./cmd/rustrush demo --iterations 5000000 --repetitions 7 --timeout 30s
```

The database fixture is a local CSV mock, not a database benchmark. The Rust JSON implementation supports the checked-in fixture shape, not arbitrary JSON.

## Installation and prerequisites

Build the CLI from the repository root:

```sh
go build ./cmd/rustrush
go run ./cmd/rustrush doctor
```

| Workflow | Requirements |
|---|---|
| Build, analyze, guide, or run the demo | Go 1.22+ |
| Measure a complete Go-versus-Rust pair | `rustc` on `PATH`; Windows MSVC builds also need the Visual C++ build tools and Windows SDK |
| `candidate --isolation process` | Go and Rust toolchains; this is process-level execution, not a sandbox |
| `candidate --isolation container` | Docker CLI and daemon, plus the configured Go and Rust images already available locally |
| Build or serve the website | Node.js 20.15+ and npm 10+ |

RustRush does not pull Docker images automatically. The container workflow is verified in Linux CI; Docker Desktop on Windows and macOS has not been verified.

Install [Go](https://go.dev/dl/) for the CLI. For Rust comparisons, install
[Rust with rustup](https://rustup.rs/); Windows builds require the MSVC toolchain,
Visual C++ build tools, and the Windows SDK.

## Running a comparison

The supported executable comparison is a Go module containing a `main` package against a single-file, dependency-free Rust source file. Each program must accept `--iterations N`, print the same deterministic output for matching inputs, and accept `--input FILE` when an input file is supplied. Go dependencies must be available offline; the workflow does not fetch them.

The repository includes a working example:

```sh
go run ./cmd/rustrush candidate --baseline examples/candidate-template/baseline --candidate examples/candidate-template/candidate.rs --input examples/candidate-template/input.txt --allow-execute --iterations 2000000 --repetitions 5
```

`--allow-execute` is required consent to build and run the supplied programs. RustRush copies source to a temporary directory and leaves the originals unchanged. The default process mode applies a timeout and minimal environment, but the processes still run with the user's permissions and filesystem/network access.

To select the Docker runner, preload its default images and add `--isolation container`:

```sh
docker pull golang:1.24
docker pull rust:1
go run ./cmd/rustrush candidate --baseline examples/candidate-template/baseline --candidate examples/candidate-template/candidate.rs --input examples/candidate-template/input.txt --allow-execute --isolation container
```

Other supported options include `--out DIR`, `--timeout DURATION`, and, for container mode, `--container-memory`, `--container-cpus`, and `--container-pids`. Run `go run ./cmd/rustrush candidate --help` for the complete current flag list.

`guide` recognizes some Node.js, Python, Java/Kotlin, Rust, Ruby, and PHP project markers for **analysis-only** guidance. Those languages are not executable baselines in the paired candidate workflow. The guided candidate workflow currently targets Go modules with a `main` package; it does not claim support for arbitrary repositories.

## Understanding the report

Every `demo` or `candidate` run writes three files to `results/` by default, or to the directory passed with `--out DIR`:

| File | Contents |
|---|---|
| `latest.json` | Versioned report with raw observations, aggregates, environment metadata, warnings, and assumptions |
| `latest.md` | Human-readable comparison, interpretation notes, and limitations |
| `latest.svg` | Shareable summary generated from report values; unavailable values remain unavailable |

The correctness field is `PASSED_FOR_CHECKSUM`, `FAILED`, or `NOT_RUN`. The verdict is one of:

- `PORT`: measured Rust throughput is at least 10% higher and both implementations' coefficients of variation are no more than 10%.
- `OPTIMIZE_FIRST`: the valid throughput difference is within the ±10% screening band.
- `DONT_PORT`: measured Rust throughput is at least 10% lower.
- `INCONCLUSIVE`: correctness failed or could not be established, required measurements are missing or invalid, execution failed, or measurement-quality checks reject the comparison.

These are screening rules for one local experiment, not statistical confidence or a recommendation to rewrite a whole service. At least three valid paired repetitions are required. Container samples below 1,000 empirically observed clock ticks are retained but cannot support a verdict; this is a timer-resolution screen, not an accuracy guarantee. No confidence intervals are calculated.

Timing modes are intentionally distinct:

- `process_end_to_end` is the default local mode. Each sample includes process startup, execution, and output capture.
- `container_process_isolated` uses a fresh container per run. Its sample includes workload-process startup and output capture, but excludes container lifecycle, compilation, timer-helper startup, and Docker dispatch. Full invocation time and a derived overhead estimate are reported separately.

Both modes report process-duration percentiles, not request-level latency distributions. Error counts describe failed benchmark process attempts, not application request errors. Rust and Go use different compilers and container images; host load, caches, and kernel state can affect measurements.

Read [Benchmark methodology](docs/benchmark-methodology.md) and [Report schema](docs/report-schema.md) for details. To render or compare saved reports:

```sh
go run ./cmd/rustrush report results/latest.json
go run ./cmd/rustrush compare results/latest.json results/another-run.json
```

Comparison rejects incompatible workloads and timing modes. Reports from different machines or toolchains are not controlled cross-machine comparisons.

## ROI modeling

ROI is a separate model, not measured production savings. Supply workload volume, CPU time per operation, infrastructure price, engineering effort and rate, and the monthly maintenance-cost change. For a break-even calculation, include the maintenance delta; missing inputs are reported rather than silently treated as zero.

```sh
go run ./cmd/rustrush roi --monthly-operations 3600000 --baseline-cpu-seconds 0.001 --target-cpu-seconds 0.0005 --cpu-hour-price 10 --engineering-hours 10 --engineering-hourly-cost 100 --monthly-maintenance-cost-delta 1 --migration-cost 500 --scenarios
```

`--report results/latest.json` can estimate target CPU time from a conclusive report's measured wall-time ratio when `--target-cpu-seconds` is omitted. Wall time is only a proxy for CPU time; the result remains labeled as an estimate. Scenario multipliers are illustrative assumptions, not forecasts or guaranteed savings. See the [ROI model documentation](docs/roi-model.md).

## Security and isolation

The demo runs only built-in workloads. Candidate execution requires explicit `--allow-execute`.

- **Process mode** uses a timeout and a minimal environment. It is not a sandbox and does not restrict filesystem, network, or user permissions.
- **Container mode** is optional for `candidate`. It fails closed if Docker or an image is unavailable, and uses no network, a read-only root filesystem, restricted mounts, dropped capabilities, non-root execution, resource limits, and a sanitized environment. It does not mount the Docker socket or run privileged.

Containers share the host kernel and are not a VM-grade security boundary. Review candidate code before running it; do not treat container mode as a guarantee for untrusted code. See [SECURITY.md](SECURITY.md) and [Architecture](ARCHITECTURE.md).

## Website

Visit [rust-rush.netlify.app](https://rust-rush.netlify.app/) for a static introduction to RustRush, its benchmark method, how to read evidence, and how to run the local demo. The site does not run benchmarks, collect analytics, or call a backend. The report preview is illustrative.

## Development and testing

From the repository root:

```sh
go test -count=1 ./...
go vet ./...
go build ./cmd/rustrush
```

For website development and checks, use Node.js 20.15+ and npm 10+:

```sh
cd website
npm ci
npm run typecheck
npm run dev
npm run build
```

The static production output is `website/dist`. Netlify uses `website/` as its base directory; see [netlify.toml](netlify.toml).

## Project structure

| Path | Purpose |
|---|---|
| `cmd/rustrush/` | CLI commands and flags |
| `internal/rustrush/` | Workloads, candidate execution, reports, analysis, ROI, and tests |
| `examples/` | Built-in candidates, fixtures, and the Go/Rust candidate template |
| `docs/` | Benchmark methodology, report schema, and ROI model |
| `assets/` | Official brand artwork and favicon files |
| `website/` | Static React, TypeScript, and Vite site |
| `.github/workflows/ci.yml` | Go, Rust-candidate, and Docker-container CI checks |
| `ARCHITECTURE.md`, `SECURITY.md`, `CONTRIBUTING.md`, `TODO.md` | Design, security limits, contribution guide, and implementation status |

## Roadmap and limitations

RustRush does not currently run a real database workload, profile production applications, collect CPU or memory utilization, translate or generate candidates, support non-Go executable baselines, or calculate confidence intervals. Static operation rankings are hypotheses, not measured hotspots.

The tracked roadmap includes a real database-backed example, additional baseline and benchmark adapters, profiling integrations, stronger isolation options, richer report validation, and further statistical analysis. See [TODO.md](TODO.md) for the current status.

## Contributing and license

See [CONTRIBUTING.md](CONTRIBUTING.md) for project guidance and validation commands. RustRush is licensed under the [MIT License](LICENSE).
