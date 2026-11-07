// Command scotticad is the ScottiCAD 3D modeler.
//
//	scotticad [design.scotticad]
//	scotticad --api 127.0.0.1:47800            # with the automation API
//	scotticad --api 127.0.0.1:0 --headless     # hidden window for tests
//	scotticad --mcp                            # for an AI assistant (MCP on stdin/stdout)
//	scotticad --mcp-connect 127.0.0.1:47800    # MCP, driving a ScottiCAD started with --api
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/scotticad/scotticad"
	"github.com/scotticad/scotticad/pkg/app"
	"github.com/scotticad/scotticad/pkg/mcp"
	"github.com/scotticad/scotticad/pkg/window"
)

func main() {
	api := flag.String("api", "", "serve the automation API on this address (host:port; port 0 picks one)")
	headless := flag.Bool("headless", false, "hidden window (for automated tests)")
	width := flag.Int("width", 1600, "window width")
	height := flag.Int("height", 1000, "window height")
	showVersion := flag.Bool("version", false, "print the version and exit")
	scale := flag.Float64("scale", 0, "interface scale (2 = 200%); default: the display's")
	mcpMode := flag.Bool("mcp", false, "serve MCP on stdin/stdout for an AI assistant, in this window")
	mcpConnect := flag.String("mcp-connect", "", "serve MCP on stdin/stdout, driving the ScottiCAD whose --api is on this address")
	mcpTools := flag.String("mcp-tools", "full", "the MCP tools listed: full or core")
	root := flag.String("root", "", "limit the files the API, MCP and scripts may use to this folder")
	flag.CommandLine.SetOutput(consoleErr{})
	flag.Usage = usage
	flag.Parse()
	if *showVersion {
		useConsole()
		fmt.Println("ScottiCAD", scotticad.Version())
		return
	}
	set := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { set[f.Name] = true })

	// sizes in logical pixels; a scale beyond these leaves no usable window
	bad := ""
	switch {
	case *width < 200 || *width > 16384 || *height < 200 || *height > 16384:
		bad = "--width and --height must be 200 to 16384"
	case *scale != 0 && (*scale < 0.5 || *scale > 5):
		bad = "--scale must be 0.5 to 5"
	case *mcpTools != "full" && *mcpTools != "core":
		bad = "--mcp-tools must be full or core"
	case *mcpMode && *mcpConnect != "":
		bad = "--mcp and --mcp-connect can't be used together"
	case set["mcp-tools"] && !*mcpMode && *mcpConnect == "":
		bad = "--mcp-tools needs --mcp or --mcp-connect"
	case *mcpConnect != "" && (set["api"] || set["headless"] || set["width"] || set["height"] || set["scale"] || flag.NArg() > 0):
		bad = "--mcp-connect opens no window: start the ScottiCAD it connects to with those options"
	case *mcpConnect != "" && set["root"]:
		bad = "--mcp-connect can't limit files: start the ScottiCAD it connects to with --root"
	}
	if bad != "" {
		fail(2, bad)
	}

	// MCP owns standard output: anything else printed goes to stderr (an
	// --api's SCOTTICAD_API line, say), so it can't corrupt the stream
	mcpOut := os.Stdout
	if *mcpMode || *mcpConnect != "" {
		os.Stdout = os.Stderr
	}
	mo := mcp.Options{Version: scotticad.Version(), Tools: *mcpTools, Log: os.Stderr}

	if *mcpConnect != "" {
		if err := mcp.CheckLoopback(*mcpConnect); err != nil {
			fail(2, err)
		}
		mo.Fallback, _ = app.MethodTable()
		if err := mcp.Serve(os.Stdin, mcpOut, &mcp.Remote{Addr: *mcpConnect}, mo); err != nil {
			fail(1, "mcp:", err)
		}
		return
	}

	o := app.Options{Headless: *headless, Width: *width, Height: *height, Scale: *scale, Root: *root, NoStartScreen: *mcpMode}
	if flag.NArg() > 0 {
		o.File = flag.Arg(0)
	}
	a, err := app.New(o)
	if err != nil {
		if !*headless {
			window.ErrorBox("ScottiCAD", err.Error())
		}
		fail(1, err)
	}
	if *api != "" {
		addr, err := a.ServeAPI(*api)
		if err != nil {
			fail(1, "api:", err)
		}
		// test harnesses read this line to find the port
		fmt.Printf("SCOTTICAD_API %s\n", addr)
		os.Stdout.Sync()
	}
	if *mcpMode {
		go func() {
			if err := mcp.Serve(os.Stdin, mcpOut, a, mo); err != nil {
				fmt.Fprintln(os.Stderr, "scotticad: mcp:", err)
			}
			a.CloseForClient() // the client has gone
		}()
	}
	a.Run()
}

func usage() {
	fmt.Fprintf(flag.CommandLine.Output(), `ScottiCAD %s, a 3D modeller for 3D printing

Usage:
  scotticad [options] [design.scotticad]
  scotticad --api 127.0.0.1:47800           serve the automation API
  scotticad --mcp                           for an AI assistant (MCP on stdin/stdout)
  scotticad --mcp-connect 127.0.0.1:47800   MCP, driving a ScottiCAD started with --api

Options:
`, scotticad.Version())
	flag.PrintDefaults()
}

// fail prints an error and exits.
func fail(code int, a ...any) {
	useConsole()
	fmt.Fprintln(os.Stderr, append([]any{"scotticad:"}, a...)...)
	os.Exit(code)
}

// consoleErr is standard error, reached through useConsole: the flag
// package writes --help and its own errors to it.
type consoleErr struct{}

func (consoleErr) Write(p []byte) (int, error) {
	useConsole()
	return os.Stderr.Write(p)
}
