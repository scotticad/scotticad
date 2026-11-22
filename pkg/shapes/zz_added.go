package shapes

import "github.com/scotticad/scotticad/pkg/geom"

// Shapes added after the first catalogue register here. This file's name
// sorts last, so its init runs after every other file's and these kinds
// come after the older ones in All(): a property test seeds each kind's
// random cases by its place in the catalogue, and new kinds at the end
// leave the older kinds' cases as they were.

func init() {
	// --- basic shapes
	register(&Kind{ID: "slot", Label: "Slot", Color: "#e2142a", Size: geom.V(30, 10, 10),
		Params: []Param{slider("sides", "Sides", 8, 128, 2, 48, true)}, build: slotMesh})
	registerGenerated("arc", "Arc", "#f6851f", []Param{
		lslider("radius", "Radius", 1, 200, 0.5, 20),
		lslider("wall", "Wall Thickness", 0, 200, 0.5, 5),
		slider("angle", "Angle", 1, 360, 1, 90, false),
		lslider("height", "Height", 0.4, 200, 0.5, 5),
		slider("sides", "Sides", 8, 256, 1, 96, true),
	}, arcModel)
	register(&Kind{ID: "fillet", Label: "Fillet", Color: "#1c9ad6", Size: geom.V(20, 5, 5),
		Params: []Param{slider("steps", "Steps", 2, 64, 1, 16, true)}, build: filletMesh})
	registerGenerated("teardrop", "Teardrop", "#7b2c9d", []Param{
		lslider("diameter", "Diameter", 1, 200, 0.5, 10),
		lslider("length", "Length", 0.4, 500, 0.5, 20),
		slider("flat", "Flat Top", 0, 1, 1, 0, true),
		slider("sides", "Sides", 12, 128, 1, 48, true),
		slider("tip", "Tip Angle", 40, 140, 1, 90, false),
	}, teardropModel)
	register(&Kind{ID: "crystal", Label: "Crystal", Color: "#b18be0", Size: geom.V(12, 12, 30),
		Params: []Param{
			slider("sides", "Sides", 3, 12, 1, 6, true),
			slider("point", "Point Height (%)", 5, 75, 1, 30, false),
			slider("taper", "Taper (%)", 0, 90, 1, 0, false),
			slider("double", "Double Pointed", 0, 1, 1, 0, true),
			slider("irregular", "Irregularity", 0, 100, 1, 0, false),
			slider("seed", "Seed", 1, 1000, 1, 1, true),
		}, build: crystalMesh})

	// --- hardware holes
	hw := &HardwareKinds
	registerPart(hw, "keyhole", "Keyhole Slot", "#9a9a9a", []Param{
		lslider("head", "Head Dia", 3, 30, 0.1, 8),
		lslider("shank", "Shank Dia", 1, 20, 0.1, 4),
		lslider("slot", "Slot Length", 2, 60, 0.5, 8),
		lslider("depth", "Depth", 1, 50, 0.5, 6),
		lslider("recess", "Recess Depth", 0, 30, 0.5, 3),
	}, keyholeModel)
	registerPart(hw, "bearingseat", "Bearing Seat", "#9a9a9a", []Param{
		choice("bearing", "Bearing", bearingNames(), 6),
		lslider("fit", "Fit", -0.3, 0.6, 0.05, 0.1),
		lslider("below", "Hole Below", 0, 50, 0.5, 2),
	}, bearingSeatModel)
	HoleKinds["keyhole"], HoleKinds["bearingseat"] = true, true

	// --- mechanical
	mk := &MechanicalKinds
	bevel := []Param{
		mslider("module", "Module", 0.5, 5, 0.1, 1.5),
		slider("teeth", "Teeth", 8, 80, 1, 20, true),
		slider("mate", "Mate Teeth", 8, 80, 1, 20, true),
		lslider("face", "Face Width", 1, 50, 0.5, 6),
		lslider("bore", "Bore Dia", 0, 50, 0.5, 5),
		lslider("back", "Back", 0, 20, 0.5, 2),
		lslider("clearance", "Clearance", 0, 0.5, 0.05, 0.1),
	}
	registerPart(mk, "bevelgear", "Bevel Gear", "#c8ccd0", bevel, bevelGearModel)
	registerHiddenPart("bevelpair-gear", "Bevel Gear", "#c8ccd0", bevel, bevelPairFirst)
	registerHiddenPart("bevelpair-mate", "Mating Bevel Gear", "#a7b0b8", bevel, bevelPairSecond)
	registerAssembly(mk, "bevelpair", "Bevel Gear Pair", "#c8ccd0", bevel, bevelPairParts)

	registerPart(mk, "gt2pulley", "GT2 Pulley", "#c8ccd0", []Param{
		slider("teeth", "Teeth", 12, 80, 1, 20, true),
		lslider("belt", "Belt Width", 2, 20, 0.5, 6),
		lslider("bore", "Bore Dia", 0, 20, 0.1, 5),
		choice("flanges", "Flanges", []string{"None", "Lower", "Both"}, 2),
		lslider("fit", "Groove Fit", 0, 0.3, 0.05, 0.2),
		lslider("hub", "Hub Height", 0, 30, 0.5, 6),
		lslider("hubdia", "Hub Dia", 4, 60, 0.5, 14),
	}, gt2Model)

	dove := []Param{
		lslider("neck", "Neck Width", 2, 50, 0.5, 8),
		lslider("length", "Length", 2, 50, 0.5, 8),
		slider("flare", "Flare Angle", 0, 30, 1, 15, false),
		lslider("height", "Height", 1, 200, 0.5, 5),
		lslider("clearance", "Clearance", 0, 1, 0.05, 0.2),
	}
	registerHiddenPart("dovetail-key", "Dovetail Key", "#d35400", dove, dovetailKey)
	registerHiddenPart("dovetail-socket", "Dovetail Socket", "#9a9a9a", dove, dovetailSocket)
	registerAssembly(mk, "dovetail", "Dovetail Joint", "#d35400", dove, dovetailParts)

	pin := []Param{
		lslider("diameter", "Diameter", 1, 30, 0.1, 5),
		lslider("length", "Length", 1, 50, 0.5, 6),
		lslider("clearance", "Clearance", 0, 1, 0.05, 0.2),
		lslider("chamfer", "Tip Chamfer", 0, 3, 0.1, 0.6),
	}
	registerHiddenPart("alignpin-pin", "Alignment Pin", "#d35400", pin, pinKey)
	registerHiddenPart("alignpin-socket", "Pin Socket", "#9a9a9a", pin, pinSocket)
	registerAssembly(mk, "alignpin", "Alignment Pin", "#d35400", pin, pinParts)

	// --- parametric: the QR code (in the Parametric library's own list)
	qr := []Param{
		{Key: "text", Label: "Text", Kind: TextParam, SizeAxis: -1},
		choice("level", "Error Correction", qrLevels, qrMedium),
		lslider("size", "Size", 10, 300, 1, 40),
		lslider("height", "Height", 0.2, 10, 0.1, 0.6),
		lslider("base", "Base", 0, 10, 0.1, 1),
		slider("margin", "Margin (modules)", 0, 8, 1, 4, true),
	}
	const qrText = "https://scotticad.com"
	// registering sized each kind with no text: give it the default text
	// and the size that text makes
	withText := func(k *Kind) {
		k.text = qrText
		k.Size = k.natural(k.Defaults())
	}
	withText(registerHiddenPart("qrcode-base", "QR Code Base", "#f2f2f2", qr, qrBase))
	withText(registerHiddenPart("qrcode-code", "QR Code", "#1e1e1e", qr, qrCode))
	var parametric []string
	registerAssembly(&parametric, "qrcode", "QR Code", "#1e1e1e", qr, qrParts)
	withText(catalogue["qrcode"])

	// --- the rounded box, crescent, port opening and finishing washer
	register(&Kind{ID: "roundedbox", Label: "Rounded Box", Color: "#3fae2a", Size: geom.V(40, 30, 20),
		Params: append([]Param{
			lslider("radius", "Corner Radius", 0, 100, 0.1, 5),
			lslider("top", "Top Edge Radius", 0, 100, 0.1, 0),
			lslider("bottom", "Bottom Edge Radius", 0, 100, 0.1, 0),
			slider("steps", "Steps", 1, 64, 1, 8, true),
		}, sizeParams()...), build: roundedBoxMesh})
	register(&Kind{ID: "crescent", Label: "Crescent", Color: "#f7c51e", Size: geom.V(40, 40, 10),
		Params: []Param{
			lslider("thickness", "Thickness", 1, 500, 0.5, 14),
			lslider("tip", "Tip Radius", 0, 8, 0.1, 0.5),
			slider("steps", "Steps", 16, 256, 1, 32, true),
		}, build: crescentMesh})
	registerPart(hw, "port", "Port Opening", "#9a9a9a", []Param{
		choice("connector", "Connector", portConnectorNames(), 1),
		lslider("depth", "Depth", 0.5, 100, 0.5, 10),
		lslider("clearance", "Clearance", 0, 2, 0.05, 0),
		lslider("width", "Custom Width", 0.5, 200, 0.1, 9.4),
		lslider("height", "Custom Height", 0.5, 200, 0.1, 3.7),
		lslider("corner", "Custom Corner Radius", 0, 100, 0.05, 1.85),
		choice("outline", "Custom Outline", portOutlines, 0),
		slider("sides", "Sides", 8, 256, 1, 32, true),
	}, portModel)
	registerPart(hw, "finishingwasher", "Finishing Washer", "#9a9a9a", []Param{
		lslider("diameter", "Outer Dia", 2, 100, 0.1, 13.4),
		lslider("height", "Height", 0.5, 30, 0.05, 3.95),
		lslider("hole", "Hole Dia", 0.2, 50, 0.1, 4.5),
		lslider("seat", "Seat Depth", 0, 30, 0.05, 2.45),
		slider("angle", "Seat Angle", 30, 150, 1, 90, false),
		slider("sides", "Sides", 8, 256, 1, 48, true),
	}, finishingWasherModel)

	// --- Gridfinity storage
	offOn := []string{"Off", "On"}
	registerPart(hw, "gridfinitybin", "Gridfinity Bin", "#e8a33d", []Param{
		slider("unitsx", "Units Long", 1, 10, 1, 2, true),
		slider("unitsy", "Units Wide", 1, 10, 1, 1, true),
		slider("unitsz", "Units High", 1, 20, 1, 3, true),
		choice("lip", "Stacking Lip", offOn, 1),
		choice("holes", "Holes", []string{"None", "Magnets", "Screws", "Magnets and Screws"}, 0),
		lslider("wall", "Wall", 0.4, 3, 0.05, 1.2),
		slider("compx", "Compartments Along", 1, 10, 1, 1, true),
		slider("compy", "Compartments Across", 1, 10, 1, 1, true),
		lslider("scoop", "Scoop Radius", 0, 30, 0.5, 0),
		lslider("label", "Label Shelf", 0, 30, 0.5, 0),
	}, gfBinModel)
	registerPart(hw, "gridfinityplate", "Gridfinity Baseplate", "#5b6770", []Param{
		slider("unitsx", "Units Long", 1, 10, 1, 3, true),
		slider("unitsy", "Units Wide", 1, 10, 1, 2, true),
		choice("bottom", "Bottom", []string{"Open", "Solid", "Magnet Holes"}, 0),
		lslider("floor", "Floor Thickness", 0.4, 20, 0.1, 3),
	}, gfBaseplateModel)

	// --- sketches (see sketch.go)
	registerSketchKinds()
}
