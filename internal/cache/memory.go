package cache

import (
	"container/list"
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// entry is an internal cache entry that holds the cached response, its
// expiration time, and a pointer to its position in the LRU list.
type entry struct {
	key     string
	value   *CachedResponse
	expiry  time.Time
	element *list.Element
}

// MemoryStore is an in-memory LRU cache with TTL-based expiration.
// It is safe for concurrent use by multiple goroutines.
type MemoryStore struct {
	mu         sync.RWMutex
	items      map[string]*entry
	evictList  *list.List
	maxEntries int

	hits      atomic.Int64
	misses    atomic.Int64
	bytesUsed atomic.Int64
}

// NewMemoryStore creates a new MemoryStore that holds at most maxEntries items.
// When the limit is reached, the least recently used entry is evicted.
func NewMemoryStore(maxEntries int) *MemoryStore {
	if maxEntries <= 0 {
		maxEntries = 1024
	}
	return &MemoryStore{
		items:      make(map[string]*entry, maxEntries),
		evictList:  list.New(),
		maxEntries: maxEntries,
	}
}

// Get retrieves a cached response by key. Expired entries are treated as
// misses and removed lazily. Valid entries are promoted to the front of the
// LRU list.
func (m *MemoryStore) Get(_ context.Context, key string) (*CachedResponse, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.items[key]
	if !ok {
		m.misses.Add(1)
		return nil, false, nil
	}

	// Lazy expiration: remove the entry if it has expired.
	if time.Now().After(e.expiry) {
		m.removeLocked(e)
		m.misses.Add(1)
		return nil, false, nil
	}

	// Move to front of LRU list (most recently used).
	m.evictList.MoveToFront(e.element)
	m.hits.Add(1)
	return e.value, true, nil
}

// Set stores a response in the cache. If the key already exists, its value
// and expiry are updated and it is moved to the front of the LRU list.
// If the cache is full, the least recently used entry is evicted.
func (m *MemoryStore) Set(_ context.Context, key string, resp *CachedResponse, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	size := int64(len(resp.Body))

	// Update existing entry.
	if e, ok := m.items[key]; ok {
		m.bytesUsed.Add(-int64(len(e.value.Body)))
		e.value = resp
		e.expiry = time.Now().Add(ttl)
		m.evictList.MoveToFront(e.element)
		m.bytesUsed.Add(size)
		return nil
	}

	// Evict oldest if at capacity.
	for m.evictList.Len() >= m.maxEntries {
		m.evictOldestLocked()
	}

	// Insert new entry.
	e := &entry{
		key:    key,
		value:  resp,
		expiry: time.Now().Add(ttl),
	}
	e.element = m.evictList.PushFront(e)
	m.items[key] = e
	m.bytesUsed.Add(size)
	return nil
}

// Delete removes a single entry from the cache.
func (m *MemoryStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if e, ok := m.items[key]; ok {
		m.removeLocked(e)
	}
	return nil
}

// Stats returns a snapshot of the cache performance metrics.
func (m *MemoryStore) Stats() CacheStats {
	m.mu.RLock()
	entries := int64(len(m.items))
	m.mu.RUnlock()

	return CacheStats{
		Hits:      m.hits.Load(),
		Misses:    m.misses.Load(),
		Entries:   entries,
		BytesUsed: m.bytesUsed.Load(),
	}
}

// Close releases all resources held by the MemoryStore. After Close is
// called the store must not be used.
func (m *MemoryStore) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.items = nil
	m.evictList.Init()
	m.bytesUsed.Store(0)
	return nil
}

// removeLocked removes an entry from both the map and the LRU list.
// The caller must hold m.mu.
func (m *MemoryStore) removeLocked(e *entry) {
	m.evictList.Remove(e.element)
	delete(m.items, e.key)
	m.bytesUsed.Add(-int64(len(e.value.Body)))
}

// evictOldestLocked removes the least recently used entry from the cache.
// The caller must hold m.mu.
func (m *MemoryStore) evictOldestLocked() {
	back := m.evictList.Back()
	if back == nil {
		return
	}
	m.removeLocked(back.Value.(*entry))
}
