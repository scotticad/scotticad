//go:build linux

package window

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The dialogs are driven through zenity or kdialog, stood in for here by
// scripts that answer as the FAKE_* variables say and log their arguments
// to FAKE_LOG. PATH holds only the fake, so the real tools (and the other
// fake) are never found.

const fakeZenity = `#!/bin/sh
printf '%s\n' "$@" >> "$FAKE_LOG"
case " $* " in
  *" --file-selection "*)
    [ -n "$FAKE_FILE" ] || exit 1
    echo "$FAKE_FILE"
    exit 0 ;;
  *" --question "*)
    case "$FAKE_ANSWER" in
      yes) exit 0 ;;
      no) case " $* " in *" --extra-button=No "*) echo No ;; esac; exit 1 ;;
      *) exit 1 ;;
    esac ;;
  *) exit 0 ;;
esac
`

const fakeKdialog = `#!/bin/sh
printf '%s\n' "$@" >> "$FAKE_LOG"
case " $* " in
  *" --yesnocancel "*|*" --yesno "*) exit "${FAKE_CODE:-2}" ;;
  *) exit 0 ;;
esac
`

// install puts a fake tool of that name alone on PATH and returns the log
// file it writes to.
func install(t *testing.T, name, script string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "log")
	t.Setenv("PATH", dir)
	t.Setenv("FAKE_LOG", log)
	t.Setenv("FAKE_ANSWER", "")
	t.Setenv("FAKE_FILE", "")
	t.Setenv("FAKE_CODE", "")
	return log
}

func logged(t *testing.T, log string) string {
	t.Helper()
	b, _ := os.ReadFile(log)
	return string(b)
}

// "Save changes?" through zenity: Yes and Cancel are the dialog's own
// buttons, No the extra one, so Esc or closing the box means Cancel and
// nothing is discarded by accident.
func TestZenityMessages(t *testing.T) {
	log := install(t, "zenity", fakeZenity)
	w := &Window{}
	for answer, want := range map[string]int{"yes": IDYes, "no": IDNo, "closed": IDCancel} {
		t.Setenv("FAKE_ANSWER", answer)
		if got := w.Message("ScottiCAD", "Save changes?", MsgYesNoCancel); got != want {
			t.Errorf("Yes/No/Cancel, %s: %d, want %d", answer, got, want)
		}
	}
	if !strings.Contains(logged(t, log), "--extra-button=No") || !strings.Contains(logged(t, log), "--no-markup") {
		t.Errorf("the question's arguments: %q", logged(t, log))
	}
	for answer, want := range map[string]int{"yes": IDYes, "no": IDNo, "closed": IDNo} {
		t.Setenv("FAKE_ANSWER", answer)
		if got := w.Message("ScottiCAD", "Really?", MsgYesNo); got != want {
			t.Errorf("Yes/No, %s: %d, want %d", answer, got, want)
		}
	}
	if got := w.Message("ScottiCAD", "Done.", MsgOK); got != IDOK {
		t.Errorf("OK box: %d", got)
	}
}

// kdialog answers Yes/No/Cancel with its exit code.
func TestKdialogMessages(t *testing.T) {
	log := install(t, "kdialog", fakeKdialog)
	w := &Window{}
	for code, want := range map[string]int{"0": IDYes, "1": IDNo, "2": IDCancel} {
		t.Setenv("FAKE_CODE", code)
		if got := w.Message("ScottiCAD", "Save changes?", MsgYesNoCancel); got != want {
			t.Errorf("Yes/No/Cancel, exit %s: %d, want %d", code, got, want)
		}
	}
	if !strings.Contains(logged(t, log), "--yesnocancel") {
		t.Errorf("the question's arguments: %q", logged(t, log))
	}
	for code, want := range map[string]int{"0": IDYes, "1": IDNo} {
		t.Setenv("FAKE_CODE", code)
		if got := w.Message("ScottiCAD", "Really?", MsgYesNo); got != want {
			t.Errorf("Yes/No, exit %s: %d, want %d", code, got, want)
		}
	}
}

// With neither tool the answer is the safe one: Cancel, or No.
func TestMessageWithoutATool(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	w := &Window{}
	if got := w.Message("ScottiCAD", "Save changes?", MsgYesNoCancel); got != IDCancel {
		t.Errorf("Yes/No/Cancel: %d, want Cancel", got)
	}
	if got := w.Message("ScottiCAD", "Really?", MsgYesNo); got != IDNo {
		t.Errorf("Yes/No: %d, want No", got)
	}
	if got := w.Message("ScottiCAD", "Done.", MsgOK); got != IDOK {
		t.Errorf("OK: %d", got)
	}
}

// The zenity file chooser: the chosen path comes back, a cancel is ok
// false, the filters are passed, and saving over a file asks first (the
// same way every time, whatever zenity's own version does).
func TestZenityFileDialog(t *testing.T) {
	log := install(t, "zenity", fakeZenity)
	filters := parseFilters([]string{"ScottiCAD designs (*.scotticad)", "*.scotticad", "All files", "*.*"})
	dir := t.TempDir()
	chosen := filepath.Join(dir, "part.scotticad")

	t.Setenv("FAKE_FILE", chosen)
	if p, ok := zenityDialog("zenity", false, "Open design", "", filters); !ok || p != chosen {
		t.Errorf("open: %q %v", p, ok)
	}
	if l := logged(t, log); !strings.Contains(l, "--file-filter=ScottiCAD designs (*.scotticad) | *.scotticad") || !strings.Contains(l, "--file-filter=All files | *.*") {
		t.Errorf("filters: %q", l)
	}
	t.Setenv("FAKE_FILE", "")
	if p, ok := zenityDialog("zenity", false, "Open design", "", filters); ok || p != "" {
		t.Errorf("cancelled open: %q %v", p, ok)
	}

	// saving to a new path asks nothing more
	os.Remove(log)
	t.Setenv("FAKE_FILE", chosen)
	if p, ok := zenityDialog("zenity", true, "Save design", "design.scotticad", filters); !ok || p != chosen {
		t.Errorf("save: %q %v", p, ok)
	}
	if l := logged(t, log); strings.Contains(l, "--question") || !strings.Contains(l, "--save") || !strings.Contains(l, "--filename=design.scotticad") {
		t.Errorf("save's arguments: %q", l)
	}
	// over a file that exists: replaced only on Yes
	if err := os.WriteFile(chosen, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	for answer, wantOK := range map[string]bool{"yes": true, "closed": false} {
		os.Remove(log)
		t.Setenv("FAKE_ANSWER", answer)
		p, ok := zenityDialog("zenity", true, "Save design", "", filters)
		if ok != wantOK || (ok && p != chosen) {
			t.Errorf("overwrite, %s: %q %v", answer, p, ok)
		}
		if l := logged(t, log); !strings.Contains(l, "--question") || !strings.Contains(l, "part.scotticad already exists") {
			t.Errorf("overwrite question, %s: %q", answer, l)
		}
	}
}

// The kdialog file chooser is given the filters in its own form and
// starts in the home folder when no file is suggested.
func TestKdialogFileDialog(t *testing.T) {
	log := install(t, "kdialog", "#!/bin/sh\nprintf '%s\\n' \"$@\" >> \"$FAKE_LOG\"\necho /tmp/x.scotticad\n")
	filters := parseFilters([]string{"ScottiCAD designs (*.scotticad)", "*.scotticad"})
	t.Setenv("HOME", "/home/someone")
	if p, ok := kdialogDialog("kdialog", false, "Open design", "", filters); !ok || p != "/tmp/x.scotticad" {
		t.Errorf("open: %q %v", p, ok)
	}
	l := logged(t, log)
	if !strings.Contains(l, "--getopenfilename\n/home/someone\n*.scotticad|ScottiCAD designs (*.scotticad)") {
		t.Errorf("arguments: %q", l)
	}
}
