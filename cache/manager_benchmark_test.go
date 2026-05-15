package cache

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// concurrentMapStore is a thread-safe Store for benchmarking concurrent access.
type concurrentMapStore[K comparable, V any] struct {
	mu   sync.RWMutex
	data map[K]V
}

func newConcurrentMapStore[K comparable, V any]() *concurrentMapStore[K, V] {
	return &concurrentMapStore[K, V]{data: make(map[K]V)}
}

func (s *concurrentMapStore[K, V]) Get(_ context.Context, key K) (V, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok
}

func (s *concurrentMapStore[K, V]) Set(_ context.Context, key K, value V, _ time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
}

func BenchmarkManager_Get_Hit(b *testing.B) {
	b.ReportAllocs()
	s := newMapStore[string, string]()
	s.data["key"] = "cached-value"
	m := NewManager[string, string](WithStore("l1", s))
	ctx := context.Background()

	b.ResetTimer()
	for b.Loop() {
		m.Get(ctx, "key")
	}
}

func BenchmarkManager_Get_Miss(b *testing.B) {
	b.ReportAllocs()
	s := newMapStore[string, string]()
	m := NewManager[string, string](WithStore("l1", s))
	ctx := context.Background()

	b.ResetTimer()
	for b.Loop() {
		m.Get(ctx, "missing")
	}
}

func BenchmarkManager_Get_MultiStore(b *testing.B) {
	b.ReportAllocs()
	s1 := newMapStore[string, string]()
	s2 := newMapStore[string, string]()
	s2.data["key"] = "from-l2"
	m := NewManager[string, string](
		WithStore("l1", s1),
		WithStore("l2", s2),
	)
	ctx := context.Background()

	b.ResetTimer()
	for b.Loop() {
		m.Get(ctx, "key")
	}
}

func BenchmarkManager_Set(b *testing.B) {
	b.ReportAllocs()
	s := newConcurrentMapStore[string, string]()
	m := NewManager[string, string](WithStore("l1", s))
	ctx := context.Background()

	b.ResetTimer()
	for b.Loop() {
		m.Set(ctx, "key", "value", time.Minute)
	}
}

func BenchmarkManager_Do_CacheHit(b *testing.B) {
	b.ReportAllocs()
	s := newConcurrentMapStore[string, string]()
	s.mu.Lock()
	s.data["key"] = "cached"
	s.mu.Unlock()
	m := NewManager[string, string](WithStore("l1", s))
	ctx := context.Background()

	fetchFn := func(_ context.Context) (FetchResult[string], error) {
		return FetchResult[string]{Value: "fresh", Policy: CacheWithTTL, TTL: time.Minute}, nil
	}

	b.ResetTimer()
	for b.Loop() {
		_, _ = m.Do(ctx, "key", fetchFn, nil)
	}
}

func BenchmarkManager_Do_CacheMiss(b *testing.B) {
	b.ReportAllocs()
	m := NewManager[string, string]()
	ctx := context.Background()

	fetchFn := func(_ context.Context) (FetchResult[string], error) {
		return FetchResult[string]{Value: "fetched", Policy: DoNotCache}, nil
	}

	b.ResetTimer()
	for b.Loop() {
		_, _ = m.Do(ctx, "key", fetchFn, nil)
	}
}

func BenchmarkManager_Do_ConcurrentSameKey(b *testing.B) {
	b.ReportAllocs()
	m := NewManager[string, string]()
	ctx := context.Background()

	fetchFn := func(_ context.Context) (FetchResult[string], error) {
		return FetchResult[string]{Value: "v", Policy: DoNotCache}, nil
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = m.Do(ctx, "shared", fetchFn, nil)
		}
	})
}

func BenchmarkManager_Do_ConcurrentDifferentKeys(b *testing.B) {
	b.ReportAllocs()
	s := newConcurrentMapStore[string, string]()
	m := NewManager[string, string](WithStore("l1", s))
	ctx := context.Background()

	fetchFn := func(_ context.Context) (FetchResult[string], error) {
		return FetchResult[string]{Value: "v", Policy: CacheWithTTL, TTL: time.Minute}, nil
	}

	var benchKeyCounter atomic.Uint64

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			id := benchKeyCounter.Add(1)
			key := fmt.Sprintf("key-%d", id)
			_, _ = m.Do(ctx, key, fetchFn, nil)
		}
	})
}
