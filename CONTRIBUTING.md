# Contributing

Thanks for helping improve RustRush. The project is currently a small Go CLI with a checked-in Rust benchmark candidate.

## Before changing code

- Read the README and benchmark methodology.
- Keep analysis local and non-executing.
- Never turn static source patterns into claims of measured bottlenecks.
- Do not add benchmark results as expected performance values in tests.
- Preserve the report schema or document a compatible schema change.

## Validate changes

With Go 1.22 or newer:

```powershell
gofmt -w (Get-ChildItem -Recurse -Filter *.go | ForEach-Object FullName)
go test ./...
go vet ./...
go build ./cmd/rustrush
```

If Rust code changes and `rustc` is available, also run the demo and check the correctness status in its report:

```powershell
go run ./cmd/rustrush demo --iterations 1000000 --repetitions 3
```

Do not compare timings from different machines as though they were equivalent. Do not commit generated `results/`, binaries, credentials, or the private local `RUSHRUSH_PLAN.md`.

## Changes and reports

Keep changes focused. Add tests for report validation, statistical calculations, path filtering, or correctness gates when changing those behaviors. Describe any unimplemented follow-up honestly rather than exposing placeholder commands.
