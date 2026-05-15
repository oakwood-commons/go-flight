---
description: "go-flight: Check if staged changes have corresponding tests, docs, and benchmarks."
agent: agent
argument-hint: "Optional: specific area to check"
---

Review staged changes and check if supporting artifacts exist:

1. Run `git diff --cached --stat` to identify staged changes
2. If nothing is staged, fall back to `git log origin/main..HEAD --stat` to check pushed commits on the branch
3. For each changed package or feature, verify:
   - Unit tests in `<package>/*_test.go`
   - Benchmark tests in `<package>/*_benchmark_test.go`
   - Fuzz tests in `<package>/*_fuzz_test.go`
   - Integration tests in `tests/integration/`
   - README documentation if public API changed
   - Concurrency safety documentation on exported types
4. Report present vs missing as a checklist
5. Do not create anything, just report the gaps
