//go:build windows

package render

import (
	"math"

	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/gl"
)

const meshVS = `#version 330 core
layout(location=0) in vec3 aPos;
layout(location=1) in vec3 aNormal;
layout(location=2) in vec3 aColor;
uniform mat4 uVP;
uniform mat4 uModel;
uniform mat4 uNormal;
out vec3 vN;
out vec3 vC;
out vec3 vW;
void main() {
  vec4 w = uModel * vec4(aPos, 1.0);
  vW = w.xyz;
  vN = mat3(uNormal) * aNormal;
  vC = aColor;
  gl_Position = uVP * w;
}`

const meshFS = `#version 330 core
in vec3 vN;
in vec3 vC;
in vec3 vW;
uniform vec3 uEye;
uniform vec4 uTint;      // rgb, amount
uniform float uAlpha;
uniform float uStripes;  // 1 = hole stripes
uniform float uFlat;     // 1 = output colour unlit (masks)
uniform float uPx;       // device pixels per logical pixel
uniform vec4 uClip;      // xyz != 0: keep only where dot(xyz, p) + w >= 0
uniform float uCap;      // 1 = inside faces seen through the clip are drawn as the cut face
out vec4 o;
void main() {
  if (dot(uClip.xyz, uClip.xyz) > 0.0 && dot(uClip.xyz, vW) + uClip.w < 0.0) discard;
  vec3 c = mix(vC, uTint.rgb, uTint.a);
  if (uFlat > 0.5) { o = vec4(c, 1.0); return; }
  vec3 n = normalize(vN);
  vec3 v = normalize(uEye - vW);
  bool back = dot(n, v) < 0.0;
  if (back) n = -n;
  vec3 l1 = normalize(vec3(-0.35, -0.55, 1.0));
  vec3 l2 = normalize(v + vec3(0.0, 0.0, 0.4));
  if (uCap > 0.5 && back) {
    // the cut face: looking into a clipped solid, its inside reads as
    // solid material, a flat darker shade with diagonal hatching
    c *= 0.62;
    if (mod(gl_FragCoord.x + gl_FragCoord.y, 7.0 * uPx) < 1.2 * uPx) c *= 0.72;
    o = vec4(c, uAlpha);
    return;
  }
  if (uStripes > 0.5) {
    // holes: light translucent grey, soft shading, inner (back) faces
    // darker, fine near-horizontal hairline stripes
    float d = 0.86 + 0.08 * max(dot(n, l1), 0.0) + 0.06 * max(dot(n, l2), 0.0);
    if (back) d *= 0.80;
    c *= d;
    float s = mod(gl_FragCoord.y - gl_FragCoord.x * 0.18, 4.0 * uPx);
    if (s < uPx) c *= 0.83;
    o = vec4(min(c, vec3(1.0)), uAlpha);
    return;
  }
  float d = 0.50 + 0.30 * max(dot(n, l1), 0.0) + 0.28 * max(dot(n, l2), 0.0);
  c = c * d;
  o = vec4(min(c, vec3(1.0)), uAlpha);
}`

const lineVS = `#version 330 core
layout(location=0) in vec3 aPos;
layout(location=1) in vec4 aColor;
uniform mat4 uVP;
uniform mat4 uModel;
out vec4 vC;
out vec3 vW;
void main() {
  vC = aColor;
  vec4 w = uModel * vec4(aPos, 1.0);
  vW = w.xyz;
  gl_Position = uVP * w;
}`

const lineFS = `#version 330 core
in vec4 vC;
in vec3 vW;
uniform float uAlpha;
uniform vec4 uOver; // a > 0: draw every line in this colour
uniform vec4 uClip; // as the mesh shader's: a shape's edges are cut with it
out vec4 o;
void main() {
  if (dot(uClip.xyz, uClip.xyz) > 0.0 && dot(uClip.xyz, vW) + uClip.w < 0.0) discard;
  if (uOver.a > 0.0) { o = vec4(uOver.rgb, uOver.a * uAlpha); return; }
  o = vec4(vC.rgb, vC.a * uAlpha);
}`

// plane shader: workplane fill with soft shadow lookup
const planeVS = `#version 330 core
layout(location=0) in vec3 aPos;
uniform mat4 uVP;
uniform vec4 uRect; // minx, miny, w, h of shadow map in world
out vec2 vUV;
void main() { vUV = (aPos.xy - uRect.xy) / uRect.zw; gl_Position = uVP * vec4(aPos, 1.0); }`

const planeFS = `#version 330 core
in vec2 vUV;
uniform sampler2D uShadow;
uniform vec4 uColor;
uniform float uShadowOn;
uniform vec2 uTexel;
out vec4 o;
void main() {
  float s = 0.0;
  if (uShadowOn > 0.5) {
    float tot = 0.0;
    for (int x = -3; x <= 3; x++) for (int y = -3; y <= 3; y++) {
      float w = 1.0 / (1.0 + float(x*x + y*y));
      s += w * texture(uShadow, vUV + vec2(x, y) * uTexel * 2.0).r;
      tot += w;
    }
    s /= tot;
  }
  o = vec4(uColor.rgb * (1.0 - 0.22 * s), uColor.a);
}`

// outline post-process: draws outlines around mask regions
const quadVS = `#version 330 core
layout(location=0) in vec2 aPos;
out vec2 vUV;
void main() { vUV = aPos * 0.5 + 0.5; gl_Position = vec4(aPos, 0.0, 1.0); }`

// The outline is a solid line uRadius wide around each mask region
// (uRadiusB for the blue channel, the hover); the selection's (red
// channel) also has a soft glow fading out to uGlow, sampled sparsely
// (16 directions, 3 distances) to stay cheap.
const outlineFS = `#version 330 core
in vec2 vUV;
uniform sampler2D uMask;
uniform vec2 uTexel;
uniform float uRadius;
uniform float uRadiusB;
uniform float uGlow;
uniform float uGlowAlpha;
uniform vec3 uColR;
uniform vec3 uColG;
uniform vec3 uColB;
out vec4 o;
void main() {
  vec3 c = texture(uMask, vUV).rgb;
  vec3 m = vec3(0.0);
  int r = int(uRadius), rb = int(uRadiusB);
  for (int x = -r; x <= r; x++) for (int y = -r; y <= r; y++) {
    int d2 = x*x + y*y;
    if (d2 > r*r + 1) continue;
    vec3 s = texture(uMask, vUV + vec2(x, y) * uTexel).rgb;
    if (d2 > rb*rb + 1) s.b = 0.0; // the hover line is thinner
    m = max(m, s);
  }
  vec3 e = m * (1.0 - step(0.5, c)); // near a region, outside it
  if (e.g > 0.5) { o = vec4(uColG, 1.0); return; }
  if (e.r > 0.5) { o = vec4(uColR, 1.0); return; }
  if (e.b > 0.5) { o = vec4(uColB, 1.0); return; }
  if (uGlow <= uRadius || c.r > 0.5) discard; // the glow: outside the selection only
  float best = 1e9;
  for (int i = 0; i < 16; i++) {
    float a = float(i) * 0.39269908;
    vec2 d = vec2(cos(a), sin(a));
    for (int j = 1; j <= 3; j++) {
      float dist = uRadius + (uGlow - uRadius) * float(j) / 3.0;
      if (texture(uMask, vUV + d * dist * uTexel).r > 0.5) best = min(best, dist);
    }
  }
  if (best > uGlow) discard;
  float t = (best - uRadius) / (uGlow - uRadius);
  o = vec4(uColR, uGlowAlpha * (1.0 - t) * (1.0 - t));
}`

type gpuMesh struct {
	vao, vbo   uint32
	n          int32
	evao, evbo uint32
	en         int32
	used       int
	adj        []adjEdge // for silhouettes, built on first use
}

// adjEdge is a mesh edge with the normals of its two faces.
type adjEdge struct {
	a, b   geom.Vec3
	n0, n1 geom.Vec3
}

// Lines is a dynamic line list (world space).
type Lines struct {
	data []float32
}

func (l *Lines) Add(a, b geom.Vec3, c [4]float32) {
	l.data = append(l.data, float32(a.X), float32(a.Y), float32(a.Z), c[0], c[1], c[2], c[3],
		float32(b.X), float32(b.Y), float32(b.Z), c[0], c[1], c[2], c[3])
}

func (l *Lines) Reset() { l.data = l.data[:0] }

// Scene renders 3D content.
type Scene struct {
	meshProg, lineProg, planeProg, outlineProg uint32
	mu                                         map[string]int32
	lu, pu, ou                                 map[string]int32
	cache                                      map[*geom.Mesh]*gpuMesh
	frame                                      int
	lineVAO, lineVBO                           uint32
	quadVAO, quadVBO                           uint32
	planeVAO, planeVBO                         uint32

	// shadow map over the workplane
	shadowFBO, shadowTex uint32
	shadowSize           int32

	// outline mask
	maskFBO, maskTex, maskDepth uint32
	maskW, maskH                int32

	VP  geom.Mat4
	Eye geom.Vec3
	// PixelScale is device pixels per logical pixel (stripe spacing).
	PixelScale float32
}

func uniforms(p uint32, names ...string) map[string]int32 {
	m := map[string]int32{}
	for _, n := range names {
		m[n] = gl.Uniform(p, n)
	}
	return m
}

func NewScene() (*Scene, error) {
	s := &Scene{cache: map[*geom.Mesh]*gpuMesh{}}
	var err error
	if s.meshProg, err = gl.Program(meshVS, meshFS); err != nil {
		return nil, err
	}
	s.mu = uniforms(s.meshProg, "uVP", "uModel", "uNormal", "uEye", "uTint", "uAlpha", "uStripes", "uFlat", "uPx", "uClip", "uCap")
	if s.lineProg, err = gl.Program(lineVS, lineFS); err != nil {
		return nil, err
	}
	s.lu = uniforms(s.lineProg, "uVP", "uModel", "uAlpha", "uOver", "uClip")
	if s.planeProg, err = gl.Program(planeVS, planeFS); err != nil {
		return nil, err
	}
	s.pu = uniforms(s.planeProg, "uVP", "uRect", "uShadow", "uColor", "uShadowOn", "uTexel")
	if s.outlineProg, err = gl.Program(quadVS, outlineFS); err != nil {
		return nil, err
	}
	s.ou = uniforms(s.outlineProg, "uMask", "uTexel", "uRadius", "uRadiusB", "uGlow", "uGlowAlpha", "uColR", "uColG", "uColB")

	s.lineVAO, s.lineVBO = gl.GenVertexArray(), gl.GenBuffer()
	gl.BindVertexArray(s.lineVAO)
	gl.BindBuffer(gl.ARRAY_BUFFER, s.lineVBO)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointer(0, 3, gl.FLOAT, false, 28, 0)
	gl.EnableVertexAttribArray(1)
	gl.VertexAttribPointer(1, 4, gl.FLOAT, false, 28, 12)

	s.quadVAO, s.quadVBO = gl.GenVertexArray(), gl.GenBuffer()
	gl.BindVertexArray(s.quadVAO)
	gl.BindBuffer(gl.ARRAY_BUFFER, s.quadVBO)
	gl.BufferData(gl.ARRAY_BUFFER, []float32{-1, -1, 1, -1, 1, 1, -1, -1, 1, 1, -1, 1}, gl.STATIC_DRAW)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointer(0, 2, gl.FLOAT, false, 8, 0)

	s.planeVAO, s.planeVBO = gl.GenVertexArray(), gl.GenBuffer()
	gl.BindVertexArray(s.planeVAO)
	gl.BindBuffer(gl.ARRAY_BUFFER, s.planeVBO)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointer(0, 3, gl.FLOAT, false, 12, 0)
	gl.BindVertexArray(0)

	s.shadowSize = 1024
	s.shadowTex = gl.GenTexture()
	gl.BindTexture(gl.TEXTURE_2D, s.shadowTex)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, s.shadowSize, s.shadowSize, gl.RGBA, gl.UNSIGNED_BYTE, nil)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	s.shadowFBO = gl.GenFramebuffer()
	gl.BindFramebuffer(gl.FRAMEBUFFER, s.shadowFBO)
	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, s.shadowTex)
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
	return s, nil
}

func (s *Scene) upload(m *geom.Mesh) *gpuMesh {
	// keyed by the mesh itself: the key keeps it alive, so its address
	// can't be reused by another mesh while its upload is here (meshes are
	// never changed once made)
	g, ok := s.cache[m]
	if ok {
		g.used = s.frame
		return g
	}
	data := make([]float32, 0, len(m.Tris)*27)
	for _, t := range m.Tris {
		n := t.Normal()
		for _, p := range t.P {
			data = append(data, float32(p.X), float32(p.Y), float32(p.Z), float32(n.X), float32(n.Y), float32(n.Z), t.C.R, t.C.G, t.C.B)
		}
	}
	g = &gpuMesh{vao: gl.GenVertexArray(), vbo: gl.GenBuffer(), n: int32(len(m.Tris) * 3), used: s.frame}
	gl.BindVertexArray(g.vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, g.vbo)
	gl.BufferData(gl.ARRAY_BUFFER, data, gl.STATIC_DRAW)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointer(0, 3, gl.FLOAT, false, 36, 0)
	gl.EnableVertexAttribArray(1)
	gl.VertexAttribPointer(1, 3, gl.FLOAT, false, 36, 12)
	gl.EnableVertexAttribArray(2)
	gl.VertexAttribPointer(2, 3, gl.FLOAT, false, 36, 24)

	// feature edges (dihedral > 25°), darker than the face colour
	edges := featureEdges(m, 25)
	if len(edges) > 0 {
		g.evao, g.evbo = gl.GenVertexArray(), gl.GenBuffer()
		gl.BindVertexArray(g.evao)
		gl.BindBuffer(gl.ARRAY_BUFFER, g.evbo)
		gl.BufferData(gl.ARRAY_BUFFER, edges, gl.STATIC_DRAW)
		gl.EnableVertexAttribArray(0)
		gl.VertexAttribPointer(0, 3, gl.FLOAT, false, 28, 0)
		gl.EnableVertexAttribArray(1)
		gl.VertexAttribPointer(1, 4, gl.FLOAT, false, 28, 12)
		g.en = int32(len(edges) / 7)
	}
	gl.BindVertexArray(0)
	s.cache[m] = g
	return g
}

// featureEdges returns the creases of a mesh (faces meeting at more than
// deg degrees) as coloured line vertices. Only edges shared by exactly two
// faces count: primitives are closed, so an unshared edge comes from a CSG
// seam (a T-junction, where one long edge meets several short ones) and
// drawing it would scratch hairlines across flat faces. Sliver triangles
// from CSG have meaningless normals and are ignored.
func featureEdges(m *geom.Mesh, deg float64) []float32 {
	type key [6]int64
	q := func(p geom.Vec3) [3]int64 {
		return [3]int64{int64(math.Round(p.X * 1e4)), int64(math.Round(p.Y * 1e4)), int64(math.Round(p.Z * 1e4))}
	}
	type info struct {
		a, b  geom.Vec3
		n     []geom.Vec3 // normals of reliable (non-sliver) faces
		faces int
		c     geom.Color
	}
	edges := map[key]*info{}
	var order []*info
	for _, t := range m.Tris {
		e1, e2, e3 := t.P[1].Sub(t.P[0]), t.P[2].Sub(t.P[0]), t.P[2].Sub(t.P[1])
		cr := e1.Cross(e2)
		long := math.Max(e1.Dot(e1), math.Max(e2.Dot(e2), e3.Dot(e3)))
		reliable := cr.Len() > 1e-5*long
		n := cr.Norm()
		for i := 0; i < 3; i++ {
			a, b := t.P[i], t.P[(i+1)%3]
			qa, qb := q(a), q(b)
			if qa == qb {
				continue
			}
			var k key
			if qa[0] < qb[0] || (qa[0] == qb[0] && (qa[1] < qb[1] || (qa[1] == qb[1] && qa[2] < qb[2]))) {
				k = key{qa[0], qa[1], qa[2], qb[0], qb[1], qb[2]}
			} else {
				k = key{qb[0], qb[1], qb[2], qa[0], qa[1], qa[2]}
			}
			e := edges[k]
			if e == nil {
				e = &info{a: a, b: b, c: t.C}
				edges[k] = e
				order = append(order, e)
			}
			e.faces++
			if reliable {
				e.n = append(e.n, n)
			}
		}
	}
	cosT := math.Cos(deg * math.Pi / 180)
	var out []float32
	for _, e := range order {
		if e.faces != 2 || len(e.n) != 2 || e.n[0].Dot(e.n[1]) >= cosT {
			continue
		}
		c := [4]float32{e.c.R * 0.55, e.c.G * 0.55, e.c.B * 0.55, 0.7}
		out = append(out, float32(e.a.X), float32(e.a.Y), float32(e.a.Z), c[0], c[1], c[2], c[3],
			float32(e.b.X), float32(e.b.Y), float32(e.b.Z), c[0], c[1], c[2], c[3])
	}
	return out
}

// Begin sets up the frame's camera.
func (s *Scene) Begin(vp geom.Mat4, eye geom.Vec3) {
	s.frame++
	s.VP = vp
	s.Eye = eye
}

// Cached is how many meshes are uploaded to the GPU at the moment.
func (s *Scene) Cached() int { return len(s.cache) }

// Evict frees GPU meshes not drawn for a while.
func (s *Scene) Evict() {
	for m, g := range s.cache {
		if s.frame-g.used > 120 {
			s.free(g)
			delete(s.cache, m)
		}
	}
}

// free releases an upload's GPU buffers.
func (s *Scene) free(g *gpuMesh) {
	gl.DeleteBuffer(g.vbo)
	gl.DeleteVertexArray(g.vao)
	if g.evbo != 0 {
		gl.DeleteBuffer(g.evbo)
		gl.DeleteVertexArray(g.evao)
	}
}

// Style controls how a mesh is drawn.
type Style struct {
	Model   geom.Mat4
	Tint    [4]float32 // rgb + amount
	Alpha   float32
	Stripes bool
	Flat    bool
	Edges   bool
	// Outline, when its alpha is > 0, draws feature edges and the
	// view-dependent silhouette in this colour (hole style).
	Outline [4]float32
	// Clip, when its normal (xyz) is non-zero, hides the part of the mesh
	// where dot(normal, p) + w < 0 (a world-space plane).
	Clip [4]float32
	// Cap draws the inside of a clipped closed mesh as its cut face.
	Cap bool
}

// DrawMesh draws a mesh (in world space unless Model is set).
func (s *Scene) DrawMesh(m *geom.Mesh, st Style) {
	if m == nil || len(m.Tris) == 0 {
		return
	}
	if st.Model == (geom.Mat4{}) {
		st.Model = geom.Identity()
	}
	if st.Alpha == 0 {
		st.Alpha = 1
	}
	g := s.upload(m)
	gl.UseProgram(s.meshProg)
	gl.UniformMatrix4(s.mu["uVP"], s.VP.Float32())
	gl.UniformMatrix4(s.mu["uModel"], st.Model.Float32())
	gl.UniformMatrix4(s.mu["uNormal"], st.Model.NormalMatrix().Float32())
	gl.Uniform3f(s.mu["uEye"], float32(s.Eye.X), float32(s.Eye.Y), float32(s.Eye.Z))
	gl.Uniform4f(s.mu["uTint"], st.Tint[0], st.Tint[1], st.Tint[2], st.Tint[3])
	gl.Uniform1f(s.mu["uAlpha"], st.Alpha)
	gl.Uniform1f(s.mu["uStripes"], bf(st.Stripes))
	gl.Uniform1f(s.mu["uFlat"], bf(st.Flat))
	gl.Uniform4f(s.mu["uClip"], st.Clip[0], st.Clip[1], st.Clip[2], st.Clip[3])
	gl.Uniform1f(s.mu["uCap"], bf(st.Cap))
	px := s.PixelScale
	if px <= 0 {
		px = 1
	}
	gl.Uniform1f(s.mu["uPx"], px)
	gl.BindVertexArray(g.vao)
	gl.DrawArrays(gl.TRIANGLES, 0, g.n)
	outline := st.Outline[3] > 0 && !st.Flat
	if (st.Edges || outline) && g.en > 0 && !st.Flat {
		gl.UseProgram(s.lineProg)
		gl.UniformMatrix4(s.lu["uVP"], s.VP.Float32())
		gl.UniformMatrix4(s.lu["uModel"], st.Model.Float32())
		gl.Uniform4f(s.lu["uClip"], st.Clip[0], st.Clip[1], st.Clip[2], st.Clip[3])
		gl.Uniform1f(s.lu["uAlpha"], 1)
		if outline {
			gl.Uniform4f(s.lu["uOver"], st.Outline[0], st.Outline[1], st.Outline[2], st.Outline[3])
		} else {
			gl.Uniform1f(s.lu["uAlpha"], st.Alpha)
			gl.Uniform4f(s.lu["uOver"], 0, 0, 0, 0)
		}
		gl.BindVertexArray(g.evao)
		gl.DrawArrays(gl.LINES, 0, g.en*2)
	}
	if outline {
		s.drawSilhouette(m, g, st)
	}
	gl.BindVertexArray(0)
}

// drawSilhouette draws edges between a front- and a back-facing triangle
// as seen from the eye.
func (s *Scene) drawSilhouette(m *geom.Mesh, g *gpuMesh, st Style) {
	if g.adj == nil {
		g.adj = adjacency(m)
	}
	inv, ok := st.Model.Inverse()
	if !ok {
		return
	}
	eye := inv.Point(s.Eye)
	ortho := s.VP[15] == 1 && s.VP[3] == 0 && s.VP[7] == 0 && s.VP[11] == 0
	var dir geom.Vec3
	if ortho {
		// view direction for orthographic projections (third row of VP)
		inv2, _ := s.VP.Inverse()
		p0, _ := inv2.Project(geom.V(0, 0, -1))
		p1, _ := inv2.Project(geom.V(0, 0, 1))
		dir = inv.Dir(p1.Sub(p0))
	}
	var data []float32
	c := st.Outline
	for _, e := range g.adj {
		var v geom.Vec3
		if ortho {
			v = dir.Neg()
		} else {
			v = eye.Sub(e.a)
		}
		if (e.n0.Dot(v) > 0) == (e.n1.Dot(v) > 0) {
			continue
		}
		data = append(data, float32(e.a.X), float32(e.a.Y), float32(e.a.Z), c[0], c[1], c[2], c[3],
			float32(e.b.X), float32(e.b.Y), float32(e.b.Z), c[0], c[1], c[2], c[3])
	}
	if len(data) == 0 {
		return
	}
	gl.UseProgram(s.lineProg)
	gl.UniformMatrix4(s.lu["uVP"], s.VP.Float32())
	gl.UniformMatrix4(s.lu["uModel"], st.Model.Float32())
	gl.Uniform4f(s.lu["uClip"], st.Clip[0], st.Clip[1], st.Clip[2], st.Clip[3])
	gl.Uniform1f(s.lu["uAlpha"], 1)
	gl.Uniform4f(s.lu["uOver"], 0, 0, 0, 0)
	gl.BindVertexArray(s.lineVAO)
	gl.BindBuffer(gl.ARRAY_BUFFER, s.lineVBO)
	gl.BufferData(gl.ARRAY_BUFFER, data, gl.DYNAMIC_DRAW)
	gl.DrawArrays(gl.LINES, 0, int32(len(data)/7))
}

func adjacency(m *geom.Mesh) []adjEdge {
	type key [6]int64
	q := func(p geom.Vec3) [3]int64 {
		return [3]int64{int64(math.Round(p.X * 1e4)), int64(math.Round(p.Y * 1e4)), int64(math.Round(p.Z * 1e4))}
	}
	idx := map[key]int{}
	var out []adjEdge
	var count []int
	for _, t := range m.Tris {
		n := t.Normal()
		for i := 0; i < 3; i++ {
			a, b := t.P[i], t.P[(i+1)%3]
			qa, qb := q(a), q(b)
			if qa == qb {
				continue
			}
			var k key
			if qa[0] < qb[0] || (qa[0] == qb[0] && (qa[1] < qb[1] || (qa[1] == qb[1] && qa[2] < qb[2]))) {
				k = key{qa[0], qa[1], qa[2], qb[0], qb[1], qb[2]}
			} else {
				k = key{qb[0], qb[1], qb[2], qa[0], qa[1], qa[2]}
			}
			if j, ok := idx[k]; ok {
				if count[j] == 1 {
					out[j].n1 = n
				}
				count[j]++
				continue
			}
			idx[k] = len(out)
			out = append(out, adjEdge{a: a, b: b, n0: n, n1: n})
			count = append(count, 1)
		}
	}
	// keep only edges shared by exactly two faces: an open or non-manifold
	// edge has no pair of normals to turn between front and back
	keep := out[:0]
	for i, e := range out {
		if count[i] == 2 {
			keep = append(keep, e)
		}
	}
	return keep
}

func bf(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

// DrawLines draws a line list.
func (s *Scene) DrawLines(l *Lines, alpha float32) { s.DrawLinesM(l, alpha, geom.Identity()) }

// DrawLinesM is DrawLines with the points given in model space.
func (s *Scene) DrawLinesM(l *Lines, alpha float32, model geom.Mat4) {
	if len(l.data) == 0 {
		return
	}
	gl.UseProgram(s.lineProg)
	gl.UniformMatrix4(s.lu["uVP"], s.VP.Float32())
	gl.UniformMatrix4(s.lu["uModel"], model.Float32())
	gl.Uniform4f(s.lu["uClip"], 0, 0, 0, 0)
	if alpha == 0 {
		alpha = 1
	}
	gl.Uniform1f(s.lu["uAlpha"], alpha)
	gl.Uniform4f(s.lu["uOver"], 0, 0, 0, 0)
	gl.BindVertexArray(s.lineVAO)
	gl.BindBuffer(gl.ARRAY_BUFFER, s.lineVBO)
	gl.BufferData(gl.ARRAY_BUFFER, l.data, gl.DYNAMIC_DRAW)
	gl.DrawArrays(gl.LINES, 0, int32(len(l.data)/7))
	gl.BindVertexArray(0)
}

// DrawTris draws world-space coloured triangles (pos3+col4 per vertex)
// through the line program.
func (s *Scene) DrawTris(data []float32, alpha float32) { s.DrawTrisM(data, alpha, geom.Identity()) }

// DrawTrisM is DrawTris with the vertices given in model space.
func (s *Scene) DrawTrisM(data []float32, alpha float32, model geom.Mat4) {
	if len(data) == 0 {
		return
	}
	gl.UseProgram(s.lineProg)
	gl.UniformMatrix4(s.lu["uVP"], s.VP.Float32())
	gl.UniformMatrix4(s.lu["uModel"], model.Float32())
	gl.Uniform4f(s.lu["uClip"], 0, 0, 0, 0)
	if alpha == 0 {
		alpha = 1
	}
	gl.Uniform1f(s.lu["uAlpha"], alpha)
	gl.Uniform4f(s.lu["uOver"], 0, 0, 0, 0)
	gl.BindVertexArray(s.lineVAO)
	gl.BindBuffer(gl.ARRAY_BUFFER, s.lineVBO)
	gl.BufferData(gl.ARRAY_BUFFER, data, gl.DYNAMIC_DRAW)
	gl.DrawArrays(gl.TRIANGLES, 0, int32(len(data)/7))
	gl.BindVertexArray(0)
}

// RenderShadows draws the given meshes, seen from straight above, into the
// shadow map covering rect (minx, miny, w, h). It restores fb afterwards.
func (s *Scene) RenderShadows(parts []ShadowPart, rect [4]float64, fb uint32, vx, vy, vw, vh int32) {
	gl.BindFramebuffer(gl.FRAMEBUFFER, s.shadowFBO)
	gl.Viewport(0, 0, s.shadowSize, s.shadowSize)
	gl.ClearColor(0, 0, 0, 0)
	gl.Clear(gl.COLOR_BUFFER_BIT)
	gl.Disable(gl.DEPTH_TEST)
	gl.Disable(gl.BLEND)
	saved := s.VP
	s.VP = geom.Ortho(rect[0], rect[0]+rect[2], rect[1], rect[1]+rect[3], -10000, 10000)
	// faint shadows first so solid ones overwrite them
	for _, light := range []bool{true, false} {
		k := float32(1)
		if light {
			k = 0.4
		}
		for _, p := range parts {
			if p.Light == light {
				s.DrawMesh(p.Mesh, Style{Model: p.M, Flat: true, Tint: [4]float32{k, k, k, 1}})
			}
		}
	}
	s.VP = saved
	gl.BindFramebuffer(gl.FRAMEBUFFER, fb)
	gl.Viewport(vx, vy, vw, vh)
}

// DrawPlane draws the workplane fill (with shadows) as a quad.
func (s *Scene) DrawPlane(minX, minY, maxX, maxY, z float64, color [4]float32, shadowRect [4]float64, shadows bool) {
	q := []float32{
		float32(minX), float32(minY), float32(z), float32(maxX), float32(minY), float32(z), float32(maxX), float32(maxY), float32(z),
		float32(minX), float32(minY), float32(z), float32(maxX), float32(maxY), float32(z), float32(minX), float32(maxY), float32(z),
	}
	gl.UseProgram(s.planeProg)
	gl.UniformMatrix4(s.pu["uVP"], s.VP.Float32())
	gl.Uniform4f(s.pu["uRect"], float32(shadowRect[0]), float32(shadowRect[1]), float32(shadowRect[2]), float32(shadowRect[3]))
	gl.Uniform4f(s.pu["uColor"], color[0], color[1], color[2], color[3])
	gl.Uniform1f(s.pu["uShadowOn"], bf(shadows))
	gl.Uniform2f(s.pu["uTexel"], 1/float32(s.shadowSize), 1/float32(s.shadowSize))
	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D, s.shadowTex)
	gl.Uniform1i(s.pu["uShadow"], 0)
	gl.BindVertexArray(s.planeVAO)
	gl.BindBuffer(gl.ARRAY_BUFFER, s.planeVBO)
	gl.BufferData(gl.ARRAY_BUFFER, q, gl.DYNAMIC_DRAW)
	gl.DrawArrays(gl.TRIANGLES, 0, 6)
	gl.BindVertexArray(0)
}

// MaskPart identifies geometry for the outline mask: channel 0 = red
// (selection), 1 = green (bundle child), 2 = blue (hover).
type MaskPart struct {
	Mesh    *geom.Mesh
	M       geom.Mat4
	Channel int
	Clip    [4]float32 // see Style.Clip
}

// ShadowPart is a mesh placed in the world for the shadow pass.
type ShadowPart struct {
	Mesh *geom.Mesh
	M    geom.Mat4
	// Light casts a fainter shadow (holes).
	Light bool
}

// DrawOutlines renders parts into an offscreen mask and composites
// outlines onto framebuffer fb: a line radius px wide (radiusB for channel
// 2, the hover), and for channel 0 (the selection) a glow fading out to
// glow px at glowAlpha.
func (s *Scene) DrawOutlines(parts []MaskPart, fb uint32, x, y, w, h int32, radius, radiusB, glow, glowAlpha float32, colR, colG, colB [3]float32) {
	if len(parts) == 0 {
		return
	}
	if s.maskFBO == 0 || s.maskW != w || s.maskH != h {
		if s.maskFBO != 0 {
			gl.DeleteFramebuffer(s.maskFBO)
			gl.DeleteTexture(s.maskTex)
			gl.DeleteRenderbuffer(s.maskDepth)
		}
		s.maskTex = gl.GenTexture()
		gl.BindTexture(gl.TEXTURE_2D, s.maskTex)
		gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, w, h, gl.RGBA, gl.UNSIGNED_BYTE, nil)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
		s.maskDepth = gl.GenRenderbuffer()
		gl.BindRenderbuffer(gl.RENDERBUFFER, s.maskDepth)
		gl.RenderbufferStorage(gl.RENDERBUFFER, gl.DEPTH24_STENCIL8, w, h)
		s.maskFBO = gl.GenFramebuffer()
		gl.BindFramebuffer(gl.FRAMEBUFFER, s.maskFBO)
		gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, s.maskTex)
		gl.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.DEPTH_STENCIL_ATTACH, gl.RENDERBUFFER, s.maskDepth)
		s.maskW, s.maskH = w, h
	}
	gl.BindFramebuffer(gl.FRAMEBUFFER, s.maskFBO)
	gl.Viewport(0, 0, w, h)
	gl.ClearColor(0, 0, 0, 0)
	gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
	gl.Disable(gl.DEPTH_TEST)
	gl.Disable(gl.BLEND)
	// draw lower-priority channels first so selection wins
	for _, ch := range []int{2, 0, 1} {
		for _, p := range parts {
			if p.Channel != ch {
				continue
			}
			t := [4]float32{0, 0, 0, 1}
			t[ch] = 1
			s.DrawMesh(p.Mesh, Style{Model: p.M, Flat: true, Tint: t, Clip: p.Clip})
		}
	}
	gl.BindFramebuffer(gl.FRAMEBUFFER, fb)
	gl.Viewport(x, y, w, h)
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.UseProgram(s.outlineProg)
	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D, s.maskTex)
	gl.Uniform1i(s.ou["uMask"], 0)
	gl.Uniform2f(s.ou["uTexel"], 1/float32(w), 1/float32(h))
	gl.Uniform1f(s.ou["uRadius"], radius)
	gl.Uniform1f(s.ou["uRadiusB"], radiusB)
	gl.Uniform1f(s.ou["uGlow"], glow)
	gl.Uniform1f(s.ou["uGlowAlpha"], glowAlpha)
	gl.Uniform3f(s.ou["uColR"], colR[0], colR[1], colR[2])
	gl.Uniform3f(s.ou["uColG"], colG[0], colG[1], colG[2])
	gl.Uniform3f(s.ou["uColB"], colB[0], colB[1], colB[2])
	gl.BindVertexArray(s.quadVAO)
	gl.DrawArrays(gl.TRIANGLES, 0, 6)
	gl.BindVertexArray(0)
}
