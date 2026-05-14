package flightgroup

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDo(t *testing.T) {
	t.Parallel()
	t.Run("single flight", func(t *testing.T) {
		t.Parallel()
		g := NewGroup[string, int]()
		result := g.Do("key", 0, func() (int, error) {
			return 42, nil
		})
		res := <-result.Ch()
		assert.Equal(t, 42, res.Value)
		assert.NoError(t, res.Err)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()
		g := NewGroup[string, int]()
		result := g.Do("key", 0, func() (int, error) {
			return 0, fmt.Errorf("some error")
		})
		res := <-result.Ch()
		assert.Equal(t, 0, res.Value)
		assert.Error(t, res.Err)
	})
	t.Run("dedup_concurrent_same_key", func(t *testing.T) {
		t.Parallel()
		g := NewGroup[string, string]()

		var fetchCount atomic.Int32
		fetch := func() (string, error) { //nolint:unparam // error intentionally nil in test
			fetchCount.Add(1)
			time.Sleep(50 * time.Millisecond) // simulate network latency
			return "deduped-token", nil
		}
		ready := make(chan struct{})
		var arrived atomic.Int32
		var wg sync.WaitGroup
		n := 100
		results := make([]Result[string], n)
		leaders := make([]bool, n)
		for i := range n {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				if arrived.Add(1) == int32(n) {
					close(ready) // last goroutine to arrive unblocks all
				}
				<-ready
				handle := g.Do("same-key", 0, fetch)
				leaders[idx] = handle.Leader
				results[idx] = <-handle.Ch()
			}(i)
		}

		wg.Wait()

		for i := range n {
			assert.NoError(t, results[i].Err)
			assert.Equal(t, "deduped-token", results[i].Value)
		}

		assert.Equal(t, int32(1), fetchCount.Load())

		// Exactly one leader among all callers
		leaderCount := 0
		for _, l := range leaders {
			if l {
				leaderCount++
			}
		}
		assert.Equal(t, 1, leaderCount)
	})

	t.Run("panic_recovered_as_error", func(t *testing.T) {
		t.Parallel()
		g := NewGroup[string, string]()
		handle := g.Do("panic-key", 0, func() (string, error) {
			panic("something broke")
		})
		res := <-handle.Ch()
		assert.Error(t, res.Err)
		assert.Contains(t, res.Err.Error(), "something broke")
		assert.Equal(t, "", res.Value)
	})

	t.Run("panic_does_not_orphan_followers", func(t *testing.T) {
		t.Parallel()
		g := NewGroup[string, string]()

		ready := make(chan struct{})
		var arrived atomic.Int32
		var wg sync.WaitGroup
		n := 5
		results := make([]Result[string], n)
		for i := range n {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				if arrived.Add(1) == int32(n) {
					close(ready) // last goroutine to arrive unblocks all
				}
				<-ready
				handle := g.Do("panic-key", 0, func() (string, error) {
					panic("boom")
				})
				results[idx] = <-handle.Ch()
			}(i)
		}

		wg.Wait()

		for i := range n {
			assert.Error(t, results[i].Err)
			assert.Contains(t, results[i].Err.Error(), "boom")
		}
	})
}

func TestLeaderTag(t *testing.T) {
	t.Parallel()

	t.Run("leader_gets_own_tag", func(t *testing.T) {
		t.Parallel()
		g := NewGroup[string, string]()
		started := make(chan struct{})
		cont := make(chan struct{})
		order := g.Do("k", 42, func() (string, error) {
			close(started)
			<-cont
			return "v", nil
		})
		<-started
		assert.True(t, order.Leader)
		assert.Equal(t, uint64(42), order.LeaderTag)
		close(cont)
		<-order.Ch()
	})

	t.Run("follower_sees_leader_tag", func(t *testing.T) {
		t.Parallel()
		g := NewGroup[string, string]()
		started := make(chan struct{})
		cont := make(chan struct{})

		leader := g.Do("k", 100, func() (string, error) {
			close(started)
			<-cont
			return "v", nil
		})
		<-started
		follower := g.Do("k", 200, func() (string, error) {
			return "should-not-run", nil
		})
		assert.False(t, follower.Leader)
		assert.Equal(t, uint64(100), follower.LeaderTag, "follower should see the leader's tag")

		close(cont)
		<-leader.Ch()
		<-follower.Ch()
	})
}

func TestForget(t *testing.T) {
	t.Parallel()
	t.Run("forget_allows_new_flight_for_same_key", func(t *testing.T) {
		t.Parallel()
		g := NewGroup[string, string]()

		oldFlightStarted := make(chan struct{})
		oldFlightContinue := make(chan struct{})

		// Start a slow flight
		oldHandle := g.Do("key", 0, func() (string, error) {
			close(oldFlightStarted) // signal that the flight is in progress
			<-oldFlightContinue     // block until test says continue
			return "old-result", nil
		})

		// Wait for the old flight to be in progress
		<-oldFlightStarted

		// Forget the key — new callers should start a fresh flight
		g.Forget("key")

		// New caller for same key should become leader
		newHandle := g.Do("key", 0, func() (string, error) {
			return "new-result", nil
		})
		assert.True(t, newHandle.Leader, "expected new caller to be leader after Forget")

		// New flight completes independently
		newRes := <-newHandle.Ch()
		assert.NoError(t, newRes.Err)
		assert.Equal(t, "new-result", newRes.Value)

		// Unblock old flight — it should still deliver to its original waiter
		close(oldFlightContinue)
		oldRes := <-oldHandle.Ch()
		assert.NoError(t, oldRes.Err)
		assert.Equal(t, "old-result", oldRes.Value)
	})

	t.Run("forget_with_followers_still_delivers", func(t *testing.T) {
		t.Parallel()
		g := NewGroup[string, string]()

		started := make(chan struct{})
		cont := make(chan struct{})

		leader := g.Do("key", 0, func() (string, error) {
			close(started)
			<-cont
			return "result", nil
		})
		<-started

		// Add followers before forgetting
		follower1 := g.Do("key", 1, func() (string, error) {
			return "should-not-run", nil
		})
		follower2 := g.Do("key", 2, func() (string, error) {
			return "should-not-run", nil
		})

		assert.False(t, follower1.Leader)
		assert.False(t, follower2.Leader)

		// Forget while followers are waiting
		g.Forget("key")

		// Unblock the original flight
		close(cont)

		// All waiters should still receive the result
		leaderRes := <-leader.Ch()
		assert.NoError(t, leaderRes.Err)
		assert.Equal(t, "result", leaderRes.Value)

		f1Res := <-follower1.Ch()
		assert.NoError(t, f1Res.Err)
		assert.Equal(t, "result", f1Res.Value)

		f2Res := <-follower2.Ch()
		assert.NoError(t, f2Res.Err)
		assert.Equal(t, "result", f2Res.Value)
	})

	t.Run("key_reuse_after_completion", func(t *testing.T) {
		t.Parallel()
		g := NewGroup[string, string]()

		first := g.Do("key", 0, func() (string, error) {
			return "first", nil
		})
		firstRes := <-first.Ch()
		assert.NoError(t, firstRes.Err)
		assert.Equal(t, "first", firstRes.Value)

		// Same key should start a new flight
		second := g.Do("key", 0, func() (string, error) {
			return "second", nil
		})
		assert.True(t, second.Leader, "expected new leader after previous flight completed")

		secondRes := <-second.Ch()
		assert.NoError(t, secondRes.Err)
		assert.Equal(t, "second", secondRes.Value)
	})

	t.Run("forget_nonexistent_key_is_noop", func(t *testing.T) {
		t.Parallel()
		g := NewGroup[string, string]()
		assert.NotPanics(t, func() {
			g.Forget("never-used")
		})
	})
}
