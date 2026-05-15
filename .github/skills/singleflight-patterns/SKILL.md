# Singleflight Patterns

## Description
Common patterns for using the go-flight singleflight library, including leader/follower execution, cache integration, and error handling.

## Patterns

### Basic Singleflight
```go
group := flightgroup.NewGroup[string, MyResult]()
order := group.Do("key", 0, func() (MyResult, error) {
    return fetchExpensiveData()
})
result := <-order.Ch()
if result.Err != nil {
    // handle error
}
// use result.Value
```

### With Cache Integration
```go
mgr := cache.NewManager[string, MyResult](
    cache.WithStore[string, MyResult]("memory", myStore),
)
val, err := mgr.Do(ctx, "key", func(ctx context.Context) (cache.FetchResult[MyResult], error) {
    return cache.FetchResult[MyResult]{
        Value:  fetchedData,
        TTL:    time.Minute,
        Policy: cache.CacheWithTTL,
    }, nil
}, nil)
```

### Error Handling
- Leader errors propagate to all followers
- Use `WithRetryFollowerOnError` for transient failures
- Context cancellation only affects the cancelled caller
