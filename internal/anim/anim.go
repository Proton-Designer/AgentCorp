// Package anim holds the pure animation math for the living-company frontend.
//
// Everything here is a deterministic function of an integer frame counter — no
// wall-clock reads, no state — for the same reason layout/ and vitals/ are
// pure: a timing bug in a 10fps overlay is nearly invisible by eye, and only a
// table test pins it down. The UI owns the frame counter (it advances once per
// frame tick); this package only turns a frame number into phases, waves, and
// positions the renderer can paint with.
package anim

import "math"

// Phase returns where `frame` sits within a cycle of `period` frames, as a
// value in [0,1). period <= 0 yields 0 (a still frame), never a divide-by-zero.
func Phase(frame, period int) float64 {
	if period <= 0 {
		return 0
	}
	f := frame % period
	if f < 0 {
		f += period
	}
	return float64(f) / float64(period)
}

// Triangle is a 0→1→0 linear wave over `period` frames: it rises across the
// first half of the cycle and falls across the second. Cheap, and good enough
// for a blinking cursor or a hard-edged strobe.
func Triangle(frame, period int) float64 {
	p := Phase(frame, period)
	if p < 0.5 {
		return p * 2
	}
	return (1 - p) * 2
}

// Pulse is a smooth 0→1→0 raised-cosine wave over `period` frames. This is the
// "breathing" curve — the ease at the top and bottom of the cycle reads as an
// organic in/out rather than a mechanical bounce, which is what makes a border
// look like it's breathing instead of flashing.
func Pulse(frame, period int) float64 {
	p := Phase(frame, period)
	// (1 - cos(2πp)) / 2 sweeps 0→1→0 with zero slope at both ends.
	return (1 - math.Cos(2*math.Pi*p)) / 2
}

// EaseInOut is smoothstep on [0,1]: 3t² − 2t³. Values outside [0,1] are
// clamped so callers can pass a raw progress ratio without pre-checking.
func EaseInOut(t float64) float64 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	return t * t * (3 - 2*t)
}

// Along returns the index in [0,n) of a marker traveling a path of n discrete
// steps, advancing one full traversal every `period` frames. Used to slide a
// message dot along a connector: n is the number of cells on the path. Returns
// 0 for a degenerate path (n <= 0).
func Along(frame, period, n int) int {
	if n <= 0 {
		return 0
	}
	idx := int(Phase(frame, period) * float64(n))
	if idx >= n {
		idx = n - 1 // guard the p→1 boundary from rounding past the last cell
	}
	return idx
}

// Level quantises a continuous 0..1 wave into one of `steps` discrete buckets
// (0..steps-1). Renderers use it to pick among a small ramp of glyphs or
// brightness attributes without floating point leaking into the paint layer.
func Level(v float64, steps int) int {
	if steps <= 1 {
		return 0
	}
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return steps - 1
	}
	idx := int(v * float64(steps))
	if idx >= steps {
		idx = steps - 1
	}
	return idx
}
