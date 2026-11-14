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
		lslider("radius", "Radius", 1, 200, 0.5, 20, false),
		lslider("wall", "Wall Thickness", 0, 200, 0.5, 5, false),
		slider("angle", "Angle", 1, 360, 1, 90, false),
		lslider("height", "Height", 0.4, 200, 0.5, 5, false),
		slider("sides", "Sides", 8, 256, 1, 96, true),
	}, arcModel)
	register(&Kind{ID: "fillet", Label: "Fillet", Color: "#1c9ad6", Size: geom.V(20, 5, 5),
		Params: []Param{slider("steps", "Steps", 2, 64, 1, 16, true)}, build: filletMesh})
	registerGenerated("teardrop", "Teardrop", "#7b2c9d", []Param{
		lslider("diameter", "Diameter", 1, 200, 0.5, 10, false),
		lslider("length", "Length", 0.4, 500, 0.5, 20, false),
		slider("flat", "Flat Top", 0, 1, 1, 0, true),
		slider("sides", "Sides", 12, 128, 1, 48, true),
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
		lslider("head", "Head Dia", 3, 30, 0.1, 8, false),
		lslider("shank", "Shank Dia", 1, 20, 0.1, 4, false),
		lslider("slot", "Slot Length", 2, 60, 0.5, 8, false),
		lslider("depth", "Depth", 1, 50, 0.5, 6, false),
		lslider("recess", "Recess Depth", 0, 30, 0.5, 3, false),
	}, keyholeModel)
	registerPart(hw, "bearingseat", "Bearing Seat", "#9a9a9a", []Param{
		choice("bearing", "Bearing", bearingNames(), 6),
		lslider("fit", "Fit", -0.3, 0.6, 0.05, 0.1, false),
		lslider("below", "Hole Below", 0, 50, 0.5, 2, false),
	}, bearingSeatModel)
	HoleKinds["keyhole"], HoleKinds["bearingseat"] = true, true

	// --- mechanical
	mk := &MechanicalKinds
	bevel := []Param{
		mslider("module", "Module", 0.5, 5, 0.1, 1.5, false),
		slider("teeth", "Teeth", 8, 80, 1, 20, true),
		slider("mate", "Mate Teeth", 8, 80, 1, 20, true),
		lslider("face", "Face Width", 1, 50, 0.5, 6, false),
		lslider("bore", "Bore Dia", 0, 50, 0.5, 5, false),
		lslider("back", "Back", 0, 20, 0.5, 2, false),
		lslider("clearance", "Clearance", 0, 0.5, 0.05, 0.1, false),
	}
	registerPart(mk, "bevelgear", "Bevel Gear", "#c8ccd0", bevel, bevelGearModel)
	registerHiddenPart("bevelpair-gear", "Bevel Gear", "#c8ccd0", bevel, bevelPairFirst)
	registerHiddenPart("bevelpair-mate", "Mating Bevel Gear", "#a7b0b8", bevel, bevelPairSecond)
	registerAssembly(mk, "bevelpair", "Bevel Gear Pair", "#c8ccd0", bevel, bevelPairParts)

	registerPart(mk, "gt2pulley", "GT2 Pulley", "#c8ccd0", []Param{
		slider("teeth", "Teeth", 12, 80, 1, 20, true),
		lslider("belt", "Belt Width", 2, 20, 0.5, 6, false),
		lslider("bore", "Bore Dia", 0, 20, 0.1, 5, false),
		choice("flanges", "Flanges", []string{"None", "Lower", "Both"}, 2),
		lslider("fit", "Groove Fit", 0, 0.3, 0.05, 0.2, false),
		lslider("hub", "Hub Height", 0, 30, 0.5, 6, false),
		lslider("hubdia", "Hub Dia", 4, 60, 0.5, 14, false),
	}, gt2Model)

	dove := []Param{
		lslider("neck", "Neck Width", 2, 50, 0.5, 8, false),
		lslider("length", "Length", 2, 50, 0.5, 8, false),
		slider("flare", "Flare Angle", 0, 30, 1, 15, false),
		lslider("height", "Height", 1, 200, 0.5, 5, false),
		lslider("clearance", "Clearance", 0, 1, 0.05, 0.2, false),
	}
	registerHiddenPart("dovetail-key", "Dovetail Key", "#d35400", dove, dovetailKey)
	registerHiddenPart("dovetail-socket", "Dovetail Socket", "#9a9a9a", dove, dovetailSocket)
	registerAssembly(mk, "dovetail", "Dovetail Joint", "#d35400", dove, dovetailParts)

	pin := []Param{
		lslider("diameter", "Diameter", 1, 30, 0.1, 5, false),
		lslider("length", "Length", 1, 50, 0.5, 6, false),
		lslider("clearance", "Clearance", 0, 1, 0.05, 0.2, false),
		lslider("chamfer", "Tip Chamfer", 0, 3, 0.1, 0.6, false),
	}
	registerHiddenPart("alignpin-pin", "Alignment Pin", "#d35400", pin, pinKey)
	registerHiddenPart("alignpin-socket", "Pin Socket", "#9a9a9a", pin, pinSocket)
	registerAssembly(mk, "alignpin", "Alignment Pin", "#d35400", pin, pinParts)

	// --- parametric: the QR code (in the Parametric library's own list)
	qr := []Param{
		{Key: "text", Label: "Text", Kind: TextParam, SizeAxis: -1},
		choice("level", "Error Correction", qrLevels, qrMedium),
		lslider("size", "Size", 10, 300, 1, 40, false),
		lslider("height", "Height", 0.2, 10, 0.1, 0.6, false),
		lslider("base", "Base", 0, 10, 0.1, 1, false),
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
}
