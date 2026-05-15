package cache

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// FuzzManagerDo exercises the cache manager with arbitrary keys and concurrent callers.
func FuzzManagerDo(f *testing.F) {
	f.Add("key", uint8(1), true)
	f.Add("", uint8(5), false)
	f.Add("special-!@#$", uint8(10), true)

	f.Fuzz(func(t *testing.T, key string, numCallers uint8, cacheResult bool) {
		if numCallers == 0 {
			numCallers = 1
		}
		if numCallers > 30 {
			numCallers = 30
		}

		store := &fuzzStore[string, string]{data: make(map[string]string)}
		m := NewManager[string, string](WithStore("l1", store))
		ctx := context.Background()
		n := int(numCallers)
		expected := fmt.Sprintf("value-for-%s", key)

		policy := DoNotCache
		if cacheResult {
			policy = CacheWithTTL
		}

		var wg sync.WaitGroup
		wg.Add(n)
		for range n {
			go func() {
				defer wg.Done()
				result, err := m.Do(ctx, key, func(_ context.Context) (FetchResult[string], error) {
					return FetchResult[string]{Value: expected, Policy: policy, TTL: time.Minute}, nil
				}, nil)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if result != expected {
					t.Errorf("got %q, want %q", result, expected)
				}
			}()
		}
		wg.Wait()
	})
}

// fuzzStore is a thread-safe Store implementation for fuzz testing.
type fuzzStore[K comparable, V any] struct {
	mu   sync.RWMutex
	data map[K]V
}

func (s *fuzzStore[K, V]) Get(_ context.Context, key K) (V, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok
}

func (s *fuzzStore[K, V]) Set(_ context.Context, key K, value V, _ time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
}
