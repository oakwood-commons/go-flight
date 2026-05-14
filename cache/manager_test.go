package cache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// mapStore is an in-memory Store for testing.
type mapStore[K comparable, V any] struct {
	data map[K]V
	sets []setCall[K, V] // records all Set calls
}

type setCall[K comparable, V any] struct {
	key   K
	value V
	ttl   time.Duration
}

func newMapStore[K comparable, V any]() *mapStore[K, V] {
	return &mapStore[K, V]{data: make(map[K]V)}
}

func (s *mapStore[K, V]) Get(_ context.Context, key K) (V, bool) {
	v, ok := s.data[key]
	return v, ok
}

func (s *mapStore[K, V]) Set(_ context.Context, key K, value V, ttl time.Duration) {
	s.data[key] = value
	s.sets = append(s.sets, setCall[K, V]{key: key, value: value, ttl: ttl})
}

// --- NewManager / options ---

func TestNewManager(t *testing.T) {
	t.Parallel()

	t.Run("no_options_does_not_panic", func(t *testing.T) {
		t.Parallel()
		assert.NotPanics(t, func() {
			m := NewManager[string, string]()
			assert.NotNil(t, m)
			assert.NotNil(t, m.sf)
		})
	})

	t.Run("default_reqID_returns_zero", func(t *testing.T) {
		t.Parallel()
		m := NewManager[string, string]()
		assert.Equal(t, uint64(0), m.reqID(context.Background()))
	})

	t.Run("WithStore_appends_in_order", func(t *testing.T) {
		t.Parallel()
		s1 := newMapStore[string, string]()
		s2 := newMapStore[string, string]()
		m := NewManager[string, string](
			WithStore("first", s1),
			WithStore("second", s2),
		)
		assert.Equal(t, 2, len(m.stores))
		assert.Equal(t, "first", m.stores[0].name)
		assert.Equal(t, "second", m.stores[1].name)
	})

	t.Run("WithSlowThreshold_sets_field", func(t *testing.T) {
		t.Parallel()
		m := NewManager[string, string](WithSlowThreshold[string, string](100 * time.Millisecond))
		assert.Equal(t, 100*time.Millisecond, m.slowThreshold)
	})

	t.Run("WithExpiryThreshold_sets_field", func(t *testing.T) {
		t.Parallel()
		m := NewManager[string, string](WithExpiryThreshold[string, string](5 * time.Minute))
		assert.Equal(t, 5*time.Minute, m.expiryThreshold)
	})

	t.Run("WithRequestIDExtractor_sets_field", func(t *testing.T) {
		t.Parallel()
		extractor := func(context.Context) uint64 { return 42 }
		m := NewManager[string, string](WithRequestIDExtractor[string, string](extractor))
		assert.Equal(t, uint64(42), m.reqID(context.Background()))
	})
}

// --- Get ---

func TestGet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("hit_on_first_store", func(t *testing.T) {
		t.Parallel()
		s := newMapStore[string, string]()
		s.data["key"] = "value"
		m := NewManager[string, string](WithStore("l1", s))

		res := m.Get(ctx, "key")
		assert.True(t, res.OK)
		assert.Equal(t, "value", res.Value)
		assert.Equal(t, "l1", res.Source)
	})

	t.Run("miss_on_first_hit_on_second", func(t *testing.T) {
		t.Parallel()
		s1 := newMapStore[string, string]()
		s2 := newMapStore[string, string]()
		s2.data["key"] = "from-l2"
		m := NewManager[string, string](
			WithStore("l1", s1),
			WithStore("l2", s2),
		)

		res := m.Get(ctx, "key")
		assert.True(t, res.OK)
		assert.Equal(t, "from-l2", res.Value)
		assert.Equal(t, "l2", res.Source)
	})

	t.Run("miss_on_all_stores", func(t *testing.T) {
		t.Parallel()
		s1 := newMapStore[string, string]()
		s2 := newMapStore[string, string]()
		m := NewManager[string, string](
			WithStore("l1", s1),
			WithStore("l2", s2),
		)

		res := m.Get(ctx, "missing")
		assert.False(t, res.OK)
		assert.Equal(t, "", res.Value)
		assert.Equal(t, "", res.Source)
	})

	t.Run("no_stores_returns_zero", func(t *testing.T) {
		t.Parallel()
		m := NewManager[string, string]()
		assert.NotPanics(t, func() {
			res := m.Get(ctx, "key")
			assert.False(t, res.OK)
		})
	})
}

// --- Set ---

func TestSet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("writes_to_all_stores", func(t *testing.T) {
		t.Parallel()
		s1 := newMapStore[string, string]()
		s2 := newMapStore[string, string]()
		m := NewManager[string, string](
			WithStore("l1", s1),
			WithStore("l2", s2),
		)

		m.Set(ctx, "key", "val", time.Minute)
		assert.Equal(t, "val", s1.data["key"])
		assert.Equal(t, "val", s2.data["key"])
	})

	t.Run("records_correct_ttl", func(t *testing.T) {
		t.Parallel()
		s := newMapStore[string, string]()
		m := NewManager[string, string](WithStore("l1", s))

		m.Set(ctx, "key", "val", 30*time.Second)
		assert.Equal(t, 30*time.Second, s.sets[0].ttl)
	})

	t.Run("no_stores_does_not_panic", func(t *testing.T) {
		t.Parallel()
		m := NewManager[string, string]()
		assert.NotPanics(t, func() {
			m.Set(ctx, "key", "val", time.Minute)
		})
	})
}

// --- shouldCache ---

func TestShouldCache(t *testing.T) {
	t.Parallel()

	t.Run("DoNotCache_returns_false", func(t *testing.T) {
		t.Parallel()
		m := NewManager[string, string]()
		assert.False(t, m.shouldCache(FetchResult[string]{Policy: DoNotCache}))
	})

	t.Run("CacheIndefinitely_returns_true", func(t *testing.T) {
		t.Parallel()
		m := NewManager[string, string]()
		assert.True(t, m.shouldCache(FetchResult[string]{Policy: CacheIndefinitely}))
	})

	t.Run("CacheWithTTL_above_threshold_returns_true", func(t *testing.T) {
		t.Parallel()
		m := NewManager[string, string](WithExpiryThreshold[string, string](10 * time.Second))
		assert.True(t, m.shouldCache(FetchResult[string]{Policy: CacheWithTTL, TTL: 20 * time.Second}))
	})

	t.Run("CacheWithTTL_at_threshold_returns_false", func(t *testing.T) {
		t.Parallel()
		m := NewManager[string, string](WithExpiryThreshold[string, string](10 * time.Second))
		assert.False(t, m.shouldCache(FetchResult[string]{Policy: CacheWithTTL, TTL: 10 * time.Second}))
	})

	t.Run("CacheWithTTL_below_threshold_returns_false", func(t *testing.T) {
		t.Parallel()
		m := NewManager[string, string](WithExpiryThreshold[string, string](10 * time.Second))
		assert.False(t, m.shouldCache(FetchResult[string]{Policy: CacheWithTTL, TTL: 5 * time.Second}))
	})

	t.Run("CacheWithTTL_threshold_zero_always_true", func(t *testing.T) {
		t.Parallel()
		m := NewManager[string, string]() // expiryThreshold defaults to 0
		assert.True(t, m.shouldCache(FetchResult[string]{Policy: CacheWithTTL, TTL: time.Nanosecond}))
	})
}

// --- storeTTL ---

func TestStoreTTL(t *testing.T) {
	t.Parallel()

	t.Run("CacheIndefinitely_returns_TTLIndefinite", func(t *testing.T) {
		t.Parallel()
		m := NewManager[string, string]()
		assert.Equal(t, TTLIndefinite, m.storeTTL(FetchResult[string]{Policy: CacheIndefinitely, TTL: time.Hour}))
	})

	t.Run("CacheWithTTL_returns_result_TTL", func(t *testing.T) {
		t.Parallel()
		m := NewManager[string, string]()
		assert.Equal(t, 5*time.Minute, m.storeTTL(FetchResult[string]{Policy: CacheWithTTL, TTL: 5 * time.Minute}))
	})

	t.Run("DoNotCache_returns_result_TTL", func(t *testing.T) {
		t.Parallel()
		m := NewManager[string, string]()
		assert.Equal(t, 5*time.Minute, m.storeTTL(FetchResult[string]{Policy: DoNotCache, TTL: 5 * time.Minute}))
	})
}

// --- Do ---

func TestDo(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// --- cache hit ---

	t.Run("cache_hit_returns_value_without_fetching", func(t *testing.T) {
		t.Parallel()
		s := newMapStore[string, string]()
		s.data["key"] = "cached"
		m := NewManager[string, string](WithStore("l1", s))

		var fetchCalled atomic.Bool
		var hitSource string
		hooks := &Hooks{
			OnCacheHit: func(source string) { hitSource = source },
		}

		val, err := m.Do(ctx, "key", func(_ context.Context) (FetchResult[string], error) {
			fetchCalled.Store(true)
			return FetchResult[string]{Value: "fresh", Policy: CacheWithTTL, TTL: time.Minute}, nil
		}, hooks)

		assert.NoError(t, err)
		assert.Equal(t, "cached", val)
		assert.False(t, fetchCalled.Load(), "fetchFn should not be called on cache hit")
		assert.Equal(t, "l1", hitSource)
	})

	// --- fetch and cache ---

	t.Run("cache_miss_calls_fetchFn_and_caches_result", func(t *testing.T) {
		t.Parallel()
		s1 := newMapStore[string, string]()
		s2 := newMapStore[string, string]()
		m := NewManager[string, string](
			WithStore("l1", s1),
			WithStore("l2", s2),
		)

		val, err := m.Do(ctx, "key", func(_ context.Context) (FetchResult[string], error) {
			return FetchResult[string]{Value: "fetched", Policy: CacheWithTTL, TTL: 5 * time.Minute}, nil
		}, nil)

		assert.NoError(t, err)
		assert.Equal(t, "fetched", val)
		assert.Equal(t, "fetched", s1.data["key"])
		assert.Equal(t, "fetched", s2.data["key"])
		assert.Equal(t, 5*time.Minute, s1.sets[0].ttl)
		assert.Equal(t, 5*time.Minute, s2.sets[0].ttl)
	})

	t.Run("DoNotCache_result_is_not_stored", func(t *testing.T) {
		t.Parallel()
		s := newMapStore[string, string]()
		m := NewManager[string, string](WithStore("l1", s))

		val, err := m.Do(ctx, "key", func(_ context.Context) (FetchResult[string], error) {
			return FetchResult[string]{Value: "ephemeral", Policy: DoNotCache}, nil
		}, nil)

		assert.NoError(t, err)
		assert.Equal(t, "ephemeral", val)
		_, exists := s.data["key"]
		assert.False(t, exists, "DoNotCache result should not be stored")
	})

	t.Run("CacheIndefinitely_stores_with_TTLIndefinite", func(t *testing.T) {
		t.Parallel()
		s := newMapStore[string, string]()
		m := NewManager[string, string](WithStore("l1", s))

		val, err := m.Do(ctx, "key", func(_ context.Context) (FetchResult[string], error) {
			return FetchResult[string]{Value: "forever", Policy: CacheIndefinitely}, nil
		}, nil)

		assert.NoError(t, err)
		assert.Equal(t, "forever", val)
		assert.Equal(t, "forever", s.data["key"])
		assert.Equal(t, TTLIndefinite, s.sets[0].ttl)
	})

	t.Run("fetch_error_forgets_key_and_fires_hook", func(t *testing.T) {
		t.Parallel()
		s := newMapStore[string, string]()
		m := NewManager[string, string](WithStore("l1", s))

		var hookErr error
		hooks := &Hooks{
			OnFetchError: func(err error) { hookErr = err },
		}
		fetchErr := errors.New("network failure")

		_, err := m.Do(ctx, "key", func(_ context.Context) (FetchResult[string], error) {
			return FetchResult[string]{}, fetchErr
		}, hooks)

		assert.ErrorIs(t, err, fetchErr)
		assert.ErrorIs(t, hookErr, fetchErr)
		_, exists := s.data["key"]
		assert.False(t, exists, "error result should not be cached")

		// Subsequent call should start a fresh flight (key was forgotten)
		val, err := m.Do(ctx, "key", func(_ context.Context) (FetchResult[string], error) {
			return FetchResult[string]{Value: "retry-ok", Policy: CacheWithTTL, TTL: time.Minute}, nil
		}, nil)
		assert.NoError(t, err)
		assert.Equal(t, "retry-ok", val)
	})
}

// --- Do: deduplication ---

func TestDo_Deduplication(t *testing.T) {
	t.Parallel()

	t.Run("concurrent_calls_deduplicated_fetchFn_called_once", func(t *testing.T) {
		t.Parallel()
		m := NewManager[string, string]()

		var fetchCount atomic.Int32
		fetch := func(_ context.Context) (FetchResult[string], error) {
			fetchCount.Add(1)
			time.Sleep(50 * time.Millisecond)
			return FetchResult[string]{Value: "deduped", Policy: CacheWithTTL, TTL: time.Minute}, nil
		}

		n := 20
		ready := make(chan struct{})
		var arrived atomic.Int32
		var wg sync.WaitGroup
		results := make([]string, n)
		errs := make([]error, n)

		for i := range n {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				if arrived.Add(1) == int32(n) {
					close(ready)
				}
				<-ready
				results[idx], errs[idx] = m.Do(context.Background(), "key", fetch, nil)
			}(i)
		}
		wg.Wait()

		for i := range n {
			assert.NoError(t, errs[i], "goroutine %d", i)
			assert.Equal(t, "deduped", results[i], "goroutine %d", i)
		}
		assert.Equal(t, int32(1), fetchCount.Load(), "fetchFn should be called exactly once")
	})
}

// --- Do: same request ID (co-ownership) ---

func TestDo_SameRequestID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		leaderValue string
		leaderErr   error
	}{
		{
			name:        "success_shared_with_follower",
			leaderValue: "shared",
			leaderErr:   nil,
		},
		{
			name:        "error_shared_with_follower",
			leaderValue: "",
			leaderErr:   errors.New("leader-failed"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := NewManager[string, string](
				WithRequestIDExtractor[string, string](func(_ context.Context) uint64 { return 42 }),
				WithSlowThreshold[string, string](10*time.Millisecond),
				WithRetryFollowerOnError[string, string](true), // would trigger retry for a normal follower
			)

			var fetchCount atomic.Int32
			fetchStarted := make(chan struct{})
			leaderResult := tc // capture

			var wg sync.WaitGroup
			var leaderVal, followerVal string
			var leaderErr, followerErr error

			// Leader
			wg.Add(1)
			go func() {
				defer wg.Done()
				leaderVal, leaderErr = m.Do(context.Background(), "key", func(_ context.Context) (FetchResult[string], error) {
					fetchCount.Add(1)
					close(fetchStarted)
					time.Sleep(200 * time.Millisecond) // well over slowThreshold
					if leaderResult.leaderErr != nil {
						return FetchResult[string]{}, leaderResult.leaderErr
					}
					return FetchResult[string]{Value: leaderResult.leaderValue, Policy: CacheWithTTL, TTL: time.Minute}, nil
				}, nil)
			}()

			// Same-request follower
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-fetchStarted
				followerVal, followerErr = m.Do(context.Background(), "key", func(_ context.Context) (FetchResult[string], error) {
					fetchCount.Add(1)
					return FetchResult[string]{Value: "should-not-run"}, nil
				}, nil)
			}()

			wg.Wait()

			assert.Equal(t, int32(1), fetchCount.Load(), "same-request follower must not trigger a separate fetch")

			if leaderResult.leaderErr != nil {
				assert.Error(t, leaderErr)
				assert.Error(t, followerErr, "same-request follower should surface leader's error")
				assert.Equal(t, leaderErr.Error(), followerErr.Error())
			} else {
				assert.NoError(t, leaderErr)
				assert.NoError(t, followerErr)
				assert.Equal(t, leaderResult.leaderValue, leaderVal)
				assert.Equal(t, leaderResult.leaderValue, followerVal, "same-request follower should get leader's result")
			}
		})
	}
}

// --- Do: context cancellation ---

func TestDo_ContextCancellation(t *testing.T) {
	t.Parallel()

	t.Run("cancelled_context_returns_error_without_aborting_flight", func(t *testing.T) {
		t.Parallel()
		m := NewManager[string, string](
			WithSlowThreshold[string, string](5 * time.Second),
		)

		leaderCtx, leaderCancel := context.WithCancel(context.Background())
		fetchStarted := make(chan struct{})
		followerJoined := make(chan struct{})

		var wg sync.WaitGroup
		var leaderVal, followerVal string
		var leaderErr, followerErr error

		// Leader — will have its context cancelled mid-flight
		wg.Add(1)
		go func() {
			defer wg.Done()
			leaderVal, leaderErr = m.Do(leaderCtx, "key", func(_ context.Context) (FetchResult[string], error) {
				close(fetchStarted)
				<-followerJoined
				leaderCancel() // cancel leader's caller context
				time.Sleep(50 * time.Millisecond)
				return FetchResult[string]{Value: "completed", Policy: CacheWithTTL, TTL: time.Minute}, nil
			}, nil)
		}()

		// Follower — uses a separate, non-cancelled context
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-fetchStarted
			close(followerJoined)
			followerVal, followerErr = m.Do(context.Background(), "key", func(_ context.Context) (FetchResult[string], error) {
				return FetchResult[string]{Value: "should-not-run"}, nil
			}, nil)
		}()

		wg.Wait()

		// Leader's context was cancelled → awaitResult returns ctx.Err()
		assert.ErrorIs(t, leaderErr, context.Canceled)
		assert.Equal(t, "", leaderVal)

		// Follower still gets the value — the in-flight fetch was not aborted
		assert.NoError(t, followerErr)
		assert.Equal(t, "completed", followerVal)
	})
}

// --- Do: slow threshold ---

func TestDo_SlowThreshold(t *testing.T) {
	t.Parallel()

	t.Run("slow_threshold_triggers_independent_retry", func(t *testing.T) {
		t.Parallel()
		m := NewManager[string, string](
			WithSlowThreshold[string, string](10 * time.Millisecond),
		)

		var fetchCount atomic.Int32
		fetchStarted := make(chan struct{})

		var wg sync.WaitGroup
		var followerVal string
		var leaderErr, followerErr error

		// Leader — slow fetch, signals when started
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, leaderErr = m.Do(context.Background(), "key", func(_ context.Context) (FetchResult[string], error) {
				fetchCount.Add(1)
				close(fetchStarted)
				time.Sleep(200 * time.Millisecond)
				return FetchResult[string]{Value: "leader-val", Policy: CacheWithTTL, TTL: time.Minute}, nil
			}, nil)
		}()

		// Follower — joins after leader is in-flight, times out, retries independently
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-fetchStarted
			followerVal, followerErr = m.Do(context.Background(), "key", func(_ context.Context) (FetchResult[string], error) {
				fetchCount.Add(1)
				return FetchResult[string]{Value: "follower-val", Policy: CacheWithTTL, TTL: time.Minute}, nil
			}, nil)
		}()

		wg.Wait()

		assert.NoError(t, leaderErr)
		assert.NoError(t, followerErr)
		assert.Equal(t, "follower-val", followerVal, "follower should get its own fetch result")
		assert.GreaterOrEqual(t, fetchCount.Load(), int32(2), "follower should have fetched independently after timeout")
	})

	t.Run("slow_threshold_result_arrives_before_timer", func(t *testing.T) {
		t.Parallel()
		m := NewManager[string, string](
			WithSlowThreshold[string, string](5 * time.Second),
		)

		var fetchCount atomic.Int32
		fetchStarted := make(chan struct{})

		n := 10
		var wg sync.WaitGroup
		results := make([]string, n)
		errs := make([]error, n)

		// Leader — fast fetch, signals when started
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[0], errs[0] = m.Do(context.Background(), "key", func(_ context.Context) (FetchResult[string], error) {
				fetchCount.Add(1)
				close(fetchStarted)
				time.Sleep(5 * time.Millisecond) // fast — well under 5s threshold
				return FetchResult[string]{Value: "fast", Policy: CacheWithTTL, TTL: time.Minute}, nil
			}, nil)
		}()

		// Followers — join after leader is in-flight, result arrives before timer
		for i := 1; i < n; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				<-fetchStarted
				results[idx], errs[idx] = m.Do(context.Background(), "key", func(_ context.Context) (FetchResult[string], error) {
					fetchCount.Add(1)
					return FetchResult[string]{Value: "should-not-run"}, nil
				}, nil)
			}(i)
		}
		wg.Wait()

		for i := range n {
			assert.NoError(t, errs[i], "goroutine %d", i)
			assert.Equal(t, "fast", results[i], "goroutine %d", i)
		}
		assert.Equal(t, int32(1), fetchCount.Load(), "timer should not fire when result arrives quickly")
	})
}

// --- Do: follower retry on leader error ---

func TestDo_FollowerRetry(t *testing.T) {
	t.Parallel()

	t.Run("follower_retries_on_leader_error_when_enabled", func(t *testing.T) {
		t.Parallel()
		m := NewManager[string, string](
			WithRetryFollowerOnError[string, string](true),
		)

		fetchStarted := make(chan struct{})
		var fetchCount atomic.Int32

		var wg sync.WaitGroup
		var leaderErr, followerErr error
		var followerVal string

		// Leader — errors out
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, leaderErr = m.Do(context.Background(), "key", func(_ context.Context) (FetchResult[string], error) {
				fetchCount.Add(1)
				close(fetchStarted)
				time.Sleep(50 * time.Millisecond)
				return FetchResult[string]{}, errors.New("leader-boom")
			}, nil)
		}()

		// Follower — joins flight, gets error, retries independently and succeeds
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-fetchStarted
			followerVal, followerErr = m.Do(context.Background(), "key", func(_ context.Context) (FetchResult[string], error) {
				fetchCount.Add(1)
				return FetchResult[string]{Value: "recovered", Policy: CacheWithTTL, TTL: time.Minute}, nil
			}, nil)
		}()

		wg.Wait()

		assert.Error(t, leaderErr)
		assert.NoError(t, followerErr, "follower should succeed after retrying")
		assert.Equal(t, "recovered", followerVal)
		assert.GreaterOrEqual(t, fetchCount.Load(), int32(2), "follower should have fetched independently")
	})

	t.Run("follower_returns_error_when_retry_disabled", func(t *testing.T) {
		t.Parallel()
		m := NewManager[string, string]() // retryFollowerOnError defaults to false

		fetchStarted := make(chan struct{})
		var fetchCount atomic.Int32

		var wg sync.WaitGroup
		var leaderErr, followerErr error

		// Leader — errors out
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, leaderErr = m.Do(context.Background(), "key", func(_ context.Context) (FetchResult[string], error) {
				fetchCount.Add(1)
				close(fetchStarted)
				time.Sleep(50 * time.Millisecond)
				return FetchResult[string]{}, errors.New("leader-boom")
			}, nil)
		}()

		// Follower — joins flight, gets leader's error directly
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-fetchStarted
			_, followerErr = m.Do(context.Background(), "key", func(_ context.Context) (FetchResult[string], error) {
				fetchCount.Add(1)
				return FetchResult[string]{Value: "should-not-run"}, nil
			}, nil)
		}()

		wg.Wait()

		assert.Error(t, leaderErr)
		assert.Error(t, followerErr, "follower should surface leader's error")
		assert.Equal(t, "leader-boom", followerErr.Error())
		assert.Equal(t, int32(1), fetchCount.Load(), "follower should not retry")
	})
}
