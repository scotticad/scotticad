//go:build windows

package app

import "github.com/scotticad/scotticad/pkg/win"

// Prompts ("Save changes?") and file dialogs go through these, so that
// automated runs can answer them: answers queued by the API's
// answer_dialogs are used first, in order of their kind. With none queued
// a headless window answers "no" to a question and cancels a file dialog
// (so a test never blocks on a dialog); a visible window shows them.

// dialogAnswer is a queued answer: kind "message" with value "yes", "no"
// or "cancel"; kind "open" or "save" with a path, or "" to cancel.
type dialogAnswer struct{ kind, value string }

func (a *App) nextAnswer(kind string) (string, bool) {
	for i, d := range a.answers {
		if d.kind == kind {
			a.answers = append(a.answers[:i], a.answers[i+1:]...)
			return d.value, true
		}
	}
	return "", false
}

// ask asks a yes / no / cancel question.
func (a *App) ask(text string) int {
	if v, ok := a.nextAnswer("message"); ok {
		switch v {
		case "yes":
			return win.IDYes
		case "cancel":
			return win.IDCancel
		}
		return win.IDNo
	}
	if a.headless {
		return win.IDNo
	}
	return a.w.Message("ScottiCAD", text, win.MsgYesNoCancel)
}

// openFile asks for a file to open; ok is false when cancelled.
func (a *App) openFile(title string, filter ...string) (string, bool) {
	if v, ok := a.nextAnswer("open"); ok {
		return v, v != ""
	}
	if a.headless {
		return "", false
	}
	return a.w.OpenFile(title, filter...)
}

// saveFile asks where to save; ok is false when cancelled.
func (a *App) saveFile(title, initial, defExt string, filter ...string) (string, bool) {
	if v, ok := a.nextAnswer("save"); ok {
		return v, v != ""
	}
	if a.headless {
		return "", false
	}
	return a.w.SaveFile(title, initial, defExt, filter...)
}
