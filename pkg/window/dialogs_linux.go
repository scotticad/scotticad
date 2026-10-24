//go:build linux

package window

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// Linux has no system file dialog of its own: they belong to the desktop's
// toolkit. ScottiCAD asks the desktop for its own through the
// xdg-desktop-portal (GNOME's dialog on GNOME, KDE's on KDE), and falls
// back to zenity or kdialog where there is no portal. Message boxes use
// zenity or kdialog.

// filterSpec is one file-type filter: a description and its patterns.
type filterSpec struct {
	name     string
	patterns []string // "*.scotticad", ...
}

// parseFilters turns the Windows-style pairs ("Description", "*.a;*.b",
// ...) into filters.
func parseFilters(filter []string) []filterSpec {
	var out []filterSpec
	for i := 0; i+1 < len(filter); i += 2 {
		var pats []string
		for _, p := range strings.Split(filter[i+1], ";") {
			if p = strings.TrimSpace(p); p != "" {
				pats = append(pats, p)
			}
		}
		out = append(out, filterSpec{filter[i], pats})
	}
	return out
}

func (w *Window) OpenFile(title string, filter ...string) (string, bool) {
	return w.fileDialog(false, title, "", "", parseFilters(filter))
}

// SaveFile asks for a path to save to; initial is the suggested file name
// (or path), defExt the extension added when the user types none.
func (w *Window) SaveFile(title, initial, defExt string, filter ...string) (string, bool) {
	p, ok := w.fileDialog(true, title, initial, defExt, parseFilters(filter))
	if ok && defExt != "" && filepath.Ext(p) == "" {
		p += "." + strings.TrimPrefix(defExt, ".")
	}
	return p, ok
}

func (w *Window) fileDialog(save bool, title, initial, defExt string, filters []filterSpec) (string, bool) {
	p, ok, err := w.portalDialog(save, title, initial, filters)
	if err == nil {
		return p, ok
	}
	if path, err := exec.LookPath("zenity"); err == nil {
		return zenityDialog(path, save, title, initial, filters)
	}
	if path, err := exec.LookPath("kdialog"); err == nil {
		return kdialogDialog(path, save, title, initial, filters)
	}
	// (with neither tool, Message prints this on stderr)
	w.Message("ScottiCAD", "ScottiCAD can't show a file dialog: install xdg-desktop-portal (with your desktop's backend), zenity or kdialog.", MsgOK)
	return "", false
}

var errNoPortal = errors.New("no desktop portal")

// portalDialog shows the desktop's file chooser through
// org.freedesktop.portal.FileChooser. err is set when the portal isn't
// available (so another way is tried); a cancelled dialog is ok false.
func (w *Window) portalDialog(save bool, title, initial string, filters []filterSpec) (string, bool, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return "", false, errNoPortal
	}
	defer conn.Close()
	token := fmt.Sprintf("scotticad%d", time.Now().UnixNano())
	sender := strings.ReplaceAll(strings.TrimPrefix(conn.Names()[0], ":"), ".", "_")
	handle := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/" + sender + "/" + token)
	signals := make(chan *dbus.Signal, 4)
	conn.Signal(signals)
	if err := conn.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.portal.Request"), dbus.WithMatchMember("Response")); err != nil {
		return "", false, errNoPortal
	}

	type rule struct {
		Kind    uint32
		Pattern string
	}
	type filt struct {
		Name  string
		Rules []rule
	}
	var fs []filt
	for _, f := range filters {
		var rs []rule
		for _, p := range f.patterns {
			rs = append(rs, rule{0, p}) // 0: a glob pattern
		}
		fs = append(fs, filt{f.name, rs})
	}
	opts := map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(token),
		"modal":        dbus.MakeVariant(true),
	}
	if len(fs) > 0 {
		opts["filters"] = dbus.MakeVariant(fs)
		opts["current_filter"] = dbus.MakeVariant(fs[0])
	}
	method := "OpenFile"
	if save {
		method = "SaveFile"
		if initial != "" {
			opts["current_name"] = dbus.MakeVariant(filepath.Base(initial))
			if dir := filepath.Dir(initial); dir != "." && filepath.IsAbs(dir) {
				opts["current_folder"] = dbus.MakeVariant(append([]byte(dir), 0))
			}
		}
	}
	parent := fmt.Sprintf("x11:%x", w.xwin)
	obj := conn.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop")
	call := obj.Call("org.freedesktop.portal.FileChooser."+method, 0, parent, title, opts)
	if call.Err != nil {
		return "", false, errNoPortal
	}
	var got dbus.ObjectPath
	if err := call.Store(&got); err == nil && got != "" {
		handle = got // older portals choose the request path themselves
	}
	for s := range signals {
		if s.Path != handle || len(s.Body) < 2 {
			continue
		}
		code, _ := s.Body[0].(uint32)
		results, _ := s.Body[1].(map[string]dbus.Variant)
		switch code {
		case 1:
			return "", false, nil // cancelled by the user
		case 2:
			// ended some other way: the portal runs but its backend has no
			// working file chooser (a desktop without one, a broken
			// session), so no dialog was shown; another way is tried
			return "", false, errNoPortal
		}
		var uris []string
		if v, ok := results["uris"]; ok {
			uris, _ = v.Value().([]string)
		}
		if len(uris) == 0 {
			return "", false, nil
		}
		u, err := url.Parse(uris[0])
		if err != nil || u.Scheme != "file" {
			return "", false, nil
		}
		return u.Path, true, nil
	}
	return "", false, errNoPortal
}

func zenityDialog(zenity string, save bool, title, initial string, filters []filterSpec) (string, bool) {
	args := []string{"--file-selection", "--title=" + title}
	if save {
		args = append(args, "--save")
		if initial != "" {
			args = append(args, "--filename="+initial)
		}
	}
	for _, f := range filters {
		args = append(args, "--file-filter="+f.name+" | "+strings.Join(f.patterns, " "))
	}
	out, err := exec.Command(zenity, args...).Output()
	if err != nil {
		return "", false
	}
	p := strings.TrimRight(string(out), "\r\n")
	if save && p != "" {
		if _, err := os.Stat(p); err == nil {
			// zenity's own overwrite question differs between versions:
			// ask here, the same way every time
			if exec.Command(zenity, "--question", "--title="+title, "--no-markup",
				"--text="+filepath.Base(p)+" already exists. Replace it?").Run() != nil {
				return "", false
			}
		}
	}
	return p, p != ""
}

func kdialogDialog(kdialog string, save bool, title, initial string, filters []filterSpec) (string, bool) {
	var fl []string
	for _, f := range filters {
		fl = append(fl, strings.Join(f.patterns, " ")+"|"+f.name)
	}
	start := initial
	if start == "" {
		start, _ = os.UserHomeDir()
	}
	op := "--getopenfilename"
	if save {
		op = "--getsavefilename"
	}
	out, err := exec.Command(kdialog, "--title", title, op, start, strings.Join(fl, "\n")).Output()
	if err != nil {
		return "", false
	}
	p := strings.TrimRight(string(out), "\r\n")
	return p, p != ""
}

// Message shows a message box. kind: MsgOK, MsgYesNo or MsgYesNoCancel.
// Returns IDOK, IDYes, IDNo or IDCancel. Closing the box is Cancel (No for
// a Yes/No box), so a closed "Save changes?" never discards anything.
// Without zenity or kdialog the text goes to stderr and the answer is
// Cancel.
func (w *Window) Message(title, text string, kind int) int {
	if path, err := exec.LookPath("zenity"); err == nil {
		return zenityMessage(path, title, text, kind)
	}
	if path, err := exec.LookPath("kdialog"); err == nil {
		return kdialogMessage(path, title, text, kind)
	}
	fmt.Fprintf(os.Stderr, "%s: %s\n", title, text)
	switch kind {
	case MsgYesNo:
		return IDNo
	case MsgYesNoCancel:
		return IDCancel
	}
	return IDOK
}

func zenityMessage(zenity, title, text string, kind int) int {
	base := []string{"--title=" + title, "--text=" + text, "--no-markup"}
	switch kind {
	case MsgYesNo:
		if exec.Command(zenity, append([]string{"--question"}, base...)...).Run() == nil {
			return IDYes
		}
		return IDNo
	case MsgYesNoCancel:
		// Yes and Cancel are the dialog's own buttons (so Esc and closing
		// mean Cancel); No is the extra button, which prints its label
		out, err := exec.Command(zenity, append([]string{"--question", "--ok-label=Yes", "--cancel-label=Cancel",
			"--extra-button=No"}, base...)...).Output()
		switch {
		case err == nil:
			return IDYes
		case strings.TrimSpace(string(out)) == "No":
			return IDNo
		}
		return IDCancel
	}
	exec.Command(zenity, append([]string{"--info"}, base...)...).Run()
	return IDOK
}

func kdialogMessage(kdialog, title, text string, kind int) int {
	code := func(args ...string) int {
		err := exec.Command(kdialog, append([]string{"--title", title}, args...)...).Run()
		if err == nil {
			return 0
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		return -1
	}
	switch kind {
	case MsgYesNo:
		if code("--yesno", text) == 0 {
			return IDYes
		}
		return IDNo
	case MsgYesNoCancel:
		switch code("--yesnocancel", text) {
		case 0:
			return IDYes
		case 1:
			return IDNo
		}
		return IDCancel
	}
	code("--msgbox", text)
	return IDOK
}

// ErrorBox reports an error with no window of ours: on stderr, and in a
// box when zenity or kdialog is there to show one.
func ErrorBox(title, text string) {
	fmt.Fprintf(os.Stderr, "%s: %s\n", title, text)
	if os.Getenv("DISPLAY") == "" {
		return
	}
	if path, err := exec.LookPath("zenity"); err == nil {
		exec.Command(path, "--error", "--title="+title, "--text="+text, "--no-markup").Run()
	} else if path, err := exec.LookPath("kdialog"); err == nil {
		exec.Command(path, "--title", title, "--error", text).Run()
	}
}

// OpenURL opens a web address (or file) with the user's default handler.
func OpenURL(u string) {
	if cmd := exec.Command("xdg-open", u); cmd.Start() == nil {
		go cmd.Wait() // reaped in the background: xdg-open returns at once
	}
}
