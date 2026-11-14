# 12. Working with an AI assistant

[Contents](README.md) · Previous: [Scripting](11-scripting.md) · Next: [The shape library](13-shape-library.md)

ScottiCAD can take instructions from an AI assistant such as Claude. You
describe a part in words, and the assistant builds it in a ScottiCAD
window while you watch.

Ask for "a wall bracket for a 25 mm pipe with two countersunk M4 holes",
and the assistant writes a script, runs it, takes a screenshot to check
the result, and corrects what it sees. Along the way it can use the
[print check](09-printing.md) to make sure the part will print.

## What to expect

- **It happens in an ordinary ScottiCAD window.** You see each step, and
  you can take over with the mouse at any moment: move something, change
  a size, or carry on by hand.
- **Every step can be undone.** Each script the assistant runs is one
  **Ctrl+Z**.
- **Your files stay yours.** You can limit the assistant to one folder,
  so it can only open and save designs there.
- **Nothing else is sent anywhere by ScottiCAD.** The assistant sees the
  design and the screenshots it asks for, because that is how it works.
  ScottiCAD itself still has no account and no online service.

## Setting it up

The assistant reaches ScottiCAD through MCP (the Model Context
Protocol), which many assistants support. For Claude Code, one command
does it. Give the full path to where you unpacked ScottiCAD:

```
claude mcp add scotticad -- C:\Tools\ScottiCAD\scotticad.exe --mcp
```

ScottiCAD then opens in its own window whenever the assistant starts a
session.

[The MCP guide](../mcp.md) covers the rest: other assistants, working in
a ScottiCAD window you already have open, limiting which files the
assistant can reach, and the full list of what it can do.

## Getting good results

- **Give real numbers.** "A box 80 × 50 × 30 mm with 2 mm walls" gets
  you further than "a small box".
- **Say how it will be printed**, if it matters: which face goes down,
  whether supports are acceptable, the nozzle size.
- **Work in steps.** Ask for the main shape, look at it, then ask for
  the details. It is easier to correct one thing at a time.
- **Point at problems.** "The hole on the left is too close to the edge"
  is something the assistant can act on. So is fixing it yourself and
  saying what you changed.
- **Keep the script.** The assistant builds with scripts, and a script
  is a recipe you can run again with different numbers. Ask it to save
  the script beside the design.

---

[Contents](README.md) · Previous: [Scripting](11-scripting.md) · Next: [The shape library](13-shape-library.md)
