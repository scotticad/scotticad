package app

import (
	"testing"

	"github.com/scotticad/scotticad/pkg/shapes"
)

// Every search keyword belongs to a shape that exists, every shape a user
// can place is in a library besides All Shapes, and no library lists a
// tile twice.
func TestLibrariesAndKeywords(t *testing.T) {
	for kind := range shapeKeywords {
		if _, ok := shapes.Lookup(kind); !ok {
			t.Errorf("keywords for %q, which isn't a shape", kind)
		}
	}
	in := map[string]bool{}
	for _, lib := range libraries {
		seen := map[tile]bool{}
		for _, tl := range tilesFor(lib) {
			if _, ok := shapes.Lookup(tl.kind); !ok {
				t.Errorf("%s: %q isn't a shape", lib, tl.kind)
			}
			if seen[tl] {
				t.Errorf("%s lists %v twice", lib, tl)
			}
			seen[tl] = true
			if lib != "All Shapes" {
				in[tl.kind] = true
			}
		}
	}
	for _, k := range shapes.All() {
		if !k.Hidden && !in[k.ID] {
			t.Errorf("%s is only in All Shapes", k.ID)
		}
	}
}

// A search matches a shape's name or its keywords, every word of it, and
// "hole" or "solid" picks the tile type.
func TestSearchTiles(t *testing.T) {
	has := func(ts []tile, kind string, hole bool) bool {
		for _, tl := range ts {
			if tl.kind == kind && tl.hole == hole {
				return true
			}
		}
		return false
	}
	for _, c := range []struct {
		q    string
		kind string
		hole bool
	}{
		{"frustum", "taperedbox", false},
		{"trapezium", "trapezoid", false},
		{"impeller", "tubejoint", false},
		{"fluted", "flutedrod", false},
		{"hexagons", "honeycomb", false},
		{"lattice", "diamondgrid", false},
		{"lattice", "diamondmesh", false},
		{"arch", "roundroof", false},
		{"vent grille", "honeycomb", false},
		{"cyl hole", "cylinder", true},
	} {
		if !has(searchTiles(c.q), c.kind, c.hole) {
			t.Errorf("%q doesn't find %s (hole %v)", c.q, c.kind, c.hole)
		}
	}
	if has(searchTiles("cyl solid"), "cylinder", true) || !has(searchTiles("cyl solid"), "cylinder", false) {
		t.Error(`"solid" doesn't pick the solid cylinder alone`)
	}
	if ts := searchTiles("frustum impeller"); len(ts) != 0 {
		t.Errorf("words of two shapes match %v: every word must match", ts)
	}
}

// get_ui_state names every tool mode and popup: a constant added without
// a name would make it index past the end.
func TestUIStateNames(t *testing.T) {
	if len(modeNames) != int(modeLayFlat)+1 {
		t.Errorf("%d mode names for %d modes", len(modeNames), modeLayFlat+1)
	}
	if len(popupNames) != int(popStart)+1 {
		t.Errorf("%d popup names for %d popups", len(popupNames), popStart+1)
	}
	if modeNames[modeNotes] != "notes" || modeNames[modeLayFlat] != "layflat" || popupNames[popLibrary] != "library" {
		t.Error("the names are out of step with the constants")
	}
}
