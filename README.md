# go-flight

A generic singleflight library for Go with a leader/follower execution model and a batteries-included multi-tier cache manager. It deduplicates concurrent calls to the same key so only one goroutine does the work while the rest wait for the shared result.

**Why not `x/sync/singleflight`?**

| | `x/sync/singleflight` | `go-flight` |
|---|---|---|
| Type safety | `interface{}` | Fully generic — no type assertions |
| Return model | Blocks until done | Non-blocking — returns a channel you can `select` on |
| Caller awareness | None | Leader/follower with caller tagging |
| Slow callers | Wait forever | Followers bail out after a threshold and retry |
| Error handling | All callers get the same error | Followers can optionally retry independently |
| Caching | BYO | Built-in multi-tier cache manager |

## Install

```bash
go get github.com/oakwood-commons/go-flight
```

Requires Go 1.21+ (generics + `context.WithoutCancel`).

---

## Packages

### `flightgroup` — Low-Level Singleflight

The core deduplication primitive. When multiple goroutines call `Do` with the same key concurrently:

1. The **first caller** becomes the **leader** — it executes the function.
2. **Subsequent callers** become **followers** — they receive a channel that delivers the leader's result.
3. `Do` returns immediately with an `ExecutionOrder` that tells you your role and provides the result channel.

```go
g := flightgroup.NewGroup[string, int]()

// Three goroutines call Do("user:42", ...) concurrently.
// Only ONE executes the function. All three receive the same result.
order := g.Do("user:42", callerID, func() (int, error) {
    return db.CountFollowers(42) // expensive call
})

fmt.Println(order.Leader) // true for the leader, false for followers

// Non-blocking — use select to race against context, timers, etc.
select {
case res := <-order.Ch():
    fmt.Println(res.Value, res.Err)
case <-ctx.Done():
    fmt.Println("timed out")
}
```

**Key details:**

- `callerTag` is an opaque `uint64` you attach to a flight. Followers can read it via `order.LeaderTag` to know who is doing the work.
- `Forget(key)` detaches the key so the next `Do` starts a fresh flight. The in-progress flight still delivers to its existing followers.
- If the function panics, the panic is recovered and delivered as a `PanicErr` (with stack trace) to **all** waiters — no goroutine is left hanging.

---

### `cache` — Multi-Tier Cache Manager

A higher-level API built on `flightgroup`. It combines cache lookups, singleflight deduplication, and smart follower behavior into a single `Do` call.

**How `Do` works:**

```
caller → check stores (L1 → L2 → ...) → hit? return
                                        → miss? deduplicate via flightgroup
                                            → leader fetches, caches result, broadcasts
                                            → followers wait (with bailout options)
```

#### Basic usage

```go
mgr := cache.NewManager[string, *User](
    cache.WithStore[string, *User]("l1", myInMemoryStore),
    cache.WithStore[string, *User]("l2", myRedisStore),
    cache.WithSlowThreshold[string, *User](200 * time.Millisecond),
    cache.WithRetryFollowerOnError[string, *User](true),
)

user, err := mgr.Do(ctx, "user:42", fetchUser, &cache.Hooks{
    OnCacheHit:   func(source string) { metrics.Inc("cache_hit", source) },
    OnSuccess:    func() { metrics.Inc("fetch_success") },
    OnFetchError: func(err error) { log.Warn("fetch failed", "err", err) },
})
```

#### Cache policies

Your `FetchFunc` returns a `FetchResult` that tells the manager how to cache:

```go
func fetchUser(ctx context.Context) (cache.FetchResult[*User], error) {
    user, err := api.GetUser(ctx, 42)
    if err != nil {
        return cache.FetchResult[*User]{}, err
    }
    return cache.FetchResult[*User]{
        Value:  user,
        TTL:    10 * time.Minute,
        Policy: cache.CacheWithTTL,   // also: DoNotCache, CacheIndefinitely
    }, nil
}
```

| Policy | Behavior |
|---|---|
| `DoNotCache` | Return the value but don't write to any store |
| `CacheWithTTL` | Write to all stores with the specified TTL |
| `CacheIndefinitely` | Write to all stores with no expiry (`TTLIndefinite`) |

#### Implementing a Store

Plug in any backend by implementing the `Store` interface:

```go
type Store[K comparable, V any] interface {
    Get(ctx context.Context, key K) (V, bool)
    Set(ctx context.Context, key K, value V, ttl time.Duration)
}
```

`Set` receives `cache.TTLIndefinite` (`math.MaxInt64`) for indefinite storage. `Set` has no error return — implementations handle failures internally (log, metric, etc.).

Stores are checked in registration order; the first hit wins. On write, the value is fanned out to **all** stores.

---

## Manager Options

| Option | Default | Description |
|---|---|---|
| `WithStore(name, store)` | none | Append a cache tier. Checked in order; first hit wins. |
| `WithSlowThreshold(d)` | 0 (disabled) | After this duration, a follower abandons the leader and retries with its own flight. |
| `WithExpiryThreshold(d)` | 0 (cache all) | `CacheWithTTL` results with TTL ≤ this threshold are not cached. |
| `WithRetryFollowerOnError(b)` | `false` | When `true`, followers retry independently if the leader's fetch errors. When `false`, the leader's error is returned directly. |
| `WithRequestIDExtractor(fn)` | returns 0 | Extract a caller ID from context. Callers with the same non-zero ID co-own the flight — they skip follower retry/bailout logic and wait directly on the leader's channel. Useful when the same logical request crosses middleware layers. |

## Direct access

You can also bypass deduplication and work with the stores directly:

```go
// Read from stores (first hit wins)
result := mgr.Get(ctx, "user:42")
if result.OK {
    fmt.Println(result.Value, "from", result.Source)
}

// Write to all stores
mgr.Set(ctx, "user:42", user, 10*time.Minute)
```

## License

[MIT](LICENSE)
