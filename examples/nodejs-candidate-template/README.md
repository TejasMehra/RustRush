# Node.js baseline vs Rust candidate

A dependency-free, deterministic CPU workload (a modular checksum recurrence)
implemented identically in `baseline.js` and `candidate.rs`. It exercises the
RustRush workflow; it does not guarantee that Rust is faster.

Requirements: Node.js 18 or newer and `rustc` on `PATH`. No npm install and no network access.

```sh
node --check baseline.js
go run ./cmd/rustrush candidate --baseline-language nodejs \
  --baseline examples/nodejs-candidate-template/baseline.js \
  --candidate examples/nodejs-candidate-template/candidate.rs \
  --input examples/nodejs-candidate-template/input.txt \
  --iterations 10000 --repetitions 5 --allow-execute
```

Both programs must print the same single line for the same `--iterations` and
`--input` before any timed run starts. Run mode is process isolation only
(`--isolation container` is rejected for Node.js baselines). Review the code
before using `--allow-execute`; process mode is not a sandbox.
