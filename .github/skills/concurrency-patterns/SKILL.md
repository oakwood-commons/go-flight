# Concurrency Patterns

## Description
Safe concurrency patterns used in go-flight, including mutex usage, channel patterns, and goroutine lifecycle management.

## Key Patterns

### Mutex Protection
```go
type Group[V any] struct {
    mu    sync.Mutex
    calls map[string]*call[V]
}
```

### Channel-Based Results
```go
// Non-blocking result delivery via channels
ch := make(chan Result[V], 1)
```

### Context Awareness
- Always pass `context.Context` through
- Check `ctx.Done()` in long-running operations
- Use `context.WithoutCancel` for shared work that shouldn't be cancelled by individual callers

### Goroutine Lifecycle
- Every goroutine must have a clear exit path
- Use `sync.WaitGroup` for coordinated shutdown
- Test for goroutine leaks with `-race` and leak detectors
