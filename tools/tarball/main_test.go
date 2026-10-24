package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var packTime = time.Unix(1767225600, 0).UTC()

// two files, given out of order: the executable one named in -exec
func sample(t *testing.T) []entry {
	t.Helper()
	dir := t.TempDir()
	files := []string{filepath.Join(dir, "run.sh"), filepath.Join(dir, "b.txt")}
	for i, data := range []string{"#!/bin/sh\n", "bee"} {
		if err := os.WriteFile(files[i], []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := read(files, []string{"run.sh"})
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

// The same files give the same bytes every time: entries sorted by name,
// one fixed time, root as the owner, and the executable bit only where
// asked.
func TestTarballIsReproducible(t *testing.T) {
	entries := sample(t)
	out := filepath.Join(t.TempDir(), "out.tar.gz")
	pack := func() []byte {
		if err := writeTar(out, "pkg", packTime, entries); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(out)
		return b
	}
	a := pack()
	time.Sleep(10 * time.Millisecond)
	if !bytes.Equal(a, pack()) {
		t.Fatal("packing twice gave different bytes")
	}
	gz, err := gzip.NewReader(bytes.NewReader(a))
	if err != nil {
		t.Fatal(err)
	}
	if !gz.ModTime.IsZero() || gz.Name != "" {
		t.Errorf("gzip header carries a time or name: %v %q", gz.ModTime, gz.Name)
	}
	var got []string
	for tr := tar.NewReader(gz); ; {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if !h.ModTime.Equal(packTime) || h.Uname != "root" || h.Gname != "root" {
			t.Errorf("%s: time %v, owner %s:%s", h.Name, h.ModTime, h.Uname, h.Gname)
		}
		got = append(got, h.Name)
		switch h.Name {
		case "pkg/":
			if h.Typeflag != tar.TypeDir || h.Mode != 0o755 {
				t.Errorf("folder: type %c mode %o", h.Typeflag, h.Mode)
			}
		case "pkg/run.sh":
			if h.Mode != 0o755 {
				t.Errorf("run.sh mode %o, want 755", h.Mode)
			}
		case "pkg/b.txt":
			if h.Mode != 0o644 {
				t.Errorf("b.txt mode %o, want 644", h.Mode)
			}
			data, _ := io.ReadAll(tr)
			if string(data) != "bee" {
				t.Errorf("b.txt holds %q", data)
			}
		}
	}
	if want := []string{"pkg/", "pkg/b.txt", "pkg/run.sh"}; !equal(got, want) {
		t.Errorf("entries %q, want %q", got, want)
	}
}

// The zip likewise, with the files at the top when no folder is given.
func TestZipIsReproducible(t *testing.T) {
	entries := sample(t)
	out := filepath.Join(t.TempDir(), "out.zip")
	pack := func() []byte {
		if err := writeZip(out, "", packTime, entries); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(out)
		return b
	}
	a := pack()
	time.Sleep(10 * time.Millisecond)
	if !bytes.Equal(a, pack()) {
		t.Fatal("packing twice gave different bytes")
	}
	zr, err := zip.NewReader(bytes.NewReader(a), int64(len(a)))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range zr.File {
		got = append(got, f.Name)
		if !f.Modified.Equal(packTime) {
			t.Errorf("%s: time %v", f.Name, f.Modified)
		}
		want := os.FileMode(0o644)
		if f.Name == "run.sh" {
			want = 0o755
		}
		if m := f.Mode().Perm(); m != want {
			t.Errorf("%s: mode %o, want %o", f.Name, m, want)
		}
	}
	if want := []string{"b.txt", "run.sh"}; !equal(got, want) {
		t.Errorf("entries %q, want %q", got, want)
	}
	// with a folder, the entries sit in it
	if err := writeZip(out, "pkg", packTime, entries); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	zr, _ = zip.NewReader(bytes.NewReader(b), int64(len(b)))
	got = got[:0]
	for _, f := range zr.File {
		got = append(got, f.Name)
	}
	if want := []string{"pkg/", "pkg/b.txt", "pkg/run.sh"}; !equal(got, want) {
		t.Errorf("entries in a folder %q, want %q", got, want)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
