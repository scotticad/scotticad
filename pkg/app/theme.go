//go:build windows

package app

import "github.com/scotticad/scotticad/pkg/render"

// Theme values. Metrics are logical pixels (scaled by DPI).
var (
	// chrome
	colBG          = render.RGB(0xff, 0xff, 0xff) // viewport background
	colChrome      = render.RGB(0xff, 0xff, 0xff) // title bar, toolbar, panel
	colBorder      = render.RGB(0xe1, 0xe5, 0xea)
	colDivider     = render.RGB(0xd5, 0xda, 0xe0)
	colText        = render.RGB(0x33, 0x3c, 0x47)
	colTextDim     = render.RGB(0x8a, 0x94, 0x9e)
	colIcon        = render.RGB(0x6f, 0x7a, 0x86)
	colIconHover   = render.RGB(0x1f, 0x7a, 0xe0)
	colIconAccent  = render.RGB(0x3c, 0x95, 0xe0) // toolbar icons' key element; lighter than hover, so hover still shows
	colIconOff     = render.RGB(0xcf, 0xd5, 0xdb)
	colBlue        = render.RGB(0x2f, 0x7f, 0xe6) // primary button, active labels
	colBlueLight   = render.RGB(0xe6, 0xf1, 0xfd)
	colHoverBG     = render.RGB(0xf0, 0xf3, 0xf6)
	colTooltip     = render.RGB(0x23, 0x2f, 0x40)
	colTooltipKey  = render.RGB(0x9f, 0xe3, 0xc6)
	colBanner      = render.RGB(0xee, 0xf0, 0xf2)
	colFieldBorder = render.RGB(0xc8, 0xcf, 0xd6)
	colDim         = render.Color{0, 0, 0, 0.35}
	colRaised      = render.RGB(0xf7, 0xf8, 0xfa) // an inset block in a panel
	colSwatchOff   = render.RGB(0xc4, 0xcb, 0xd3) // the Hole swatch, not chosen
	colOnColor     = render.RGB(0xff, 0xff, 0xff) // text and marks on the accent or a dark fill, in every theme

	// viewport
	colPlane       = render.Color{0.80, 0.90, 0.97, 0.88} // workplane fill
	colPlaneEdge   = render.Color{0.60, 0.80, 0.94, 0.75}
	colGridMinor   = render.Color{0.62, 0.79, 0.91, 0.55}
	colGridMajor   = render.Color{0.45, 0.68, 0.87, 0.85}
	colWatermark   = render.Color{0.53, 0.76, 0.92, 0.55}
	colShapeWP     = render.Color{0.55, 0.85, 0.45, 0.35} // Shape Workplane (green)
	colHoleTint    = [4]float32{0.80, 0.82, 0.85, 1}      // hole body
	colHoleAlpha   = float32(0.34)
	colHoleEdge    = [4]float32{0.27, 0.31, 0.36, 0.85} // hole rims and silhouette
	colSelect      = [3]float32{0.09, 0.78, 1.0}        // cyan outline
	colBundleChild = [3]float32{0.24, 0.82, 0.35}       // green outline
	colHover       = [3]float32{0.45, 0.86, 1.0}
	colHandleDark  = render.RGB(0x20, 0x24, 0x2a)
	colHandleLight = render.RGB(0xff, 0xff, 0xff)
	colHandleHot   = render.RGB(0xff, 0x6a, 0x00)
	colProtractor  = render.RGB(0x55, 0x60, 0x6b)
	colSweep       = render.Color{1.0, 0.55, 0.15, 0.55}
	colProtBand    = render.Color{0.42, 0.72, 0.89, 0.45} // protractor band
	colProtLine    = render.RGB(0x3a, 0x9d, 0xdc)
	colProtRef     = render.RGB(0xff, 0x6a, 0x00) // orange 0° reference line
	colRulerBlue   = render.RGB(0x2f, 0x5f, 0x9f)
	colRulerTick   = render.RGB(0x3a, 0x9d, 0xdc)         // ruler arms and ticks
	colDimBand     = render.Color{0.55, 0.78, 0.93, 0.55} // size-line highlight
	colDimLine     = render.RGB(0x2b, 0x33, 0x3b)         // size lines and arrows
	colRulerBand   = render.Color{0.45, 0.85, 0.35, 0.45} // ruler distance highlight
	colRulerArrow  = render.Color{0.22, 0.55, 0.20, 0.9}
	colMarquee     = render.RGB(0xe8, 0x3a, 0x3a)
	colGhostAlign  = [4]float32{1.0, 0.55, 0.10, 0.6}
	colMirrorHot   = render.RGB(0xe8, 0x2c, 0x2c)
	colGlow        = render.Color{0.25, 0.80, 1.0, 1}

	// set by the theme too (see Theme)
	colBlueDark = render.RGB(0x1d, 0x6b, 0xd0)         // a primary button hovered
	colTabIcon  = render.RGB(0x2b, 0x9c, 0xd8)         // the Workplane / Ruler / Notes tabs
	colBusy     = render.RGB(0x1b, 0x63, 0xb8)         // the "working…" pill
	colTextSel  = render.RGB(0x3d, 0x8e, 0xf0)         // a field's text, all selected
	colCodeSel  = render.Color{0.24, 0.56, 0.94, 0.25} // a selection in the code editor and console
)

const (
	titleH       = 48.0
	toolbarH     = 46.0
	panelW       = 246.0 // right shape panel
	tileSize     = 64.0
	inspectorW   = 236.0
	leftColX     = 18.0
	leftBtn      = 34.0
	viewCube     = 64.0
	iconSize     = 24.0 // toolbar icons
	toolBtnW     = 38.0 // toolbar button pitch: sets the minimum window width
	fontUI       = 13.0
	fontSmall    = 11.5
	fontLabel    = 12.0
	handleSize   = 9.0
	dimOffset    = 30.0 // px from the bounding box to a size line
	protBandOut  = 1.18 // protractor band outer edge / inner radius
	protOuter    = 1.38 // protractor outer circle / inner radius
	labelPadX    = 6.0
	tooltipDelay = 450 // ms
)
