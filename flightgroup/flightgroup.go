// Package flightgroup provides a generic singleflight that deduplicates
// concurrent calls by key under a single lock.
package flightgroup

import "sync"

// Result holds the outcome of a flight execution.
type Result[T any] struct {
	Value T
	Err   error
}

// ExecutionOrder is returned by Do. Leader is set under the lock so the caller
// knows its role immediately. The result arrives via Ch.
type ExecutionOrder[T any] struct {
	Leader    bool
	LeaderTag uint64
	ch        <-chan Result[T]
}

// Ch returns the result channel for use in select statements.
func (e ExecutionOrder[T]) Ch() <-chan Result[T] {
	return e.ch
}

// Func is the function signature accepted by Do.
type Func[T any] func() (T, error)

// flight is an in-progress execution for a given key.
type flight[T any] struct {
	callerTag uint64
	waiters   []chan<- Result[T]
}

// Coordinator captures the behavior needed by higher-level packages that want
// to depend on singleflight behavior without binding to a concrete Group.
type Coordinator[K comparable, V any] interface {
	Do(key K, callerTag uint64, fn Func[V]) ExecutionOrder[V]
	Forget(key K)
}

// Group deduplicates function calls by key. The first caller becomes the
// executor; subsequent callers share its result.
// Must be created with NewGroup; the zero value is not valid.
type Group[K comparable, T any] struct {
	mu      sync.Mutex
	flights map[K]*flight[T]
}

var _ Coordinator[any, any] = (*Group[any, any])(nil)

// NewGroup creates a ready-to-use Group.
func NewGroup[K comparable, T any]() *Group[K, T] {
	return &Group[K, T]{
		flights: make(map[K]*flight[T]),
	}
}

// Do registers or joins a flight for key. The first caller becomes the executor
// (Leader=true); subsequent callers share the same result. callerTag is an
// opaque ID surfaced to followers via ExecutionOrder.LeaderTag.
// Panics in fn are recovered and delivered as errors to all callers.
func (g *Group[K, T]) Do(key K, callerTag uint64, fn Func[T]) ExecutionOrder[T] {
	g.mu.Lock()
	ch := make(chan Result[T], 1)
	if f, ok := g.flights[key]; ok {
		f.waiters = append(f.waiters, ch)
		g.mu.Unlock()
		return ExecutionOrder[T]{Leader: false, LeaderTag: f.callerTag, ch: ch}
	}
	f := &flight[T]{callerTag: callerTag, waiters: []chan<- Result[T]{ch}}
	g.flights[key] = f
	g.mu.Unlock()
	go g.execute(key, f, fn)
	return ExecutionOrder[T]{Leader: true, LeaderTag: callerTag, ch: ch}
}

// execute runs fn and delivers the result to all waiters.
// Panics are recovered so waiters are never orphaned.
func (g *Group[K, T]) execute(key K, f *flight[T], fn Func[T]) {
	result := Result[T]{}
	defer func() {
		if r := recover(); r != nil {
			result = Result[T]{Err: newPanicErr(r)}
		}
		g.mu.Lock()
		if g.flights[key] == f {
			delete(g.flights, key)
		}
		waiters := f.waiters
		g.mu.Unlock()
		for _, waiter := range waiters {
			waiter <- result
			close(waiter)
		}
	}()
	result.Value, result.Err = fn()
}

// Forget detaches key so the next Do starts a new flight. The in-progress
// flight still completes and delivers to its existing waiters.
func (g *Group[K, T]) Forget(key K) {
	g.mu.Lock()
	delete(g.flights, key)
	g.mu.Unlock()
}
