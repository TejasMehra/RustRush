# Benchmark methodology

## Built-in workloads

`demo --workload cpu` compares a deterministic 64-bit integer-mixing loop implemented in Go and Rust. Each process receives the same `--iterations` value and prints its final checksum. The checksum is checked before interpreting performance.

`demo --workload json` reads the shared integer-array fixture, doubles each signed integer, and serializes the result repeatedly. Go uses the standard JSON library. The dependency-free Rust implementation only supports this built-in fixture shape; it is not a general JSON parser. Both implementations must emit identical JSON before performance is interpreted.

`demo --workload database-fixture` parses a local CSV fixture and sums the `amount` field for active rows. It is a dependency-free mock, not a database benchmark: it excludes query planning, connections, transactions, indexes, concurrency, and database/network latency.

The default is 2,000,000 iterations and five measured repetitions per implementation; `--repetitions` accepts 3 through 100. Rust compilation is outside the measurement interval. The first run for each implementation is a warm-up and correctness probe; its timer result is discarded, and outputs are compared. A mismatch prevents all timed trials. After matching outputs, paired measurements alternate order (Go then Rust, Rust then Go) to reduce systematic order bias. Each process receives the same workload configuration and fixture.

The Rust source is compiled with `rustc --edition=2021 -C opt-level=3`. Go uses the binary built by `go run` or `go build`; these compiler settings are not equivalent, and the report records the Go runtime and Rust compiler version when available.

## Candidate baselines

`candidate --baseline-language go` (the default) accepts a Go module whose top-level package is `main`. `candidate --baseline-language python` accepts one `.py` file and requires Python 3 on `PATH`. Python source is copied to a temporary directory, syntax-checked, and invoked with `-s -S -B`; it must use the standard library and implement the same `--iterations`/optional `--input` contract as the Rust candidate. RustRush passes both programs the same workload arguments and copied input, checks their nonempty stdout before timed repetitions, and uses the same paired order and aggregation. Python container execution is explicitly unsupported.

Python process samples include interpreter startup, script execution, and output capture. Reports identify the baseline language and Python runtime version. Go and Python baseline builds/runs have different runtime costs, so interpret each comparison only for its selected workload and timing mode.

## Statistics

Raw nanosecond process durations and outputs are retained in JSON for each measured repetition, along with each implementation's correctness-probe output. The correctness warm-up's duration is not retained as a benchmark sample. Throughput is calculated as `iterations / median(process duration)`. p50, p95, and p99 use the nearest-rank method over trial durations; with five trials p95/p99 are effectively the maximum observation. These are process-duration percentiles (in the default `process_end_to_end` mode including startup, argument parsing, workload, and output capture), not request-level or per-operation latency distributions. Error counts report failed or correctness-invalid benchmark attempts; the harness does not issue multiple requests within one process.

No speedup is preloaded. If Rust is unavailable, the report includes baseline observations only and marks the verdict inconclusive. Missing, invalid, failed, or timed-out measurements, correctness mismatches, and incomplete repetitions cannot produce a performance verdict. The verdict requires at least three valid paired trials and rejects a comparison when either implementation's population coefficient of variation exceeds 10%. A measured throughput change of at least +10% yields `PORT`, at least -10% yields `DONT_PORT`, and a smaller change yields `OPTIMIZE_FIRST`. These thresholds are simple screening rules, not a statistical guarantee. Invalid/noisy measurements remain labeled as observations in the raw report, but the aggregate throughput delta is omitted and the verdict is `INCONCLUSIVE`.

Container samples also carry the timer helper's empirical minimum monotonic-clock tick. RustRush reports `INCONCLUSIVE` if any timed process duration spans fewer than 1,000 observed ticks. This screening budget was selected so nominal timestamp quantization is below 0.1% of the interval (one tick per 1,000 ticks); it is not an accuracy guarantee, does not account for every source of noise, and is not applied to local-runner samples whose clock resolution is not measured by the workload process. Timer accuracy is not calibrated.

## Interpretation limits

These workloads are useful for exercising the CLI, not for estimating application-wide benefits. The CPU workload is synthetic, and the JSON and CSV examples are constrained fixtures. Process launch time can dominate short runs. Results are sensitive to background load, power management, operating system, compiler version, and machine. Rust and Go compile flags and runtimes differ. Repeat with representative application inputs and workloads before making an engineering decision.

The implementation does not currently measure CPU utilization, memory, allocations, concurrency, request-level error rates, or system-wide noise. It does not perform confidence-interval analysis. `PORT`/`DONT_PORT` use a simple threshold on this single local test and are suggestions only.

## Reproduction

Run from the repository root:

```powershell
go run ./cmd/rustrush demo --iterations 2000000 --repetitions 5
```

Use `--workload json` or `--workload database-fixture` to run the fixture examples. The JSON report retains raw timings and environment information. Do not compare results across different environments as a controlled experiment.

## Timing modes

- `process_end_to_end` (local runner): each sample is host-measured wall-clock time of one process launch.
- `container_process_isolated` (`--isolation container`): builds and image preparation finish before measurements. Each implementation's warm-up also serves as its correctness probe; that run's duration is discarded. Each warm-up/correctness run and each measured repetition uses a new hardened `docker run --rm` container under the same resource limits and read-only binary/input mounts. A small Go helper times the workload child with `time.Since`, which uses Go's monotonic clock when available. It transports stdout as base64 in a structured JSON response, so workload output cannot spoof a timing marker; malformed, duplicate-field, missing, negative, zero, or out-of-range timing responses are rejected. Child non-zero exit status is propagated and invalidates the run. The sample includes workload process startup and stdout capture but excludes timer-helper startup, container lifecycle, image preparation, compilation, and Docker CLI dispatch. `container_invocation_wall_ms_median` records full invocation duration across correctness/warm-up and trials. `container_run_overhead_ms_estimate_median` is a derived residual after subtracting child duration across successful invocations; it combines timer, lifecycle, and dispatch costs, and clocks are not cross-calibrated, so it is only an estimate. Failed invocation wall times are reported separately and never used as samples. `environment.timing_resolution_ns` records an empirical minimum monotonic-clock tick, not timer accuracy.

The current timing modes are recorded in `environment.timing_mode` (`process_end_to_end` or `container_process_isolated`; absent in older reports means `process_end_to_end`). Legacy `steady_state_in_container` reports remain readable but are not emitted by current versions. `compare` and ROI linkage never mix modes. A failed `docker run`, missing or malformed response, clock/timer failure, invalid process exit, or failed environment cleanup invalidates the comparison and yields `INCONCLUSIVE`. Fresh containers prevent process-created files and caches from persisting into later repetitions. Host page caches, kernel state, and external machine load are not reset. Baseline and candidate use different images and system libraries, containers share the host kernel, no confidence intervals are computed, and CPU/memory are not measured.
