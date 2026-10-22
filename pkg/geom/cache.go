package geom

import (
	"container/list"
	"sync"
	"time"
)

// Cache is a size-limited map safe for concurrent use: when its entries'
// total weight passes the limit, the least recently used go. Entries used
// within Keep are never evicted, so a working set bigger than the limit
// (every mesh a frame draws) is kept whole rather than churned through
// again and again; the cache then shrinks once the set is no longer in
// use. It has sync.Map's Load and Store, so it can stand in for one.
type Cache struct {
	mu     sync.Mutex
	limit  int
	weight func(any) int
	total  int
	order  *list.List // front: most recently used
	items  map[string]*list.Element
	// Keep is how long a used entry is safe from eviction.
	Keep time.Duration
}

type cacheEntry struct {
	key  string
	val  any
	w    int
	used time.Time
}

// NewCache makes a cache holding up to limit weight; weight nil counts
// each entry as 1.
func NewCache(limit int, weight func(any) int) *Cache {
	if weight == nil {
		weight = func(any) int { return 1 }
	}
	return &Cache{limit: limit, weight: weight, order: list.New(), items: map[string]*list.Element{}, Keep: time.Second}
}

// MeshWeight weighs a *Mesh by its triangle count (at least 1).
func MeshWeight(v any) int {
	if m, ok := v.(*Mesh); ok && m != nil {
		return max(1, len(m.Tris))
	}
	return 1
}

// Load returns the value stored for key, marking it recently used.
func (c *Cache) Load(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(e)
	e.Value.(*cacheEntry).used = time.Now()
	return e.Value.(*cacheEntry).val, true
}

// Store sets the value for key, then evicts the least recently used
// entries until the total is within the limit, keeping those used within
// Keep.
func (c *Cache) Store(key string, val any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	w := c.weight(val)
	now := time.Now()
	if e, ok := c.items[key]; ok {
		ce := e.Value.(*cacheEntry)
		c.total += w - ce.w
		ce.val, ce.w, ce.used = val, w, now
		c.order.MoveToFront(e)
	} else {
		c.items[key] = c.order.PushFront(&cacheEntry{key, val, w, now})
		c.total += w
	}
	for c.total > c.limit && c.order.Len() > 1 {
		e := c.order.Back()
		ce := e.Value.(*cacheEntry)
		if now.Sub(ce.used) < c.Keep {
			break // the oldest is still in use: so is everything newer
		}
		c.order.Remove(e)
		delete(c.items, ce.key)
		c.total -= ce.w
	}
}

// Weight is the entries' total weight (triangles, for a mesh cache).
func (c *Cache) Weight() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.total
}

// Len is the number of entries.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}
