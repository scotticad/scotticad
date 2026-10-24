//go:build linux

package gl

import "testing"

// GL_VERSION strings as drivers write them: the number comes first, and
// the minor may be followed by a patch level or a space.
func TestAtLeast33(t *testing.T) {
	for v, want := range map[string]bool{
		"4.5 (Compatibility Profile) Mesa 25.2.0": true,
		"3.3.0 NVIDIA 550.54.14":                  true,
		"3.3 Mesa 24.0.9":                         true,
		"10.0":                                    true,
		"3.1 Mesa 24.0.9":                         false, // a Raspberry Pi
		"2.1 Mesa 20.3.5":                         false,
		"OpenGL ES 3.2 Mesa 24.0.9":               false, // not desktop GL
		"":                                        false,
		"nonsense":                                false,
	} {
		if got := atLeast33(v); got != want {
			t.Errorf("atLeast33(%q) = %v, want %v", v, got, want)
		}
	}
}
