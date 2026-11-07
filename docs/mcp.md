# Using ScottiCAD with an AI assistant (MCP)

ScottiCAD is a server for the [Model Context Protocol](https://modelcontextprotocol.io)
(MCP), so an AI assistant that speaks MCP can build and change designs in
it. Claude Code, Claude Desktop and many other assistants do.

The assistant works in an ordinary ScottiCAD window that you can see.
You watch each change as it happens, and you can use the mouse and
keyboard at the same time. A typical request goes like this:

1. You ask for a part: "a stand for drill bits from 1 to 10 mm".
2. The assistant writes a script and runs it. The whole run is one undo
   step, so **Ctrl+Z** takes it all away.
3. It takes a screenshot to look at the result, and fixes what it sees,
   either by running the script again or with small direct edits.
4. It saves or exports the part when you ask.

## Two ways to connect

| | `scotticad --mcp` | `scotticad --mcp-connect 127.0.0.1:47800` |
|---|---|---|
| Who starts ScottiCAD | the assistant's client, when it needs it | you, with `scotticad --api 127.0.0.1:47800` |
| The window | a new one for each assistant session | the one you already have open |
| When the assistant session ends | the window closes, asking to save changes first; cancel to keep it open | nothing changes: ScottiCAD stays open |
| Good for | the simplest setup | keeping one window between sessions, or sharing it with your own scripts |

In both, ScottiCAD is visible. `--headless` hides the window, but only
for automated tests.

### All-in-one: `--mcp`

The client starts ScottiCAD, and the two talk over ScottiCAD's standard
input and output. In this mode ScottiCAD opens on an empty design, not
the start screen.

**Claude Code:**

```
claude mcp add scotticad -- C:\Tools\ScottiCAD\scotticad.exe --mcp
```

Use the path where you unpacked ScottiCAD; on Linux it's the path to
`scotticad`. This adds ScottiCAD for Claude Code sessions started in the
current folder; add `--scope user` after `add` to have it in every folder.

**Claude Desktop:** add this to its configuration file
(*Settings → Developer → Edit Config*), then restart Claude Desktop.
Backslashes in Windows paths are doubled in JSON.

```json
{
  "mcpServers": {
    "scotticad": {
      "command": "C:\\Tools\\ScottiCAD\\scotticad.exe",
      "args": ["--mcp"]
    }
  }
}
```

The other options work as usual, such as `--width`, `--height`, `--scale`
or a design file to open. `--api` serves the automation API from the same
window too; its `SCOTTICAD_API` line then goes to standard error, because
standard output belongs to MCP.

### Connect to a running ScottiCAD: `--mcp-connect`

Start ScottiCAD yourself with the automation API:

```
scotticad --api 127.0.0.1:47800
```

Then give the assistant's client the connecting server instead:

```
claude mcp add scotticad-live -- C:\Tools\ScottiCAD\scotticad.exe --mcp-connect 127.0.0.1:47800
```

`--mcp-connect` opens no window of its own: it passes the assistant's
requests to the ScottiCAD on that address.
- It connects when the assistant first needs it, and again if you close
  and reopen ScottiCAD, so it doesn't matter which you start first.
- If ScottiCAD isn't running, the assistant is told how to start it.
- The address must be on this computer (`127.0.0.1`, `::1` or
  `localhost`).

## What the assistant can do

The tools are ScottiCAD's [automation API](api.md) methods, and their
descriptions and parameters come from it. The main ones:

| Tool | What it's for |
|---|---|
| `run_script` | runs a [Starlark script](../README.md#scripting): the way to build anything with more than a few shapes, as one undo step |
| `screenshot` | a picture of the window, so the assistant can check its work |
| `list_nodes`, `get_node`, `get_state` | reading the design: ids, kinds, sizes, positions |
| `add_shape`, `move`, `set_size`, `set_bounds`, `rotate`, `set_hole`, `set_color`, `group`, … | small direct edits |
| `undo`, `redo` | the editor's undo and redo |
| `fit_view`, `set_camera`, `set_section` | framing the screenshot, and cutting the design open to see inside |
| `new_design`, `open`, `save`, `import`, `export` | files |

**Not available:** the API's simulated mouse and keyboard, the methods
that read the interface's layout, and the ones for automated test runs.
An assistant does better calling methods than clicking its way round the
window.

**`--mcp-tools core`** lists about 20 tools instead of nearly 60, for
assistants that do worse with a long list.

The assistant can also read three **resources**:
- `scotticad://script-api`: every `cad` method scripts can call;
- `scotticad://design`: the open design, as JSON;
- `scotticad://shape-kinds`: the shape catalogue.

If the assistant cancels a request while its script is running, the
script stops, as it would with the console's **Stop** button.

## Getting good results

- **Give sizes.** "A box for a 60 × 40 mm board, 2 mm walls, M3 screw
  holes 3 mm in from each corner" gets closer first time than "a box for my
  board".
- **Ask it to look.** The assistant usually takes a screenshot after a
  change; asking it to check from another side, or to cut the design open
  with a section, catches mistakes inside a part.
- **Keep the script.** The assistant's scripts are ordinary ScottiCAD
  scripts. Ask it to show you the one it ran (an assistant that can write
  files, such as Claude Code, can save it as a `.star` file), and paste or
  open it in the console's **Editor** tab to change a size and run it
  again yourself.
- **Say where to save.** Name a folder and file, or start ScottiCAD with
  `--root` (below) so everything it saves goes in one place.

## Files and safety

The assistant can do what the automation API can, in your ScottiCAD:
- add, change and delete shapes;
- open, save, import and export files.

Most assistants ask your permission before each tool call, unless you've
allowed that tool permanently. The tools that discard work or overwrite
files (`new_design`, `open`, `save`, `export` and `delete`) are marked
as such for the assistant.

Scripts can't touch your files, the network or other programs, apart
from the files they name in `cad` calls.

**`--root DIR`** keeps every file the assistant names inside one folder:
- the file tools' descriptions name the folder, so the assistant knows
  where to save;
- relative paths resolve against it;
- a path outside it is refused, whether it comes from a tool or from a
  script's `cad.save` or `cad.export`;
- saving over the open design's own file is refused if that file is
  outside the folder.

```
scotticad --mcp --root C:\Users\me\Documents\AI-designs
```

With `--mcp-connect`, give `--root` to the ScottiCAD you start with
`--api` instead: that ScottiCAD enforces it.

The automation API has no password. With `--mcp-connect`, anything on
your computer that can reach its port can drive ScottiCAD, so serve it on
`127.0.0.1` only (see [api.md](api.md)).

## When something goes wrong

- **ScottiCAD is busy.** A call that hasn't finished after 60 seconds
  tells the assistant that ScottiCAD is busy. Usually a dialog (a file
  dialog, or a question) is waiting for you in its window. Scripts have
  no time limit; **Stop** ends one.
- **Nothing happens.** Your assistant's MCP log shows ScottiCAD's
  messages, which go to standard error. In Claude Code, `claude mcp list`
  shows whether the server started.
- **The tools seem out of date.** With `--mcp-connect`, the tools are
  read from the ScottiCAD you connect to. Until it is running, the list
  comes from the version of ScottiCAD the connecting server belongs to.
  Keep the two the same version.
