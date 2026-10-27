# CPU-heavy example

This example repeatedly mixes a 64-bit integer to produce a deterministic checksum. The Go baseline is implemented in RustRush's internal workload module; the Rust candidate is [candidate.rs](candidate.rs).

Run the end-to-end correctness and benchmark workflow from the repository root:

```powershell
go run ./cmd/rustrush demo --iterations 2000000 --repetitions 5
```

RustRush performs a warm-up, checks the baseline/candidate checksums, and records raw timing samples. If `rustc` is unavailable, the candidate is not run and the report is inconclusive. See the [methodology](../../docs/benchmark-methodology.md) for timing scope and limitations.
