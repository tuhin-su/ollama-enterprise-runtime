package memory

import (
	"time"

	ristretto "github.com/dgraph-io/ristretto/v2"
)

// RistrettoCache implements CacheProvider using Dgraph's Ristretto.
// It provides <100ns reads via a concurrent, lock-free TinyLFU admission policy.
type RistrettoCache struct {
	cache      *ristretto.Cache[string, any]
	defaultTTL time.Duration
}

// NewRistrettoCache creates a production-ready cache.
//
//   - numCounters should be ~10× the expected max items for good admission accuracy.
//   - maxCost is the maximum total cost (typically bytes).
//   - defaultTTL applies when SetWithTTL is not used explicitly.
func NewRistrettoCache(numCounters, maxCost int64, defaultTTL time.Duration) (*RistrettoCache, error) {
	cache, err := ristretto.NewCache(&ristretto.Config[string, any]{
		NumCounters: numCounters,
		MaxCost:     maxCost,
		BufferItems: 64, // number of keys per Get buffer
	})
	if err != nil {
		return nil, err
	}

	return &RistrettoCache{
		cache:      cache,
		defaultTTL: defaultTTL,
	}, nil
}

// Get retrieves an item. Returns (value, true) on hit.
func (c *RistrettoCache) Get(key string) (any, bool) {
	return c.cache.Get(key)
}

// Set stores an item with the default TTL.
func (c *RistrettoCache) Set(key string, value any, cost int64) bool {
	return c.cache.SetWithTTL(key, value, cost, c.defaultTTL)
}

// SetWithTTL stores an item with an explicit TTL.
func (c *RistrettoCache) SetWithTTL(key string, value any, cost int64, ttl time.Duration) bool {
	return c.cache.SetWithTTL(key, value, cost, ttl)
}

// Del removes an item.
func (c *RistrettoCache) Del(key string) {
	c.cache.Del(key)
}

// Clear empties the entire cache.
func (c *RistrettoCache) Clear() {
	c.cache.Clear()
}

// Close shuts down the cache, releasing all resources.
func (c *RistrettoCache) Close() {
	c.cache.Close()
}
