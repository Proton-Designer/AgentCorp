package ui

import (
	"strings"
	"testing"
)

func TestBootRevealsAndFits(t *testing.T) {
	defer func(c bool) { colorEnabled = c }(colorEnabled)
	colorEnabled = false

	const w, h = 80, 24
	early := renderBoot(3, w, h)
	late := renderBoot(bootDuration, w, h)

	if early == late {
		t.Errorf("boot should animate — early and final frames must differ")
	}
	// By the end the full wordmark is present (spaced).
	if !strings.Contains(late, "A G E N T C O R P") {
		t.Errorf("final boot frame should show the full wordmark:\n%s", late)
	}
	// The tagline lands by the end.
	if !strings.Contains(late, "command") {
		t.Errorf("final boot frame should show the tagline:\n%s", late)
	}
	// No line may exceed the width, at any frame.
	for f := 0; f <= bootDuration; f++ {
		for _, ln := range strings.Split(renderBoot(f, w, h), "\n") {
			if vw := visibleWidth(ln); vw > w {
				t.Fatalf("boot frame %d has a line of width %d > %d: %q", f, vw, w, ln)
			}
		}
	}
}

func TestBootDeterministic(t *testing.T) {
	if renderBoot(7, 80, 24) != renderBoot(7, 80, 24) {
		t.Errorf("renderBoot must be a pure function of its inputs")
	}
}

func TestBootNoANSIWhenColorOff(t *testing.T) {
	defer func(c bool) { colorEnabled = c }(colorEnabled)
	colorEnabled = false
	if strings.Contains(renderBoot(10, 80, 24), "\x1b") {
		t.Errorf("no ESC bytes when colour is disabled")
	}
}

func TestVisibleWidthIgnoresANSI(t *testing.T) {
	// wrapANSI adds escapes that must not count toward width.
	defer func(c bool) { colorEnabled = c }(colorEnabled)
	colorEnabled = true
	defer func(i int) { currentTheme = i }(currentTheme)
	currentTheme = 0
	got := visibleWidth(wrapANSI("hello", styActive))
	if got != 5 {
		t.Errorf("visibleWidth should count 5 cells, ignoring ANSI, got %d", got)
	}
}
