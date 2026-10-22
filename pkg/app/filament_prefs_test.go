//go:build windows

package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scotticad/scotticad/pkg/filament"
)

// Filament settings saved and read back are the same; a missing file, a
// damaged one or one from an older version (settings left out) gives the
// defaults where it has nothing; out-of-range values come back in range;
// the folder is made if need be, and no temporary file is left behind.
func TestFilamentPrefs(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ScottiCAD", "filament.json") // the folder doesn't exist yet
	if got := readFilamentPrefs(p); got != filament.Defaults() {
		t.Fatalf("missing file: %+v", got)
	}
	s := filament.Defaults()
	s.Material, s.Infill, s.PricePerKg, s.Walls = "PETG", 35, 24.5, 4
	if err := writeFilamentPrefs(p, s); err != nil {
		t.Fatal(err)
	}
	if got := readFilamentPrefs(p); got != s.Normalized() {
		t.Fatalf("read back %+v, saved %+v", got, s)
	}
	if _, err := os.Stat(p + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary file left behind: %v", err)
	}
	// an older file without some settings: those are the defaults
	os.WriteFile(p, []byte(`{"material": "ABS", "infill": 20}`), 0o644)
	got := readFilamentPrefs(p)
	want := filament.Defaults()
	want.Material, want.Infill = "ABS", 20
	if got != want.Normalized() {
		t.Fatalf("partial file: %+v, want %+v", got, want.Normalized())
	}
	// damaged: the defaults, not a half-read mix
	os.WriteFile(p, []byte(`{"material": "ABS", "infill": `), 0o644)
	if got := readFilamentPrefs(p); got != filament.Defaults() {
		t.Fatalf("damaged file: %+v", got)
	}
	// out of range: brought into range
	os.WriteFile(p, []byte(`{"infill": 250, "walls": -3, "price_per_kg": -5}`), 0o644)
	if got := readFilamentPrefs(p); got.Infill != 100 || got.Walls != 0 || got.PricePerKg != 0 {
		t.Fatalf("out of range: %+v", got)
	}
	// a folder where the file should be: an error, not a crash
	bad := filepath.Join(dir, "blocked")
	os.MkdirAll(filepath.Join(bad, "filament.json"), 0o755)
	if err := writeFilamentPrefs(filepath.Join(bad, "filament.json"), s); err == nil {
		t.Fatal("saved over a folder")
	}
}
