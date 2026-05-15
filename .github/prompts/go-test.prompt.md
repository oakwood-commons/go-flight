---
description: "Generate comprehensive Go tests for the selected code"
---

Generate tests for the selected code following these patterns:

1. Table-driven tests with descriptive names
2. Use testify (assert/require)
3. Cover: happy path, error cases, edge cases, nil inputs
4. For concurrent code: add race-condition tests
5. For benchmarks: add a benchmark with `b.ReportAllocs()`

Include `t.Parallel()` where safe. Use meaningful variable names in test cases.
