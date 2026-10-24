// Package scotticad holds project-wide facts, such as the version, that the
// program builds in.
package scotticad

import (
	_ "embed"
	"strings"
)

// The VERSION file at the top of the repository is compiled into the
// program, so a release only needs that file changed.
//
//go:embed VERSION
var version string

// Version is ScottiCAD's version, from the VERSION file (e.g. "0.1.0").
func Version() string { return strings.TrimSpace(version) }
