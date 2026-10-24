// Package sysfont finds font files installed on the system: in the Windows
// Fonts folder, or in the usual Linux font folders (searched recursively,
// as Linux keeps fonts in subfolders).
package sysfont

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// Dirs are the folders searched, in order.
func Dirs() []string {
	if runtime.GOOS == "windows" {
		w := os.Getenv("WINDIR")
		if w == "" {
			w = `C:\Windows`
		}
		return []string{filepath.Join(w, "Fonts")}
	}
	var dirs []string
	home, _ := os.UserHomeDir()
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		dirs = append(dirs, filepath.Join(d, "fonts"))
	} else if home != "" {
		dirs = append(dirs, filepath.Join(home, ".local", "share", "fonts"))
	}
	if home != "" {
		dirs = append(dirs, filepath.Join(home, ".fonts"))
	}
	data := os.Getenv("XDG_DATA_DIRS")
	if data == "" {
		data = "/usr/local/share:/usr/share"
	}
	for _, d := range strings.Split(data, ":") {
		if d != "" {
			dirs = append(dirs, filepath.Join(d, "fonts"))
		}
	}
	// the system folders even when XDG_DATA_DIRS leaves them out
	dirs = append(dirs, "/usr/local/share/fonts", "/usr/share/fonts")
	seen := map[string]bool{}
	var out []string
	for _, d := range dirs {
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}

var (
	once  sync.Once
	paths map[string]string // lower-case file name -> path (the first found)
	names []string          // every font file's name, sorted
)

func index() {
	paths, names = map[string]string{}, nil
	for _, d := range Dirs() {
		filepath.WalkDir(d, func(p string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() {
				return nil
			}
			n := strings.ToLower(e.Name())
			if !strings.HasSuffix(n, ".ttf") && !strings.HasSuffix(n, ".otf") && !strings.HasSuffix(n, ".ttc") {
				return nil
			}
			if _, dup := paths[n]; !dup {
				paths[n] = p
				names = append(names, e.Name())
			}
			return nil
		})
	}
	sort.Strings(names)
}

// Find returns the path of the first of the font files named (by file name,
// ignoring case) that is installed.
func Find(names ...string) (string, bool) {
	once.Do(index)
	for _, n := range names {
		if p, ok := paths[strings.ToLower(filepath.Base(n))]; ok {
			return p, true
		}
	}
	return "", false
}

// List is every installed font file's name, sorted.
func List() []string {
	once.Do(index)
	return append([]string(nil), names...)
}
