package doc

import (
	"os"
	"testing"
	"time"
)

// Times evaluating every top-level node of a real design (when
// present). Run with -cpuprofile to see where the time goes:
//
//	go test ./pkg/doc -run Perf -v -cpuprofile cpu.out
func TestPerfEvaluate(t *testing.T) {
	path := os.Getenv("SCOTTICAD_PERF_FILE")
	if path == "" {
		t.Skip("set SCOTTICAD_PERF_FILE to a .scotticad design to time it")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip("no design to time")
	}
	d, err := Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("SCOTTICAD_PERF_PREWARM") != "" {
		t0 := time.Now()
		d.Prewarm()
		t.Logf("prewarm (parallel) %v", time.Since(t0).Round(time.Millisecond))
	}
	start := time.Now()
	tris := 0
	for _, n := range d.Nodes {
		t0 := time.Now()
		m := n.WorldMesh(false)
		tris += len(m.Tris)
		t.Logf("%-24s %7d tris  %v", n.Label(), len(m.Tris), time.Since(t0).Round(time.Millisecond))
	}
	t.Logf("total %v, %d triangles", time.Since(start).Round(time.Millisecond), tris)
}
