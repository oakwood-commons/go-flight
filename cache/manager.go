// Package cache provides a multi-tier cache manager with singleflight deduplication.
package cache

import (
	"context"
	"time"

	"github.com/olisajc/go-flight/flightgroup"
)

// CachePolicy controls how a fetch result is cached.
type CachePolicy int

const (
	DoNotCache        CachePolicy = iota // fetch succeeded, do not cache
	CacheWithTTL                         // cache with FetchResult.TTL
	CacheIndefinitely                    // cache with no expiry
)

// FetchResult is returned by a FetchFunc to the Manager.
type FetchResult[V any] struct {
	Value  V
	TTL    time.Duration // meaningful only when Policy == CacheWithTTL
	Policy CachePolicy
}

// FetchFunc is called when no cache layer has the value.
type FetchFunc[V any] func(ctx context.Context) (FetchResult[V], error)

// GetResult is returned by Manager to the caller.
type GetResult[V any] struct {
	Value  V
	Source string
	OK     bool
}

// RequestIDExtractor pulls a caller ID from context for singleflight tagging.
type RequestIDExtractor func(ctx context.Context) uint64

// Hooks provides optional callbacks for observability.
// All fields are optional; nil hooks are silently skipped.
type Hooks struct {
	OnCacheHit   func(source string) // called on any cache hit, with the store name
	OnSuccess    func()              // called after a successful fetch
	OnFetchError func(err error)     // called when fetchFn returns an error
}

// ManagerOption configures a Manager.
type ManagerOption[K comparable, V any] func(*Manager[K, V])

type namedStore[K comparable, V any] struct {
	name  string
	store Store[K, V]
}

// Manager is a multi-tier cache with singleflight deduplication.
// Stores are checked in registration order; the first hit wins.
// Use Do to fetch with deduplication; Get and Set for direct access.
type Manager[K comparable, V any] struct {
	sf                   flightgroup.Coordinator[K, V]
	slowThreshold        time.Duration // followers bail and retry independently after this duration
	expiryThreshold      time.Duration // FetchResults with TTL <= this are not cached if expiryThreshold == 0, all CacheWithTTL results are cached
	retryFollowerOnError bool          // if true, followers retry independently when the leader errors
	stores               []namedStore[K, V]
	reqID                RequestIDExtractor
}

// WithRetryFollowerOnError controls whether followers retry independently when the
// leader's fetch fails. When false (default), the leader error is returned directly.
func WithRetryFollowerOnError[K comparable, V any](retry bool) ManagerOption[K, V] {
	return func(m *Manager[K, V]) { m.retryFollowerOnError = retry }
}

// WithStore appends a named cache store. Stores are queried in the order they are added.
func WithStore[K comparable, V any](name string, s Store[K, V]) ManagerOption[K, V] {
	return func(m *Manager[K, V]) {
		m.stores = append(m.stores, namedStore[K, V]{name: name, store: s})
	}
}

// WithSlowThreshold sets the duration after which a follower abandons the in-flight
// result and issues its own fetch rather than waiting for the leader.
func WithSlowThreshold[K comparable, V any](d time.Duration) ManagerOption[K, V] {
	return func(m *Manager[K, V]) { m.slowThreshold = d }
}

// WithExpiryThreshold sets the minimum TTL a CacheWithTTL result must have to be stored.
// Results with TTL <= threshold are treated as DoNotCache.
func WithExpiryThreshold[K comparable, V any](d time.Duration) ManagerOption[K, V] {
	return func(m *Manager[K, V]) { m.expiryThreshold = d }
}

// WithRequestIDExtractor sets the function used to derive a caller ID from context.
// Callers sharing the same non-zero ID skip the slow-follower path and read directly
// from the leader channel, making them behave as co-owners of the same flight.
func WithRequestIDExtractor[K comparable, V any](fn RequestIDExtractor) ManagerOption[K, V] {
	return func(m *Manager[K, V]) { m.reqID = fn }
}

// NewManager creates a Manager. Configure stores and behaviour with option functions.
func NewManager[K comparable, V any](opts ...ManagerOption[K, V]) *Manager[K, V] {
	m := &Manager[K, V]{
		sf:    flightgroup.NewGroup[K, V](),
		reqID: func(context.Context) uint64 { return 0 },
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Get checks all stores in order and returns the first hit.
func (m *Manager[K, V]) Get(ctx context.Context, key K) GetResult[V] {
	for _, ns := range m.stores {
		if val, ok := ns.store.Get(ctx, key); ok {
			return GetResult[V]{Value: val, Source: ns.name, OK: true}
		}
	}
	return GetResult[V]{}
}

// Set writes the value to every registered store with the given TTL.
// Writes are best-effort; store errors are ignored.
func (m *Manager[K, V]) Set(ctx context.Context, key K, value V, ttl time.Duration) {
	for _, ns := range m.stores {
		ns.store.Set(ctx, key, value, ttl)
	}
}

// shouldCache reports whether the result should be written to the stores.
func (m *Manager[K, V]) shouldCache(result FetchResult[V]) bool {
	switch result.Policy {
	case DoNotCache:
		return false
	case CacheIndefinitely:
		return true
	case CacheWithTTL:
		return m.expiryThreshold <= 0 || result.TTL > m.expiryThreshold
	default:
		return false
	}
}

// storeTTL returns the TTL to pass to Set. CacheIndefinitely maps to TTLIndefinite.
func (m *Manager[K, V]) storeTTL(result FetchResult[V]) time.Duration {
	if result.Policy == CacheIndefinitely {
		return TTLIndefinite
	}
	return result.TTL
}

// awaitResult blocks until the flight channel delivers or the caller context is cancelled.
func (m *Manager[K, V]) awaitResult(ctx context.Context, ch <-chan flightgroup.Result[V]) (V, error) {
	select {
	case res := <-ch:
		return res.Value, res.Err
	case <-ctx.Done():
		var zero V
		return zero, ctx.Err()
	}
}

// Do returns the cached value for key, fetching it if absent.
// Concurrent calls for the same key are deduplicated: one leader fetches, others wait.
// Callers sharing a non-zero request ID co-own the flight and bypass follower retry logic.
// A cancelled caller context does not abort an in-flight fetch shared with others.
func (m *Manager[K, V]) Do(ctx context.Context, key K, fetchFn FetchFunc[V], hooks *Hooks) (V, error) {
	callerTag := m.reqID(ctx)

	order := m.sf.Do(key, callerTag, func() (V, error) {
		if res := m.Get(ctx, key); res.OK {
			if hooks != nil && hooks.OnCacheHit != nil {
				hooks.OnCacheHit(res.Source)
			}
			return res.Value, nil
		}
		fetchCtx := context.WithoutCancel(ctx)
		result, err := fetchFn(fetchCtx)
		if err != nil {
			m.sf.Forget(key)
			if hooks != nil && hooks.OnFetchError != nil {
				hooks.OnFetchError(err)
			}
			return result.Value, err
		}
		if hooks != nil && hooks.OnSuccess != nil {
			hooks.OnSuccess()
		}
		if m.shouldCache(result) {
			m.Set(ctx, key, result.Value, m.storeTTL(result))
		}
		return result.Value, nil
	})
	sameReq := callerTag != 0 && callerTag == order.LeaderTag
	if order.Leader || sameReq {
		return m.awaitResult(ctx, order.Ch())
	}

	var timerC <-chan time.Time
	if m.slowThreshold > 0 {
		timer := time.NewTimer(m.slowThreshold)
		defer timer.Stop()
		timerC = timer.C
	}

	return m.followerFetch(ctx, key, order.Ch(), timerC, fetchFn, hooks)
}

// followerFetch waits for the leader result, the caller context, or the slow threshold.
// On timeout it always retries independently. On leader error it retries only if
// retryFollowerOnError is enabled, otherwise it returns the error directly.
func (m *Manager[K, V]) followerFetch(ctx context.Context, key K, ch <-chan flightgroup.Result[V], timerC <-chan time.Time, fetch FetchFunc[V], hooks *Hooks) (V, error) {
	select {
	case res := <-ch:
		if res.Err != nil && m.retryFollowerOnError {
			return m.retryViaFlight(ctx, key, fetch, hooks)
		}
		return res.Value, res.Err
	case <-ctx.Done():
		var zero V
		return zero, ctx.Err()
	case <-timerC:
		m.sf.Forget(key)
		return m.retryViaFlight(ctx, key, fetch, hooks)
	}
}

// retryViaFlight starts or joins a new singleflight for key, used when a follower
// needs to fetch independently of the original leader.
func (m *Manager[K, V]) retryViaFlight(ctx context.Context, key K, fetch FetchFunc[V], hooks *Hooks) (V, error) {
	retryCtx := context.WithoutCancel(ctx)
	order := m.sf.Do(key, m.reqID(ctx), func() (V, error) {
		return m.fetchAndCache(retryCtx, key, fetch, hooks)
	})
	select {
	case res := <-order.Ch():
		return res.Value, res.Err
	case <-ctx.Done():
		var zero V
		return zero, ctx.Err()
	}
}

// fetchAndCache calls fetch, fires hooks, and writes to stores on success.
func (m *Manager[K, V]) fetchAndCache(ctx context.Context, key K, fetch FetchFunc[V], hooks *Hooks) (V, error) {
	result, err := fetch(ctx)
	if err != nil {
		var zero V
		return zero, err
	}
	if hooks != nil && hooks.OnSuccess != nil {
		hooks.OnSuccess()
	}
	if m.shouldCache(result) {
		m.Set(ctx, key, result.Value, m.storeTTL(result))
	}
	return result.Value, nil
}
