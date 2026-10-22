package geom

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestCacheEvictsLeastRecentlyUsed(t *testing.T) {
	c := NewCache(3, nil)
	c.Keep = 0
	c.Store("a", 1)
	c.Store("b", 2)
	c.Store("c", 3)
	c.Load("a") // a is now the most recent; b the least
	c.Store("d", 4)
	if _, ok := c.Load("b"); ok {
		t.Fatal("b should have gone")
	}
	for _, k := range []string{"a", "c", "d"} {
		if _, ok := c.Load(k); !ok {
			t.Fatalf("%s went", k)
		}
	}
	// replacing a value doesn't grow the cache
	c.Store("d", 5)
	if v, _ := c.Load("d"); c.Len() != 3 || v != 5 {
		t.Fatalf("len %d, d=%v", c.Len(), v)
	}
}

func TestCacheWeighsMeshes(t *testing.T) {
	c := NewCache(20, MeshWeight)
	c.Keep = 0
	big := &Mesh{Tris: make([]Tri, 12)}
	c.Store("x", big)
	c.Store("y", big) // 24 > 20: x goes
	if _, ok := c.Load("x"); ok || c.Len() != 1 {
		t.Fatalf("len %d", c.Len())
	}
	// an entry over the limit on its own still stays
	c.Store("huge", &Mesh{Tris: make([]Tri, 50)})
	if _, ok := c.Load("huge"); !ok || c.Len() != 1 {
		t.Fatalf("len %d", c.Len())
	}
}

// A working set bigger than the limit is kept whole while it is in use
// (otherwise every frame would evict what the next load needs), and
// trimmed once it isn't.
func TestCacheKeepsWhatIsInUse(t *testing.T) {
	c := NewCache(2, nil)
	c.Keep = 50 * time.Millisecond
	for _, k := range []string{"a", "b", "c", "d"} {
		c.Store(k, k)
	}
	if c.Len() != 4 {
		t.Fatalf("len %d: evicted something in use", c.Len())
	}
	time.Sleep(80 * time.Millisecond)
	c.Store("e", "e")
	if c.Len() != 2 { // the limit: the newest two
		t.Fatalf("len %d: the stale entries should have gone", c.Len())
	}
	for _, k := range []string{"d", "e"} {
		if _, ok := c.Load(k); !ok {
			t.Fatalf("%s went", k)
		}
	}
}

func TestCacheConcurrent(t *testing.T) {
	c := NewCache(50, nil)
	c.Keep = 0
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				k := fmt.Sprint(i % 80)
				c.Store(k, i)
				c.Load(k)
			}
		}(g)
	}
	wg.Wait()
	if c.Len() > 50 {
		t.Fatalf("len %d", c.Len())
	}
}
