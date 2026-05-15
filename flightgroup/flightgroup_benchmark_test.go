package flightgroup

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func BenchmarkDo_SingleCaller(b *testing.B) {
	b.ReportAllocs()
	g := NewGroup[string, int]()

	b.ResetTimer()
	for b.Loop() {
		order := g.Do("key", 0, func() (int, error) {
			return 42, nil
		})
		<-order.Ch()
	}
}

func BenchmarkDo_ConcurrentSameKey(b *testing.B) {
	b.ReportAllocs()
	g := NewGroup[string, int]()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			order := g.Do("shared-key", 0, func() (int, error) {
				return 42, nil
			})
			<-order.Ch()
		}
	})
}

func BenchmarkDo_ConcurrentDifferentKeys(b *testing.B) {
	b.ReportAllocs()
	g := NewGroup[string, int]()

	var keyCounter atomic.Uint64

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Each iteration uses a unique key to exercise distinct flights
			id := fmt.Sprintf("key-%d", keyCounter.Add(1))
			order := g.Do(id, 0, func() (int, error) {
				return 42, nil
			})
			<-order.Ch()
		}
	})
}

func BenchmarkDo_ManyFollowers(b *testing.B) {
	b.ReportAllocs()
	g := NewGroup[string, int]()

	// Measures the cost of many goroutines joining the same flight sequentially.
	// Each iteration starts a fresh flight with one leader and N-1 followers.
	const followers = 10

	b.ResetTimer()
	for b.Loop() {
		var wg sync.WaitGroup
		wg.Add(followers + 1)
		for i := range followers + 1 {
			go func(idx int) {
				defer wg.Done()
				order := g.Do("key", uint64(idx), func() (int, error) {
					return 42, nil
				})
				<-order.Ch()
			}(i)
		}
		wg.Wait()
	}
}

func BenchmarkForget(b *testing.B) {
	b.ReportAllocs()
	g := NewGroup[string, int]()

	b.ResetTimer()
	for b.Loop() {
		g.Forget("nonexistent")
	}
}
