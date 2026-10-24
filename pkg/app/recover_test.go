package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/geom"
)

// The recovery file a crash writes is named after the design's file and
// opens as the design it was; a name with characters a file name can't
// have still gives a file, and an unsaved design is "design".
func TestWriteRecovery(t *testing.T) {
	d := doc.New()
	if _, err := d.AddShape("box", geom.V(5, 0, 0), false); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "not yet made")
	now := time.Date(2026, 10, 20, 9, 30, 15, 0, time.Local)
	p, err := writeRecovery(dir, `Bracket: v2 "final"?`, d, now)
	if err != nil {
		t.Fatal(err)
	}
	if want := `Bracket_ v2 _final__ (recovered 2026-10-20 09-30-15).scotticad`; filepath.Base(p) != want {
		t.Fatalf("named %q, want %q", filepath.Base(p), want)
	}
	if p2, err := writeRecovery(dir, "", d, now); err != nil || !strings.HasPrefix(filepath.Base(p2), "design (recovered ") {
		t.Fatalf("an unsaved design: %q %v", p2, err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	back, err := doc.Unmarshal(b)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := d.Marshal()
	got, _ := back.Marshal()
	if string(got) != string(want) {
		t.Fatal("the recovered design differs from the one saved")
	}
}
