package integration

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oakwood-commons/go-flight/cache"
	"github.com/oakwood-commons/go-flight/flightgroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memoryStore is a simple in-memory Store implementation for testing.
type memoryStore[K comparable, V any] struct {
	mu    sync.RWMutex
	items map[K]V
}

func newMemoryStore[K comparable, V any]() *memoryStore[K, V] {
	return &memoryStore[K, V]{items: make(map[K]V)}
}

func (s *memoryStore[K, V]) Get(_ context.Context, key K) (V, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.items[key]
	return v, ok
}

func (s *memoryStore[K, V]) Set(_ context.Context, key K, value V, _ time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[key] = value
}

func (s *memoryStore[K, V]) Delete(_ context.Context, key K) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, key)
}

// TestIntegration_FlightGroupDeduplication verifies that concurrent calls
// to the same key are deduplicated: only one execution occurs.
func TestIntegration_FlightGroupDeduplication(t *testing.T) {
	g := flightgroup.NewGroup[string, string]()

	var execCount atomic.Int32
	const concurrency = 50
	const key = "test-key"

	var wg sync.WaitGroup
	results := make([]flightgroup.ExecutionOrder[string], concurrency)

	// Synchronize all goroutines to start at the same time
	ready := make(chan struct{})

	for i := range concurrency {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-ready
			results[idx] = g.Do(key, uint64(idx), func() (string, error) {
				execCount.Add(1)
				time.Sleep(10 * time.Millisecond)
				return "result", nil
			})
		}(i)
	}

	close(ready)
	wg.Wait()

	// Verify only one execution occurred
	assert.Equal(t, int32(1), execCount.Load(), "expected exactly one execution")

	// Verify exactly one leader
	leaderCount := 0
	for _, r := range results {
		if r.Leader {
			leaderCount++
		}
	}
	assert.Equal(t, 1, leaderCount, "expected exactly one leader")

	// All callers should receive the same result
	for i, r := range results {
		res := <-r.Ch()
		assert.NoError(t, res.Err, "caller %d got error", i)
		assert.Equal(t, "result", res.Value, "caller %d got wrong value", i)
	}
}

// TestIntegration_FlightGroupPanicRecovery verifies that a panic in the
// flight function is recovered and delivered as an error to all waiters.
func TestIntegration_FlightGroupPanicRecovery(t *testing.T) {
	g := flightgroup.NewGroup[string, int]()

	const concurrency = 10
	var wg sync.WaitGroup
	results := make([]flightgroup.ExecutionOrder[int], concurrency)

	// block keeps the leader's fn running until we release it.
	// started signals that the leader has entered its fn (flight is registered).
	block := make(chan struct{})
	started := make(chan struct{})

	// First goroutine becomes leader; it signals once inside fn, then waits.
	wg.Add(1)
	go func() {
		defer wg.Done()
		results[0] = g.Do("panic-key", 0, func() (int, error) {
			close(started) // signal that the flight is registered
			<-block
			panic("test panic")
		})
	}()

	// Wait for the leader to register the flight.
	<-started

	// Remaining goroutines join as followers.
	for i := 1; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = g.Do("panic-key", uint64(idx), func() (int, error) {
				panic("should not execute — follower")
			})
		}(i)
	}

	// Give followers time to join the flight, then trigger the panic.
	time.Sleep(10 * time.Millisecond)
	close(block)
	wg.Wait()

	// All callers should receive an error (not panic)
	for i, r := range results {
		res := <-r.Ch()
		assert.Error(t, res.Err, "caller %d should get error from panic", i)
		assert.Contains(t, res.Err.Error(), "test panic", "error should contain panic message")
	}
}

// TestIntegration_CacheManagerWithFlightGroup verifies end-to-end behavior:
// cache miss triggers fetch with deduplication, result is cached, subsequent
// calls hit the cache.
func TestIntegration_CacheManagerWithFlightGroup(t *testing.T) {
	store := newMemoryStore[string, string]()
	mgr := cache.NewManager[string, string](
		cache.WithStore[string, string]("memory", store),
	)

	var fetchCount atomic.Int32
	ctx := context.Background()

	fetchFn := func(ctx context.Context) (cache.FetchResult[string], error) {
		fetchCount.Add(1)
		time.Sleep(20 * time.Millisecond) // simulate work
		return cache.FetchResult[string]{
			Value:  "fetched-value",
			TTL:    time.Minute,
			Policy: cache.CacheWithTTL,
		}, nil
	}

	// First call: should fetch
	val, err := mgr.Do(ctx, "key1", fetchFn, nil)
	require.NoError(t, err)
	assert.Equal(t, "fetched-value", val)
	assert.Equal(t, int32(1), fetchCount.Load())

	// Second call: should hit cache (no additional fetch)
	val, err = mgr.Do(ctx, "key1", fetchFn, nil)
	require.NoError(t, err)
	assert.Equal(t, "fetched-value", val)
	assert.Equal(t, int32(1), fetchCount.Load(), "second call should hit cache, not trigger a new fetch")
}

// TestIntegration_CacheManagerConcurrentFetch verifies that concurrent
// fetches for the same key are deduplicated through the cache manager.
func TestIntegration_CacheManagerConcurrentFetch(t *testing.T) {
	store := newMemoryStore[string, int]()
	mgr := cache.NewManager[string, int](
		cache.WithStore[string, int]("memory", store),
	)

	var fetchCount atomic.Int32
	ctx := context.Background()

	fetchFn := func(_ context.Context) (cache.FetchResult[int], error) { //nolint:unparam // error intentionally nil in test
		fetchCount.Add(1)
		time.Sleep(50 * time.Millisecond) // simulate slow fetch
		return cache.FetchResult[int]{
			Value:  42,
			TTL:    time.Minute,
			Policy: cache.CacheWithTTL,
		}, nil
	}

	const concurrency = 20
	var wg sync.WaitGroup
	errs := make([]error, concurrency)
	vals := make([]int, concurrency)

	ready := make(chan struct{})

	for i := range concurrency {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-ready
			vals[idx], errs[idx] = mgr.Do(ctx, "concurrent-key", fetchFn, nil)
		}(i)
	}

	close(ready)
	wg.Wait()

	for i := range concurrency {
		assert.NoError(t, errs[i], "goroutine %d got error", i)
		assert.Equal(t, 42, vals[i], "goroutine %d got wrong value", i)
	}

	// With singleflight, concurrent fetches for the same key should be deduplicated
	// to a single fetch (or at most a small number due to timing).
	assert.LessOrEqual(t, fetchCount.Load(), int32(2),
		"singleflight should deduplicate concurrent fetches to at most 2")
}

// TestIntegration_CacheManagerMultipleKeys verifies that different keys
// execute independently and don't interfere with each other.
func TestIntegration_CacheManagerMultipleKeys(t *testing.T) {
	store := newMemoryStore[string, string]()
	mgr := cache.NewManager[string, string](
		cache.WithStore[string, string]("memory", store),
	)

	ctx := context.Background()

	for i := range 5 {
		key := fmt.Sprintf("key-%d", i)
		expected := fmt.Sprintf("value-%d", i)

		fetchFn := func(ctx context.Context) (cache.FetchResult[string], error) {
			return cache.FetchResult[string]{
				Value:  expected,
				TTL:    time.Minute,
				Policy: cache.CacheWithTTL,
			}, nil
		}

		val, err := mgr.Do(ctx, key, fetchFn, nil)
		require.NoError(t, err)
		assert.Equal(t, expected, val)
	}

	// Verify all values are cached
	for i := range 5 {
		key := fmt.Sprintf("key-%d", i)
		result := mgr.Get(ctx, key)
		assert.True(t, result.OK, "key %s should be cached", key)
		assert.Equal(t, fmt.Sprintf("value-%d", i), result.Value)
	}
}

// TestIntegration_CacheManagerFetchError verifies that fetch errors are
// properly propagated to callers.
func TestIntegration_CacheManagerFetchError(t *testing.T) {
	store := newMemoryStore[string, string]()
	mgr := cache.NewManager[string, string](
		cache.WithStore[string, string]("memory", store),
	)

	ctx := context.Background()
	expectedErr := fmt.Errorf("fetch failed: %w", context.DeadlineExceeded)

	fetchFn := func(ctx context.Context) (cache.FetchResult[string], error) {
		return cache.FetchResult[string]{}, expectedErr
	}

	_, err := mgr.Do(ctx, "error-key", fetchFn, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)

	// Error result should not be cached
	result := mgr.Get(ctx, "error-key")
	assert.False(t, result.OK, "error results should not be cached")
}
