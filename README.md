# RustRush

**Measure a backend operation in Go and Rust. Decide from local evidence.**

RustRush is an experimental, local-first Go CLI for developers evaluating a small,
hand-written Rust alternative to an existing Go operation. It checks matching
outputs before timed repetitions, collects paired process measurements, and keeps
invalid or insufficient comparisons `INCONCLUSIVE`.

RustRush does not translate arbitrary applications, profile production traffic, or
upload source code. A benchmark result for one operation is not a recommendation to
rewrite an entire backend.

## What it does

- Runs built-in CPU, JSON-fixture, and CSV-fixture demonstrations.
- Guides inspection of supported Go modules and reports static operation hypotheses.
- Benchmarks a Go `main` package against a single-file, dependency-free Rust candidate.
- Requires a matching correctness warm-up before measured trials; repetitions alternate order.
- Writes raw JSON, a readable Markdown report, and an SVG summary card locally.
- Offers optional Docker isolation for the candidate workflow and assumption-driven ROI scenarios.
- Reports `INCONCLUSIVE` rather than inventing missing measurements or speedups.

## Requirements

| Workflow | Requirements |
|---|---|
| Build, analyze, guide, or run the demo | Go 1.22 or newer |
| Complete Go-versus-Rust comparison | Rust `rustc` on `PATH`; on Windows, the MSVC Rust toolchain also needs Visual C++ build tools and the Windows SDK |
| `candidate --isolation container` | Docker CLI, working Docker daemon, locally available Go and Rust images; Linux Docker is the verified environment |
| Build the website | Node.js 20.15+ and npm 10+ |

The demo works without Rust. Without `rustc`, the Go baseline may still be measured,
but the report marks the Rust candidate unavailable and the verdict `INCONCLUSIVE`.
Images are never pulled implicitly by container mode. Docker Desktop on Windows and
macOS has not been verified.

## Install and run

Clone the repository and build from its root:

```powershell
git clone https://github.com/TejasMehra/RustRush.git
cd RustRush
go build -o rustrush.exe ./cmd/rustrush
```

Run the no-API-key demo:

```powershell
go run ./cmd/rustrush demo
```

The default workload is a deterministic integer-mixing loop with 2,000,000
iterations and five measured repetitions per implementation. The first run for each
implementation is a correctness warm-up; its duration is discarded. Timed runs
begin only after stdout matches. Try the other checked-in workloads with:

```powershell
go run ./cmd/rustrush demo --workload json --iterations 10000
go run ./cmd/rustrush demo --workload database-fixture --iterations 1000
go run ./cmd/rustrush demo --iterations 5000000 --repetitions 7 --timeout 30s
```

The database example is a CSV fixture, **not** a database benchmark. The JSON
candidate supports only the built-in fixture shape.

### Reports

The demo writes these files into `results/` by default (or the directory passed to
`--out DIR`):

- `latest.json`: versioned raw samples, aggregates, environment, assumptions, and limitations.
- `latest.md`: readable measurements, uncertainty, rationale, and ROI guidance.
- `latest.svg`: a shareable summary using only report values; missing values remain missing.

Reports are generated from the current run. They contain process-level p50/p95/p99,
throughput, coefficient of variation, process failure counts, and available runtime
metadata. They do not contain request-level latency distributions or CPU/memory profiling.

Other report commands:

```powershell
go run ./cmd/rustrush report results/latest.json
go run ./cmd/rustrush compare results/latest.json results/another-run.json
go run ./cmd/rustrush analyze --json .
go run ./cmd/rustrush guide .
```

`compare` rejects incompatible workloads and timing modes. Reports from different
machines or toolchains are not controlled cross-machine comparisons.

### Troubleshooting

- Run `go run ./cmd/rustrush doctor` to see which local toolchains are available.
- If the Rust candidate is unavailable, install Rust and verify `rustc --version`;
  Rust-only comparisons remain `INCONCLUSIVE` until the candidate can be built and
  run. On Windows, install the Rust MSVC target plus Visual C++ build tools and SDK.
- Container mode requires a running daemon and preloaded images:
  `docker info`, `docker image ls`, and (if needed) the `docker pull` commands above.
  RustRush does not pull images or silently fall back to local execution.
- Reports default to `results/latest.json`, `results/latest.md`, and
  `results/latest.svg`; use `--out DIR` if that location is not writable. Inspect
  the Markdown report's rationale, limitations, and per-implementation errors when
  its verdict is `INCONCLUSIVE`.

## Try a supported backend

Start with non-executing project inspection:

```powershell
go run ./cmd/rustrush guide path\to\your\project
```

The guided candidate workflow supports a Go module with a `main` package. Static
analysis can identify Go entry points and rank possible operations from source
patterns; these are hypotheses, not profiled hotspots. Recognized Node, Python,
Java/Kotlin, Rust, Ruby, and PHP markers receive analysis-only guidance. Other
repositories are reported as unsupported rather than being treated as arbitrary
supported backends.

To compare an operation, isolate it behind a small Go `main` package and provide a
reviewed, dependency-free Rust single-file candidate. Both programs must accept
`--iterations N` and emit the same deterministic stdout for matching arguments.
Optionally pass the same fixture with `--input FILE`.

```powershell
go run ./cmd/rustrush candidate `
  --baseline examples/candidate-template/baseline `
  --candidate examples/candidate-template/candidate.rs `
  --input examples/candidate-template/input.txt `
  --allow-execute `
  --iterations 2000000 `
  --repetitions 5
```

`--allow-execute` is explicit consent to building and running the supplied programs.
RustRush copies the sources to a temporary directory, does not modify the originals,
and does not fetch Go dependencies (`GOPROXY=off`). Review code before execution:
default process mode has a timeout and minimal environment but is **not a sandbox**.

### Optional Docker-isolated candidate run

The container workflow is fail-closed. Pull the configured images yourself, then run
the same candidate command with `--isolation container`:

```powershell
docker pull golang:1.24
docker pull rust:1
go run ./cmd/rustrush candidate `
  --baseline examples/candidate-template/baseline `
  --candidate examples/candidate-template/candidate.rs `
  --input examples/candidate-template/input.txt `
  --allow-execute `
  --isolation container
```

The runner uses no network, a read-only root and input/source mounts, dropped
capabilities, non-root execution, resource limits, and a sanitized environment. It
does not mount the Docker socket, run privileged, or fall back to local execution if
Docker is unavailable. Container mode is verified in Linux CI; containers still
share the host kernel and are not a VM-grade boundary.

## Correctness, verdicts, and timing

The first run for each implementation acts as its warm-up and correctness probe.
RustRush discards those durations and starts paired, alternating repetitions only
when stdout matches. Matching output is a necessary check, not proof of semantic
equivalence. Build failures, execution errors, missing or invalid samples,
correctness mismatches, incompatible evidence, excessive variability, and
low-resolution container samples cannot support a comparison.

Valid evidence may lead to `PORT`, `OPTIMIZE_FIRST`, or `DONT_PORT`. Otherwise the
verdict is `INCONCLUSIVE`. A Rust candidate needs at least 10% higher measured
throughput to reach `PORT` (with each implementation's coefficient of variation at
or below 10%); these are investigation rules for this microbenchmark, not
statistical confidence or a promise of application performance.

- `process_end_to_end` (default): host monotonic process duration, including process
  startup and output capture.
- `container_process_isolated`: each correctness/warm-up run and repetition gets a
  fresh hardened container. The sample times the workload child, including its
  process startup and stdout capture; timer-helper startup, container lifecycle,
  builds, and Docker dispatch are excluded. Full invocation time is measured, and
  residual overhead is labeled as an estimate because clocks are not cross-calibrated.

Container timing resolution is an empirical minimum clock tick, not an accuracy
guarantee. Samples below the documented 1,000-tick screening budget are retained
but produce `INCONCLUSIVE`. The screen is not a universal noise threshold.
Fresh containers prevent process-created files from persisting between trials, but
host caches, kernel state, external load, and differences between baseline and
candidate images remain. Legacy `steady_state_in_container` reports can still be
read but are not emitted by current runs; timing modes are never mixed.

See [benchmark methodology](docs/benchmark-methodology.md) and
[report schema](docs/report-schema.md) for details.

## Measured values, estimates, and ROI

The report's process timings are observations from the current run. Optional ROI
analysis is a separate model using explicit assumptions for monthly operation
volume, CPU seconds per operation, infrastructure price, engineering effort, and
maintenance changes.

```powershell
go run ./cmd/rustrush roi --monthly-operations 3600000 `
  --baseline-cpu-seconds 0.001 --target-cpu-seconds 0.0005 `
  --cpu-hour-price 10 --engineering-hours 10 `
  --engineering-hourly-cost 100 --monthly-maintenance-cost-delta 1 `
  --scenarios
```

`roi --report results/latest.json` can use a conclusive report's measured median
wall-time ratio as a labeled proxy to estimate target CPU time from user-supplied
baseline CPU time. It does not measure CPU time. Scenario outputs are modeled
estimates, not forecasts or guaranteed savings; missing or inconclusive evidence
does not produce a report-derived savings claim. See the [ROI model](docs/roi-model.md).

## Security and privacy

Analysis reads source as text and does not execute it. The CLI works locally, does
not upload code, and does not require an API key. Process-level execution is not a
sandbox. Docker hardening reduces exposure but cannot remove the shared-kernel or
container-runtime risk; do not treat it as safe execution for untrusted code.
Reports omit source contents and environment variable values. Read
[SECURITY.md](SECURITY.md) before running user-supplied candidates.

## Supported environments and limitations

Hosted CI verifies Go 1.22 and 1.24 on Windows and Ubuntu, Rust candidate checks on
Ubuntu, and Docker container checks on Ubuntu. Local process mode does not require
Docker. Docker Desktop on Windows/macOS has not been tested.

RustRush currently supports the built-in workloads and a focused Go-to-single-file
Rust candidate contract. It does not provide production profiling, automatic code
translation, non-Go baselines, real database benchmarking, request-level latency,
CPU/memory measurement, confidence intervals, or AI generation. It is an
experimental/alpha developer tool; use representative workloads and your own
correctness tests before making migration decisions.

## Contributing and validation

From the repository root:

```powershell
go test -count=1 ./...
go vet ./...
go build ./cmd/rustrush
```

Website development and production build instructions are in
[website/README.md](website/README.md). To build it:

```powershell
cd website
npm ci
npm run typecheck
npm run build
npm run preview
```

Further reading: [architecture](ARCHITECTURE.md),
[contributing](CONTRIBUTING.md), [benchmark methodology](docs/benchmark-methodology.md),
[report schema](docs/report-schema.md), and [security](SECURITY.md).
