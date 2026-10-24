// Command tarball writes a reproducible release package: a .tar.gz with the
// files in a single top folder, owned by root, or a .zip (when -o ends in
// .zip) with the files in the given folder or at the top. Entries are sorted
// and share a fixed modification time, and there are no other time stamps,
// so the same files always give the same bytes.
//
//	go run ./tools/tarball -o out.tar.gz -folder name -mtime 1767225600 -exec scotticad,install.sh file...
//	go run ./tools/tarball -o out.zip -mtime 1767225600 file...
//
// Each file is stored by its base name; the ones named in -exec are
// executable (0755), the rest 0644.
package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type entry struct {
	name string // base name
	mode os.FileMode
	data []byte
}

func main() {
	out := flag.String("o", "", "the .tar.gz or .zip to write")
	folder := flag.String("folder", "", "the top folder inside the archive (optional for a .zip)")
	mtime := flag.Int64("mtime", 1767225600, "modification time of every entry (Unix seconds)")
	execs := flag.String("exec", "", "comma-separated base names to make executable")
	flag.Parse()
	isZip := strings.HasSuffix(strings.ToLower(*out), ".zip")
	if *out == "" || (*folder == "" && !isZip) || flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: tarball -o out.tar.gz -folder name [-mtime N] [-exec a,b] file...")
		fmt.Fprintln(os.Stderr, "       tarball -o out.zip [-folder name] [-mtime N] [-exec a,b] file...")
		os.Exit(2)
	}
	entries, err := read(flag.Args(), strings.Split(*execs, ","))
	if err == nil {
		mt := time.Unix(*mtime, 0).UTC()
		if isZip {
			err = writeZip(*out, *folder, mt, entries)
		} else {
			err = writeTar(*out, *folder, mt, entries)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "tarball:", err)
		os.Exit(1)
	}
}

func read(files, execs []string) ([]entry, error) {
	isExec := map[string]bool{}
	for _, e := range execs {
		isExec[strings.TrimSpace(e)] = true
	}
	var entries []entry
	for _, p := range files {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		e := entry{name: filepath.Base(p), mode: 0o644, data: data}
		if isExec[e.name] {
			e.mode = 0o755
		}
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	return entries, nil
}

func writeTar(out, folder string, mt time.Time, entries []entry) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	gz, _ := gzip.NewWriterLevel(f, gzip.BestCompression) // no name or time in the header
	tw := tar.NewWriter(gz)
	hdr := func(name string, mode int64, typ byte, size int64) *tar.Header {
		return &tar.Header{Name: name, Mode: mode, Typeflag: typ, Size: size, ModTime: mt,
			Uname: "root", Gname: "root", Format: tar.FormatPAX}
	}
	if err := tw.WriteHeader(hdr(folder+"/", 0o755, tar.TypeDir, 0)); err != nil {
		return err
	}
	for _, e := range entries {
		if err := tw.WriteHeader(hdr(folder+"/"+e.name, int64(e.mode), tar.TypeReg, int64(len(e.data)))); err != nil {
			return err
		}
		if _, err := tw.Write(e.data); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	return f.Close()
}

func writeZip(out, folder string, mt time.Time, entries []entry) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	prefix := ""
	if folder != "" {
		prefix = folder + "/"
		h := &zip.FileHeader{Name: prefix, Modified: mt}
		h.SetMode(os.ModeDir | 0o755)
		if _, err := zw.CreateHeader(h); err != nil {
			return err
		}
	}
	for _, e := range entries {
		h := &zip.FileHeader{Name: prefix + e.name, Method: zip.Deflate, Modified: mt}
		h.SetMode(e.mode)
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		if _, err := w.Write(e.data); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return f.Close()
}
