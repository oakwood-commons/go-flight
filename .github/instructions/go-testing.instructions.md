---
applyTo: "**/*_test.go,tests/**"
---

# Go Testing Instructions

## Test Structure
- Use Arrange/Act/Assert pattern
- Table-driven tests for multiple cases
- Descriptive test names: `TestFunction_Scenario_Expected`

## Assertions
- `require.NoError` for errors that should abort the test
- `assert.Equal` for value comparisons that allow continued execution
- `assert.Eventually` for async operations

## Concurrency Tests
- Use `sync.WaitGroup` to coordinate goroutines
- Test with `-race` flag
- Use `goleak` or manual checks for goroutine leaks

## Benchmarks
- Always call `b.ReportAllocs()`
- Use `b.ResetTimer()` after setup
- Benchmark with realistic data sizes
