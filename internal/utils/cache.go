package utils

// This is a minimal, self-contained replacement for the original yaklang cache
// helpers. The ported Java tooling only needs a simple TTL cache with Get/Set, so
// this avoids any third-party cache dependency. Expiry is evaluated lazily on Get.

import (
	"sync"
	"time"
)

type cacheEntry[T any] struct {
	value    T
	expireAt time.Time // zero means "never expires"
}

// CacheWithKey is a concurrency-safe key/value cache with optional per-instance TTL.
type CacheWithKey[U comparable, T any] struct {
	mu  sync.RWMutex
	ttl time.Duration
	m   map[U]cacheEntry[T]
}

// Cache is a CacheWithKey keyed by string.
type Cache[T any] struct {
	*CacheWithKey[string, T]
}

// NewTTLCacheWithKey creates a cache whose entries expire after the given TTL.
// If no TTL is supplied (or it is <= 0), entries never expire.
func NewTTLCacheWithKey[U comparable, T any](ttls ...time.Duration) *CacheWithKey[U, T] {
	var ttl time.Duration
	if len(ttls) > 0 {
		ttl = ttls[0]
	}
	return &CacheWithKey[U, T]{
		ttl: ttl,
		m:   make(map[U]cacheEntry[T]),
	}
}

// NewTTLCache creates a string-keyed TTL cache.
func NewTTLCache[T any](ttls ...time.Duration) *Cache[T] {
	return &Cache[T]{CacheWithKey: NewTTLCacheWithKey[string, T](ttls...)}
}

// Set stores value under key, applying the cache TTL (if any).
func (c *CacheWithKey[U, T]) Set(key U, value T) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := cacheEntry[T]{value: value}
	if c.ttl > 0 {
		entry.expireAt = time.Now().Add(c.ttl)
	}
	c.m[key] = entry
}

// Get returns the value stored under key and whether it was present and unexpired.
func (c *CacheWithKey[U, T]) Get(key U) (T, bool) {
	c.mu.RLock()
	entry, ok := c.m[key]
	c.mu.RUnlock()
	if !ok {
		var zero T
		return zero, false
	}
	if !entry.expireAt.IsZero() && time.Now().After(entry.expireAt) {
		c.mu.Lock()
		if cur, still := c.m[key]; still && cur.expireAt == entry.expireAt {
			delete(c.m, key)
		}
		c.mu.Unlock()
		var zero T
		return zero, false
	}
	return entry.value, true
}

// GetOrLoad returns the cached value for key, or loads, stores and returns it.
func (c *CacheWithKey[U, T]) GetOrLoad(key U, loader func() (T, error)) (T, error) {
	if v, ok := c.Get(key); ok {
		return v, nil
	}
	v, err := loader()
	if err != nil {
		var zero T
		return zero, err
	}
	c.Set(key, v)
	return v, nil
}

// Delete removes key from the cache.
func (c *CacheWithKey[U, T]) Delete(key U) {
	c.mu.Lock()
	delete(c.m, key)
	c.mu.Unlock()
}

// Close clears the cache.
func (c *CacheWithKey[U, T]) Close() {
	c.mu.Lock()
	c.m = make(map[U]cacheEntry[T])
	c.mu.Unlock()
}
