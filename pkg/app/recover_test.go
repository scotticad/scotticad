//go:build windows

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

// The recovery file a crash writes opens as the design it was; a name
// with characters a file name can't have still gives a file.
func TestWriteRecovery(t *testing.T) {
	d := doc.New(`Bracket: v2 / "final"?`)
	if _, err := d.AddShape("box", geom.V(5, 0, 0), false); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "not yet made")
	now := time.Date(2026, 10, 20, 9, 30, 15, 0, time.Local)
	p, err := writeRecovery(dir, d, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filepath.Base(p), "recovered 2026-10-20 09-30-15") || filepath.Ext(p) != ".scotticad" {
		t.Fatalf("named %q", p)
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
