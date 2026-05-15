---
description: "go-flight: Run integration tests and verify cross-package behavior works correctly."
---

Run integration tests to verify changes work across package boundaries.

## Steps

1. Identify what changed -- use `git diff --name-only` to find modified packages.
2. Run the full test suite:
   ```
   go test ./...
   ```
3. Run integration tests specifically:
   ```
   go test -v ./tests/integration/...
   ```
4. Run with race detector to catch concurrency issues:
   ```
   go test -race ./...
   ```
5. For changes to flightgroup:
   - Verify cache package still works (it depends on flightgroup)
   - Run benchmarks to check for performance regressions: `go test -bench=. ./flightgroup/...`
6. For changes to cache:
   - Verify Store interface contract is maintained
   - Run benchmarks: `go test -bench=. ./cache/...`
7. Report results as a table: package, result (pass/fail), and any issues found.
8. If a test reveals a bug, fix the code and re-test.

## Key things to watch for

- Race conditions (always run with `-race`)
- Deadlocks in singleflight paths
- Panic propagation from leader to followers
- Cache store implementations not being concurrent-safe
- Channel operations that could block forever
