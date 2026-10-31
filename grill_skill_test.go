package main

import (
	"os"
	"strings"
	"testing"
)

// The mw:grill skill must exist, name itself, use only mw commands this build has, and be linked and credited.
func TestGrillSkill(t *testing.T) {
	read := func(p string) string {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		return string(b)
	}
	g := read("plugins/mw/skills/grill/SKILL.md")
	if !strings.HasPrefix(g, "---\nname: grill\ndescription: ") {
		t.Errorf("grill frontmatter:\n%.200s", g)
	}
	for _, want := range []string{"mw answer ID --accept", "mw log -A --action decide --grep", "mw ask", "mw decide", "mw done", "--on ID", "--answerer", "Never run `mw answer`"} {
		if !strings.Contains(g, want) {
			t.Errorf("grill skill lacks %q", want)
		}
	}
	if !strings.Contains(read("plugins/mw/skills/mw/SKILL.md"), "mw:grill") {
		t.Error("the mw skill does not point to mw:grill")
	}
	if r := read("README.md"); !strings.Contains(r, "Matt Pocock") || !strings.Contains(r, "mw:grill") {
		t.Error("README must credit Matt Pocock's grilling idea next to mw:grill")
	}
}
