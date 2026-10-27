# Benchmark methodology

## Built-in workloads

`demo --workload cpu` compares a deterministic 64-bit integer-mixing loop implemented in Go and Rust. Each process receives the same `--iterations` value and prints its final checksum. The checksum is checked before interpreting performance.

`demo --workload json` reads the shared integer-array fixture, doubles each signed integer, and serializes the result repeatedly. Go uses the standard JSON library. The dependency-free Rust implementation only supports this built-in fixture shape; it is not a general JSON parser. Both implementations must emit identical JSON before performance is interpreted.

`demo --workload database-fixture` parses a local CSV fixture and sums the `amount` field for active rows. It is a dependency-free mock, not a database benchmark: it excludes query planning, connections, transactions, indexes, concurrency, and database/network latency.

The default is 2,000,000 iterations and five measured repetitions per implementation. Each process gets one warm-up run. Every measured run is separately timed. The timer includes process startup, argument parsing, workload execution, and output capture. Rust compilation is outside the measurement interval.

The Rust source is compiled with `rustc --edition=2021 -C opt-level=3`. Go uses the binary built by `go run` or `go build`; these compiler settings are not equivalent, and the report records the Go runtime and Rust compiler version when available.

## Statistics

Raw nanosecond durations are retained for each measured repetition. Throughput is calculated as `iterations / median(duration)`. p50, p95, and p99 use the nearest-rank method over trial durations; with five trials p95/p99 are effectively the maximum observation and should not be read as request-level tail latency.

No speedup is preloaded. If Rust is unavailable, the report includes baseline observations only and marks the verdict inconclusive. Failed builds, failed runs, and checksum mismatches cannot produce a positive verdict. The verdict requires at least three trials and rejects a comparison when either implementation's population coefficient of variation exceeds 10%. A measured throughput change of at least +10% yields `PORT`, at least -10% yields `DONT_PORT`, and a smaller change yields `OPTIMIZE_FIRST`. These thresholds are simple screening rules, not a statistical guarantee.

## Interpretation limits

These workloads are useful for exercising the CLI, not for estimating application-wide benefits. The CPU workload is synthetic, and the JSON and CSV examples are constrained fixtures. Process launch time can dominate short runs. Results are sensitive to background load, power management, operating system, compiler version, and machine. Rust and Go compile flags and runtimes differ. Repeat with representative application inputs and workloads before making an engineering decision.

The implementation does not currently measure CPU utilization, memory, allocations, concurrency, error rates, or system-wide noise. It does not perform confidence-interval analysis. `PORT`/`DONT_PORT` use a simple threshold on this single local test and are suggestions only.

## Reproduction

Run from the repository root:

```powershell
go run ./cmd/rustrush demo --iterations 2000000 --repetitions 5
```

Use `--workload json` or `--workload database-fixture` to run the fixture examples. The JSON report retains raw timings and environment information. Do not compare results across different environments as a controlled experiment.
