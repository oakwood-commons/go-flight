package cache

import (
	"context"
	"math"
	"time"
)

// TTLIndefinite is passed to Store.Set to signal that a value should
// be stored with no expiry. Store implementations that don't support
// indefinite storage may treat it as their maximum TTL.
const TTLIndefinite time.Duration = math.MaxInt64

// Store is the interface that cache backends must implement.
// Set receives TTLIndefinite (math.MaxInt64) to mean "no expiry".
// Set has no error return; implementations must handle failures internally.
type Store[K comparable, V any] interface {
	Get(ctx context.Context, key K) (V, bool)
	Set(ctx context.Context, key K, value V, ttl time.Duration)
}
