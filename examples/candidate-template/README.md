# Candidate template

A minimal, working example of the `rustrush candidate` contract.

- `baseline/` is a Go module whose `main` package is the operation under test.
- `candidate.rs` is a self-contained, dependency-free Rust port.
- `input.txt` is the shared input, passed to both programs as `--input`.

Both programs must accept `--iterations N` (and `--input FILE`) and print one
deterministic line to stdout. RustRush requires identical stdout before timing.

```
go run ./cmd/rustrush candidate \
  --baseline examples/candidate-template/baseline \
  --candidate examples/candidate-template/candidate.rs \
  --input examples/candidate-template/input.txt \
  --allow-execute
```

Without `rustc` the report records the Go baseline and marks the Rust candidate
unavailable (INCONCLUSIVE). Copy this directory to start a candidate for your own
operation.
