package ui

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Proton-Designer/AgentCorp/internal/layout"
	"github.com/Proton-Designer/AgentCorp/internal/vitals"
)

// demoOffice builds the 5-node shape from the spec's demo org: a CEO over
// three departments, one of which (backend) has an IC beneath it.
func demoOffice() (*layout.Node, map[string]vitals.Status) {
	intern := &layout.Node{ID: "intern", W: cardW, H: cardH}
	backend := &layout.Node{ID: "backend", W: cardW, H: cardH, Children: []*layout.Node{intern}}
	frontend := &layout.Node{ID: "frontend", W: cardW, H: cardH}
	research := &layout.Node{ID: "research", W: cardW, H: cardH}
	root := &layout.Node{ID: "CEO", W: cardW, H: cardH, Children: []*layout.Node{backend, frontend, research}}

	statuses := map[string]vitals.Status{
		"CEO":      vitals.StatusActive,
		"backend":  vitals.StatusActive,
		"intern":   vitals.StatusDead,
		"frontend": vitals.StatusQuiet,
		"research": vitals.StatusPending,
	}
	return root, statuses
}

func TestOffice_NilRoot(t *testing.T) {
	if out := renderOffice(nil, nil, 100, 30); out != "" {
		t.Fatalf("nil root: want empty string, got %q", out)
	}
}

func TestOffice_EveryAgentNameAppears(t *testing.T) {
	root, statuses := demoOffice()
	out := renderOffice(root, statuses, 100, 30)
	for _, name := range []string{"CEO", "backend", "frontend", "research", "intern"} {
		if !strings.Contains(out, name) {
			t.Errorf("expected agent %q to appear in office view, output:\n%s", name, out)
		}
	}
}

func TestOffice_DeadAgentStillAppears(t *testing.T) {
	root, statuses := demoOffice()
	out := renderOffice(root, statuses, 100, 30)
	if !strings.Contains(out, "intern") {
		t.Fatalf("dead agent %q was dropped instead of dimmed, output:\n%s", "intern", out)
	}
}

// Width checks render monochrome, matching paint_test.go's convention (see
// Render's doc comment): ANSI escapes are themselves runes, so a raw rune
// count over styled output measures escape-sequence bytes, not cells. The
// geometry is identical either way (RenderStyled and Render share buildGrid;
// joinOfficeLines clamps segment text, not the ANSI-wrapped string) — turning
// colour off just makes utf8.RuneCountInString a true cell count again.
func TestOffice_NoLineExceedsWidth(t *testing.T) {
	old := colorEnabled
	colorEnabled = false
	defer func() { colorEnabled = old }()

	root, statuses := demoOffice()
	for _, width := range []int{20, 24, 40, 60, 80, 100, 160} {
		out := renderOffice(root, statuses, width, 30)
		for i, line := range strings.Split(out, "\n") {
			if w := utf8.RuneCountInString(line); w > width {
				t.Errorf("width=%d: line %d is %d runes wide, want <= %d:\n%q", width, i, w, width, line)
			}
		}
	}
}

func TestOffice_NoAnsiWhenColorDisabled(t *testing.T) {
	old := colorEnabled
	colorEnabled = false
	defer func() { colorEnabled = old }()

	root, statuses := demoOffice()
	out := renderOffice(root, statuses, 100, 30)
	if strings.ContainsRune(out, 0x1b) {
		t.Fatalf("colorEnabled=false: output still contains ESC bytes:\n%q", out)
	}
}

func TestOffice_Deterministic(t *testing.T) {
	root, statuses := demoOffice()
	a := renderOffice(root, statuses, 100, 30)
	b := renderOffice(root, statuses, 100, 30)
	if a != b {
		t.Fatalf("renderOffice is not deterministic:\n--- a ---\n%s\n--- b ---\n%s", a, b)
	}
}

// TestOffice_TinyWindow covers the graceful-degradation path (contract §4):
// too small a canvas gets a short message, never a malformed floor plan, and
// never a panic.
func TestOffice_TinyWindow(t *testing.T) {
	root, statuses := demoOffice()
	cases := []struct{ w, h int }{
		{19, 30}, // width just under the floor
		{100, 7}, // height just under the floor
		{5, 5},   // both tiny
		{0, 30},  // zero width
		{-5, 30}, // negative width
	}
	for _, c := range cases {
		out := renderOffice(root, statuses, c.w, c.h)
		for i, line := range strings.Split(out, "\n") {
			if w := utf8.RuneCountInString(line); c.w > 0 && w > c.w {
				t.Errorf("w=%d h=%d: line %d is %d runes wide:\n%q", c.w, c.h, i, w, line)
			}
		}
		if c.w <= 0 && out != "" {
			t.Errorf("w=%d h=%d: want empty string for non-positive width, got %q", c.w, c.h, out)
		}
	}
}

// TestOffice_EmptyOrg covers a root with no departments — the "flat org"
// empty state, not just the nil-root case.
func TestOffice_EmptyOrg(t *testing.T) {
	root := &layout.Node{ID: "solo", W: cardW, H: cardH}
	out := renderOffice(root, map[string]vitals.Status{"solo": vitals.StatusActive}, 60, 20)
	if !strings.Contains(out, "solo") {
		t.Fatalf("solo root missing from empty-department office view:\n%s", out)
	}
	if out == "" {
		t.Fatal("empty-department org rendered nothing")
	}
}

// TestOffice_UnknownStatus covers an agent absent from the statuses map — the
// map lookup returns the zero value (""), which must map to a neutral
// marker/style rather than panicking or defaulting to a real status.
func TestOffice_UnknownStatus(t *testing.T) {
	root := &layout.Node{ID: "ghost", W: cardW, H: cardH}
	out := renderOffice(root, map[string]vitals.Status{}, 60, 20)
	if !strings.Contains(out, "ghost") {
		t.Fatalf("unknown-status agent missing from office view:\n%s", out)
	}
}

// TestOffice_ManyDeptsAndDesksOverflowGracefully stresses both truncation
// paths at once: more departments than a modest floor can seat, and more
// desks than a single room's grid cap. Neither may silently drop an agent —
// each must surface as a "+N" count, and no line may exceed width.
func TestOffice_ManyDeptsAndDesksOverflowGracefully(t *testing.T) {
	old := colorEnabled
	colorEnabled = false
	defer func() { colorEnabled = old }()

	var depts []*layout.Node
	for d := 0; d < 12; d++ {
		var desks []*layout.Node
		for i := 0; i < 25; i++ {
			desks = append(desks, &layout.Node{ID: nameOf(d, i), W: cardW, H: cardH})
		}
		depts = append(depts, &layout.Node{
			ID:       fmt.Sprintf("dept%d", d),
			W:        cardW,
			H:        cardH,
			Children: desks,
		})
	}
	root := &layout.Node{ID: "CEO", W: cardW, H: cardH, Children: depts}

	statuses := map[string]vitals.Status{"CEO": vitals.StatusActive}
	width, height := 80, 24
	out := renderOffice(root, statuses, width, height)

	for i, line := range strings.Split(out, "\n") {
		if w := utf8.RuneCountInString(line); w > width {
			t.Fatalf("line %d is %d runes wide, want <= %d:\n%q", i, w, width, line)
		}
	}
	if !strings.Contains(out, "more") {
		t.Errorf("expected an overflow marker ('+N more') somewhere in a 300-agent org squeezed into %dx%d, got:\n%s", width, height, out)
	}
	if lines := strings.Split(out, "\n"); len(lines) > height {
		t.Errorf("rendered %d lines, want <= height %d", len(lines), height)
	}
}

func nameOf(dept, i int) string {
	return fmt.Sprintf("d%dic%d", dept, i)
}
