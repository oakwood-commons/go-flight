---
applyTo: "**/*.go"
---

# Go Conventions

## Error Handling
- Always wrap errors with context: `fmt.Errorf("operation failed: %w", err)`
- Use sentinel errors for expected conditions: `var ErrNotFound = errors.New("not found")`
- Never panic in library code — return errors instead

## Naming
- Use short, descriptive variable names (Go convention)
- Acronyms should be all caps: `HTTP`, `ID`, `URL`
- Interfaces with single methods: name after the method + "er" suffix

## Concurrency
- Document goroutine safety in godoc for all exported types
- Use `sync.Mutex` for shared state, prefer `sync.RWMutex` for read-heavy workloads
- Always use `context.Context` for cancellation in long-running operations

## Testing
- Use `testify/assert` and `testify/require`
- Table-driven tests with descriptive `name` fields
- Use `t.Parallel()` where safe
- Benchmark hot paths with `b.ReportAllocs()`
