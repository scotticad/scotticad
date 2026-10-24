package app

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/scotticad/scotticad/pkg/script"
)

var updateAPIDoc = flag.Bool("update", false, "rewrite docs/api.md's method reference from the API's method table")

const apiDocPath = "../../docs/api.md"

// apiDocMarker starts docs/api.md's generated part: everything after it
// is made from the method table.
const apiDocMarker = "<!-- Everything below is generated from pkg/app/api.go: go test ./pkg/app -run TestAPIDoc -update -->"

// docs/api.md's method reference is the API's method table, as describe
// returns it: run with -update after changing a method.
func TestAPIDoc(t *testing.T) {
	data, err := os.ReadFile(apiDocPath)
	if err != nil {
		t.Fatal(err)
	}
	doc := strings.ReplaceAll(string(data), "\r\n", "\n")
	i := strings.Index(doc, apiDocMarker)
	if i < 0 {
		t.Fatalf("%s has no generated part (the marker %q)", apiDocPath, apiDocMarker)
	}
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatal(err)
	}
	want := doc[:i] + apiDocMarker + "\n\n" + apiReference(string(src))
	if want == doc {
		return
	}
	if *updateAPIDoc {
		if err := os.WriteFile(apiDocPath, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Errorf("%s's method reference is out of date: run go test ./pkg/app -run TestAPIDoc -update", apiDocPath)
}

// apiReference is the method reference: the methods in api.go's sections,
// each with the Python client's call and its parameters.
func apiReference(src string) string {
	// the sections, from the "// --- name" comments in the method table
	section := map[string]string{}
	cur := ""
	head := regexp.MustCompile(`^\s*// --- (.+)$`)
	entry := regexp.MustCompile(`^\s*\{Name: "([a-z_0-9]+)"`)
	for _, line := range strings.Split(src, "\n") {
		if m := head.FindStringSubmatch(line); m != nil {
			cur = m[1]
		} else if m := entry.FindStringSubmatch(line); m != nil && cur != "" {
			section[m[1]] = cur
		}
	}
	// the Python client's methods for each API method
	client := map[string][]script.Method{}
	for _, m := range script.Methods {
		client[m.API] = append(client[m.API], m)
	}

	var b strings.Builder
	b.WriteString("## Methods\n\n")
	var order []string
	for _, m := range apiMethods {
		if len(order) == 0 || order[len(order)-1] != section[m.Name] {
			order = append(order, section[m.Name])
		}
	}
	for _, s := range order {
		var names []string
		for _, m := range apiMethods {
			if section[m.Name] == s {
				names = append(names, fmt.Sprintf("[`%s`](#%s)", m.Name, m.Name))
			}
		}
		fmt.Fprintf(&b, "- **%s**: %s\n", sectionTitle(s), strings.Join(names, ", "))
	}
	last := "\x00"
	for _, m := range apiMethods {
		if s := section[m.Name]; s != last {
			fmt.Fprintf(&b, "\n### %s\n", sectionTitle(s))
			last = s
		}
		fmt.Fprintf(&b, "\n#### `%s`\n\n%s\n", m.Name, mdText(m.Doc))
		switch cs := client[m.Name]; {
		case len(cs) > 0:
			for _, c := range cs {
				fmt.Fprintf(&b, "\nPython: `cad.%s`", pythonCall(c))
				if c.Result != "" {
					fmt.Fprintf(&b, " returns the reply's `%s`", c.Result)
				} else if c.None {
					b.WriteString(" returns nothing")
				}
				b.WriteString("\n")
			}
		case script.NotInScripts[m.Name] != "":
			fmt.Fprintf(&b, "\nPython: `cad.%s(...)` (not in scripts: %s)\n", m.Name, script.NotInScripts[m.Name])
		default:
			fmt.Fprintf(&b, "\nPython: `cad.call(%q, ...)`\n", m.Name)
		}
		if len(m.Params) > 0 {
			b.WriteString("\n| Parameter | Type | | Meaning |\n|---|---|---|---|\n")
			for _, p := range m.Params {
				need := "required"
				if p.Optional {
					need = "optional"
				}
				fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", p.Name, mdText(p.Type), need, mdText(p.Doc))
			}
		}
	}
	return b.String()
}

func sectionTitle(s string) string {
	if s == "" {
		return "Other"
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// mdText is text for a Markdown table cell or paragraph.
func mdText(s string) string { return strings.ReplaceAll(s, "|", `\|`) }

// pythonCall is a client method's call with its parameters as Python
// writes them: required ones by name, optional ones with their defaults.
func pythonCall(m script.Method) string {
	var ps []string
	for _, p := range m.Params {
		if p.Required {
			ps = append(ps, p.Name)
			continue
		}
		ps = append(ps, p.Name+"="+pythonValue(p.Default))
	}
	return m.Name + "(" + strings.Join(ps, ", ") + ")"
}

func pythonValue(v any) string {
	switch v := v.(type) {
	case nil:
		return "None"
	case bool:
		if v {
			return "True"
		}
		return "False"
	case string:
		return fmt.Sprintf("%q", v)
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%.1f", v)
		}
		return fmt.Sprint(v)
	}
	return fmt.Sprint(v)
}
