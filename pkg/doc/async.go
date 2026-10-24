package doc

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/scotticad/scotticad/pkg/geom"
)

// Background evaluation. Booleans of a big group can take seconds; in async
// mode (the editor window turns it on) a group whose result isn't ready is
// shown as a preview — its solid parts merged, without the booleans — while
// the real result is computed on another goroutine from a snapshot of the
// group. When it finishes, OnReady is called (from that goroutine) so the
// window can redraw. Exports, the automation API and tests evaluate
// synchronously (see Sync), so they always see exact geometry.

var (
	asyncOn   atomic.Bool
	syncDepth atomic.Int32
	pending   sync.Map // union cache key -> struct{}
	pendingN  atomic.Int32

	// OnReady is called, from a background goroutine, whenever a
	// background evaluation finishes.
	OnReady func()
)

// SetAsync turns background evaluation on or off.
func SetAsync(on bool) { asyncOn.Store(on) }

// Sync runs f with geometry evaluated synchronously (exact results).
func Sync(f func()) {
	syncDepth.Add(1)
	defer syncDepth.Add(-1)
	f()
}

// Pending is the number of evaluations running in the background.
func Pending() int { return int(pendingN.Load()) }

// WaitIdle blocks until no background evaluation is running.
func WaitIdle() {
	for pendingN.Load() > 0 {
		time.Sleep(2 * time.Millisecond)
	}
}

func async() bool { return asyncOn.Load() && syncDepth.Load() == 0 }

// unionMeshFor returns a group's evaluated mesh; in async mode, when it
// isn't ready yet, it returns a preview and starts the evaluation. exact
// reports which.
func (n *Node) unionMeshFor() (m *geom.Mesh, exact bool) {
	if !async() {
		return n.UnionMesh(), true
	}
	k := "u:" + n.key()
	if m, ok := meshCache.Load(k); ok {
		return m.(*geom.Mesh), true
	}
	if _, ok := meshCache.Load("ub:" + n.shapeKey()); ok {
		return n.UnionMesh(), true // only its colour or hole flag changed: quick
	}
	latest.Store(n.ID, k)
	if _, running := pending.LoadOrStore(k, struct{}{}); !running {
		pendingN.Add(1)
		snap := n.Clone() // the document may change while it is evaluated
		id := n.ID
		go func() {
			defer func() {
				if r := recover(); r != nil {
					// the preview stays on screen; say why
					fmt.Fprintf(os.Stderr, "scotticad: combining shapes failed: %v\n", r)
				}
				pending.Delete(k)
				pendingN.Add(-1)
				if f := OnReady; f != nil {
					f()
				}
			}()
			// a few at a time; a job whose group has changed again while
			// it waited (a slider dragged through many values) is dropped,
			// its result would never show
			workers <- struct{}{}
			defer func() { <-workers }()
			if want, _ := latest.Load(id); want != k {
				return
			}
			snap.UnionMesh()
		}()
	}
	return n.previewMesh(), false
}

var (
	// latest is each group's current cache key: the one still wanted
	latest sync.Map // node id -> key
	// workers bounds how many booleans run at once (each parallelises its
	// own parts); the rest wait, and stale ones are dropped
	workers = make(chan struct{}, 2)
)

// previewMesh is a group's solid parts merged as they are, without the
// booleans (holes are left out): what shows while the real result is
// computed. Nested groups that are already evaluated are used as they are.
func (n *Node) previewMesh() *geom.Mesh {
	k := "p:" + n.key()
	if m, ok := meshCache.Load(k); ok {
		return m.(*geom.Mesh)
	}
	var ms []*geom.Mesh
	var rec func(x *Node, xf geom.Mat4)
	rec = func(x *Node, xf geom.Mat4) {
		for _, c := range x.Children {
			if c.Hole {
				continue
			}
			w := xf.Mul(c.M)
			if !c.IsGroup() {
				ms = append(ms, c.LeafMesh().Transform(w))
				continue
			}
			if m, ok := meshCache.Load("u:" + c.key()); ok && !c.IsBundle() {
				ms = append(ms, m.(*geom.Mesh).Transform(w))
				continue
			}
			rec(c, w)
		}
	}
	rec(n, geom.Identity())
	m := geom.Merge(ms...)
	if !n.Multicolor {
		m = m.Colored(geom.ParseColor(n.Color))
	}
	meshCache.Store(k, m)
	return m
}
