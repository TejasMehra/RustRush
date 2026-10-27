# Benchmark methodology

## Built-in workloads

`demo --workload cpu` compares a deterministic 64-bit integer-mixing loop implemented in Go and Rust. Each process receives the same `--iterations` value and prints its final checksum. The checksum is checked before interpreting performance.

`demo --workload json` reads the shared integer-array fixture, doubles each signed integer, and serializes the result repeatedly. Go uses the standard JSON library. The dependency-free Rust implementation only supports this built-in fixture shape; it is not a general JSON parser. Both implementations must emit identical JSON before performance is interpreted.

`demo --workload database-fixture` parses a local CSV fixture and sums the `amount` field for active rows. It is a dependency-free mock, not a database benchmark: it excludes query planning, connections, transactions, indexes, concurrency, and database/network latency.

The default is 2,000,000 iterations and five measured repetitions per implementation; `--repetitions` accepts 3 through 100. Rust compilation is outside the measurement interval. RustRush first runs one untimed correctness probe for each implementation and compares the output. A mismatch prevents all timed trials. After matching probes, each implementation gets one warm-up run. Timed measurements are paired by repetition and alternate order (Go then Rust, Rust then Go) to reduce systematic order bias. Each process receives the same workload configuration and fixture.

The Rust source is compiled with `rustc --edition=2021 -C opt-level=3`. Go uses the binary built by `go run` or `go build`; these compiler settings are not equivalent, and the report records the Go runtime and Rust compiler version when available.

## Statistics

Raw nanosecond process durations and outputs are retained in JSON for each measured repetition, along with each implementation's correctness-probe output. Throughput is calculated as `iterations / median(process duration)`. p50, p95, and p99 use the nearest-rank method over trial durations; with five trials p95/p99 are effectively the maximum observation. These are whole-process latency percentiles (including startup, argument parsing, workload, and output capture), not request-level or per-operation latency distributions. Error counts report failed or correctness-invalid benchmark attempts; the harness does not issue multiple requests within one process.

No speedup is preloaded. If Rust is unavailable, the report includes baseline observations only and marks the verdict inconclusive. Missing, invalid, failed, or timed-out measurements, correctness mismatches, and incomplete repetitions cannot produce a performance verdict. The verdict requires at least three valid paired trials and rejects a comparison when either implementation's population coefficient of variation exceeds 10%. A measured throughput change of at least +10% yields `PORT`, at least -10% yields `DONT_PORT`, and a smaller change yields `OPTIMIZE_FIRST`. These thresholds are simple screening rules, not a statistical guarantee. Invalid/noisy measurements remain labeled as observations in the raw report, but the aggregate throughput delta is omitted and the verdict is `INCONCLUSIVE`.

## Interpretation limits

These workloads are useful for exercising the CLI, not for estimating application-wide benefits. The CPU workload is synthetic, and the JSON and CSV examples are constrained fixtures. Process launch time can dominate short runs. Results are sensitive to background load, power management, operating system, compiler version, and machine. Rust and Go compile flags and runtimes differ. Repeat with representative application inputs and workloads before making an engineering decision.

The implementation does not currently measure CPU utilization, memory, allocations, concurrency, request-level error rates, or system-wide noise. It does not perform confidence-interval analysis. `PORT`/`DONT_PORT` use a simple threshold on this single local test and are suggestions only.

## Reproduction

Run from the repository root:

```powershell
go run ./cmd/rustrush demo --iterations 2000000 --repetitions 5
```

Use `--workload json` or `--workload database-fixture` to run the fixture examples. The JSON report retains raw timings and environment information. Do not compare results across different environments as a controlled experiment.
