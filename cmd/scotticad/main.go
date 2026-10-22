//go:build windows

// Command scotticad is the ScottiCAD 3D modeler.
//
//	scotticad [design.scotticad]
//	scotticad --api 127.0.0.1:47800            # with the automation API
//	scotticad --api 127.0.0.1:0 --headless     # hidden window for tests
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/scotticad/scotticad"
	"github.com/scotticad/scotticad/pkg/app"
	"github.com/scotticad/scotticad/pkg/win"
)

func main() {
	api := flag.String("api", "", "serve the automation API on this address (host:port; port 0 picks one)")
	headless := flag.Bool("headless", false, "hidden window (for automated tests)")
	width := flag.Int("width", 1600, "window width")
	height := flag.Int("height", 1000, "window height")
	showVersion := flag.Bool("version", false, "print the version and exit")
	scale := flag.Float64("scale", 0, "interface scale (2 = 200%); default: the display's")
	flag.Parse()
	if *showVersion {
		fmt.Println("ScottiCAD", scotticad.Version())
		return
	}

	// sizes in logical pixels; a scale beyond these leaves no usable window
	bad := ""
	switch {
	case *width < 200 || *width > 16384 || *height < 200 || *height > 16384:
		bad = "--width and --height must be 200 to 16384"
	case *scale != 0 && (*scale < 0.5 || *scale > 5):
		bad = "--scale must be 0.5 to 5"
	}
	if bad != "" {
		fmt.Fprintln(os.Stderr, "scotticad:", bad)
		os.Exit(2)
	}
	o := app.Options{Headless: *headless, Width: *width, Height: *height, Scale: *scale}
	if flag.NArg() > 0 {
		o.File = flag.Arg(0)
	}
	a, err := app.New(o)
	if err != nil {
		fmt.Fprintln(os.Stderr, "scotticad:", err)
		if !*headless {
			win.ErrorBox("ScottiCAD", err.Error())
		}
		os.Exit(1)
	}
	if *api != "" {
		addr, err := a.ServeAPI(*api)
		if err != nil {
			fmt.Fprintln(os.Stderr, "scotticad: api:", err)
			os.Exit(1)
		}
		// test harnesses read this line to find the port
		fmt.Printf("SCOTTICAD_API %s\n", addr)
		os.Stdout.Sync()
	}
	a.Run()
}
