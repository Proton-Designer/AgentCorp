package ui

import (
	"strings"

	"github.com/Proton-Designer/AgentCorp/internal/anim"
)

// Boot sequence (F7). A brief cinematic reveal on launch, driven by the frame
// clock. It is pure branding — deliberately NO faked diagnostics: it never
// prints a "connecting… OK" line that would imply a real check ran, because the
// honesty rule that governs the live view governs the splash too. It draws the
// wordmark in, states the tagline, fills a purely-decorative progress bar, then
// hands off to the live chart. Any keypress skips straight to the chart.

// bootDuration is the boot animation length in frames (~2.2s at FrameInterval).
const bootDuration = 22

// bootWord is the wordmark, revealed a letter at a time.
var bootWord = []rune("AGENTCORP")

const bootTagline = "a company of agents you can see and command"

// bootInner is the interior width of the wordmark box.
const bootInner = 25

// renderBoot draws one boot frame, centered in width×height. frame runs 0..
// bootDuration; past the end it renders the fully-assembled splash (the model
// flips to the chart on the same tick, so this is only ever the last held frame).
func renderBoot(frame, width, height int) string {
	// Wordmark types in: one letter roughly every frame after a short beat.
	typed := frame - 2
	if typed < 0 {
		typed = 0
	}
	if typed > len(bootWord) {
		typed = len(bootWord)
	}
	var word strings.Builder
	for i := range bootWord {
		if i > 0 {
			word.WriteByte(' ')
		}
		if i < typed {
			word.WriteRune(bootWord[i])
		} else {
			word.WriteByte(' ') // keep the slot's width so the mark doesn't jump
		}
	}
	// A blinking cursor sits at the typing head while letters are still landing.
	mark := word.String()
	if typed < len(bootWord) && frame%2 == 0 {
		mark = strings.TrimRight(word.String(), " ") + "_"
	}

	// Tagline reveals a few characters per frame, after the wordmark starts, and
	// completes a beat before the splash ends so it's readable at handoff.
	tn := (frame - 9) * 4
	tag := ""
	if tn > 0 {
		r := []rune(bootTagline)
		if tn > len(r) {
			tn = len(r)
		}
		tag = string(r[:tn])
	}

	// Decorative progress bar — a launch flourish, not a real measurement.
	barW := bootInner
	fill := frame * barW / bootDuration
	if fill > barW {
		fill = barW
	}
	bar := strings.Repeat("█", fill) + strings.Repeat("░", barW-fill)
	status := "initializing"
	if frame >= bootDuration-2 {
		status = "ready"
	}

	var lines []string
	lines = append(lines,
		bootScan(frame, bootInner+2, styActive), // animated top border
		bootBox("mid", "", styActive),
		bootBox("mid", centerIn(mark, bootInner), styActive),
		bootBox("mid", "", styActive),
		bootBox("bot", "", styActive),
		"",
		wrapANSI(centerIn(tag, bootInner+2), styQuiet),
		"",
		wrapANSI(centerIn(bar, bootInner), styActive),
		wrapANSI(centerIn("> "+status, bootInner), styQuiet),
	)

	// Centre the block vertically, and each line horizontally, within width.
	var b strings.Builder
	top := (height - len(lines)) / 2
	if top < 0 {
		top = 0
	}
	for i := 0; i < top; i++ {
		b.WriteByte('\n')
	}
	for i, ln := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(centerPad(width, visibleWidth(ln)) + ln)
	}
	return b.String()
}

// bootBox renders a mid or bottom row of the wordmark box; the mid row holds
// centered content. (The top border is drawn by bootScan for its moving cue.)
func bootBox(kind, content string, style cellStyle) string {
	if kind == "bot" {
		return wrapANSI("╰"+strings.Repeat("─", bootInner+2)+"╯", style)
	}
	body := content
	if w := visibleWidth(content); w < bootInner {
		body = content + strings.Repeat(" ", bootInner-w)
	}
	return wrapANSI("│", style) + " " + body + " " + wrapANSI("│", style)
}

// centerIn centers s within n cells (rune-based), returning exactly n runes wide
// (or s unchanged if already wider).
func centerIn(s string, n int) string {
	w := len([]rune(s))
	if w >= n {
		return s
	}
	lp := (n - w) / 2
	rp := n - w - lp
	return strings.Repeat(" ", lp) + s + strings.Repeat(" ", rp)
}

// visibleWidth is the cell width of s ignoring ANSI escapes, so centred boot
// lines align whether or not colour is on.
func visibleWidth(s string) int {
	n, inEsc := 0, false
	for _, r := range s {
		switch {
		case r == 0x1b:
			inEsc = true
		case inEsc:
			if r == 'm' {
				inEsc = false
			}
		default:
			n++
		}
	}
	return n
}

// bootScan returns the top-border string with a single bright scanning cell, a
// small motion cue while the splash assembles.
func bootScan(frame, inner int, style cellStyle) string {
	pos := anim.Along(frame, 10, inner)
	base := []rune(strings.Repeat("─", inner))
	left := wrapANSI("╭"+string(base[:pos]), style)
	head := wrapANSI("┈", style)
	right := wrapANSI(string(base[pos+1:])+"╮", style)
	if pos+1 > len(base) {
		right = wrapANSI("╮", style)
	}
	return left + head + right
}
