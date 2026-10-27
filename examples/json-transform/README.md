# JSON transformation example

This example reads the shared [fixture](fixture.json), doubles each signed integer in its `values` array, and serializes the same JSON shape. The Go baseline uses Go's JSON decoder and encoder. The Rust candidate is dependency-free and intentionally supports only this narrow integer-array fixture shape; it is not a general JSON parser.

Run the workflow from the repository root:

```powershell
go run ./cmd/rustrush demo --workload json --iterations 10000 --repetitions 5
```

Both programs transform the fixture the requested number of times, emit the final JSON result, and must match exactly before the report can assign a performance verdict. A missing Rust compiler produces an inconclusive report with baseline timings only.
