//go:build windows

package app

import (
	"testing"

	"github.com/scotticad/scotticad/pkg/doc"
)

// Closing the Settings dialog applies only what was changed in it: a
// change made meanwhile elsewhere (the snap menu, a script) stays.
func TestMergeSettings(t *testing.T) {
	base := doc.DefaultSettings()
	draft := base
	draft.Units, draft.ShowGrid = "in", false // changed in the dialog
	cur := base
	cur.Snap, cur.WorkplaneW = 5, 300 // changed elsewhere while it was open
	got := mergeSettings(base, draft, cur)
	want := cur
	want.Units, want.ShowGrid = "in", false
	if got != want {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	// nothing changed in the dialog: the current settings as they are
	if got := mergeSettings(base, base, cur); got != cur {
		t.Fatalf("an untouched dialog changed %+v to %+v", cur, got)
	}
	// every field the dialog can change is carried over
	all := doc.Settings{Units: "in", WorkplaneW: 1, WorkplaneL: 2, Snap: 3, ShowGrid: !base.ShowGrid, ShowShadows: !base.ShowShadows}
	if got := mergeSettings(base, all, base); got != all {
		t.Fatalf("a field was dropped: %+v, want %+v", got, all)
	}
}
