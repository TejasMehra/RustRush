# Database-like fixture example (mock)

This is a deterministic local CSV fixture, **not a real database**. Each iteration parses the fixture and sums `amount` for rows whose `active` field is `true`. It exercises a small filter-and-aggregate operation without requiring SQLite, a service, or network access.

Run from the repository root:

```powershell
go run ./cmd/rustrush demo --workload database-fixture --iterations 1000 --repetitions 5
```

The Go and Rust implementations consume the same [records.csv](records.csv) and must emit the same aggregate before performance is interpreted. The fixture does not model connection setup, query planning, indexes, transactions, locks, network latency, or concurrent clients; do not treat its timings as database evidence.
