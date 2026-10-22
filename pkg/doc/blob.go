package doc

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"unsafe"
)

// Shared snapshots for undo history. Imported meshes are stored in the
// design as (often megabytes of) encoded data; copying that into every
// undo step would use that much memory per edit. A BlobStore keeps each
// distinct mesh once, and history snapshots refer to it by hash.

const (
	blobMin    = 4096 // smaller data stays inline
	blobPrefix = "blob:"
)

// BlobStore holds mesh data referred to by shared snapshots.
type BlobStore struct {
	mu     sync.Mutex
	byHash map[string]string
	// strings are immutable, so a string's data pointer and length
	// identify it: this saves re-hashing the same mesh on every snapshot.
	// Only the stored copies (byHash's values) are looked up this way:
	// byHash keeps them alive, so their addresses can't be reused by a
	// string with other contents, and no other string is kept alive.
	byPtr map[[2]uintptr]string
}

func strKey(s string) [2]uintptr {
	return [2]uintptr{uintptr(unsafe.Pointer(unsafe.StringData(s))), uintptr(len(s))}
}

func (s *BlobStore) put(data string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byHash == nil {
		s.byHash, s.byPtr = map[string]string{}, map[[2]uintptr]string{}
	}
	if h, ok := s.byPtr[strKey(data)]; ok {
		return h
	}
	// a string not stored here (a new import, a paste, an opened file):
	// hashed each time until undo or redo swaps in the stored copy
	sum := sha256.Sum256([]byte(data))
	h := hex.EncodeToString(sum[:])
	if _, ok := s.byHash[h]; !ok {
		s.byHash[h] = data
		s.byPtr[strKey(data)] = h
	}
	return h
}

// Intern gives the nodes' large mesh data the stored copies' strings (storing
// any new ones), so later snapshots find them by address instead of hashing
// them again: for meshes that arrive as new strings (an opened file, a
// paste) with the same contents as one already stored.
func (s *BlobStore) Intern(ns []*Node) {
	for _, n := range ns {
		n.Walk(func(x *Node) {
			if len(x.MeshData) >= blobMin {
				h := s.put(x.MeshData)
				if d, ok := s.get(h); ok {
					x.MeshData = d
				}
			}
		})
	}
}

// Reset forgets every stored mesh (when the history that refers to them
// is cleared).
func (s *BlobStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byHash, s.byPtr = nil, nil
}

func (s *BlobStore) get(h string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.byHash[h]
	return d, ok
}

// Len is the number of distinct meshes stored.
func (s *BlobStore) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.byHash)
}

// MarshalShared is Marshal with large mesh data kept in s and referred to
// by hash. Use UnmarshalShared with the same store to read it back.
func (d *Document) MarshalShared(s *BlobStore) ([]byte, error) {
	c := *d
	c.Nodes = make([]*Node, len(d.Nodes))
	for i, n := range d.Nodes {
		cl := n.Clone() // shares the mesh strings, it doesn't copy them
		cl.Walk(func(x *Node) {
			if len(x.MeshData) >= blobMin {
				x.MeshData = blobPrefix + s.put(x.MeshData)
			}
		})
		c.Nodes[i] = cl
	}
	return c.Marshal()
}

// UnmarshalShared reads a MarshalShared snapshot.
func UnmarshalShared(b []byte, s *BlobStore) (*Document, error) {
	d, err := Unmarshal(b)
	if err != nil {
		return nil, err
	}
	for _, n := range d.Nodes {
		n.Walk(func(x *Node) {
			if h, ok := strings.CutPrefix(x.MeshData, blobPrefix); ok {
				if data, ok := s.get(h); ok {
					x.MeshData = data
				}
			}
		})
	}
	return d, nil
}
