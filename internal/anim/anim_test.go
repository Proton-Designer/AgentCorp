package anim

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestPhase(t *testing.T) {
	cases := []struct {
		frame, period int
		want          float64
	}{
		{0, 4, 0.0},
		{1, 4, 0.25},
		{2, 4, 0.5},
		{3, 4, 0.75},
		{4, 4, 0.0}, // wraps
		{5, 4, 0.25},
		{-1, 4, 0.75}, // negative frames wrap forward, never negative
		{7, 0, 0.0},   // guard: period 0 is a still frame, not a panic
		{7, -3, 0.0},  // guard: negative period likewise
	}
	for _, c := range cases {
		if got := Phase(c.frame, c.period); !approx(got, c.want) {
			t.Errorf("Phase(%d,%d)=%v want %v", c.frame, c.period, got, c.want)
		}
	}
}

func TestTriangle(t *testing.T) {
	// Rises 0→1 across the first half, falls 1→0 across the second.
	if got := Triangle(0, 4); !approx(got, 0) {
		t.Errorf("Triangle(0,4)=%v want 0", got)
	}
	if got := Triangle(1, 4); !approx(got, 0.5) {
		t.Errorf("Triangle(1,4)=%v want 0.5", got)
	}
	if got := Triangle(2, 4); !approx(got, 1) {
		t.Errorf("Triangle(2,4)=%v want 1 (peak at half cycle)", got)
	}
	if got := Triangle(3, 4); !approx(got, 0.5) {
		t.Errorf("Triangle(3,4)=%v want 0.5", got)
	}
}

func TestPulseEndsAreCalm(t *testing.T) {
	// The raised cosine must sit at 0 at the cycle start and 1 at the midpoint,
	// with zero slope there — that flatness is what makes breathing read organic.
	if got := Pulse(0, 8); !approx(got, 0) {
		t.Errorf("Pulse(0,8)=%v want 0", got)
	}
	if got := Pulse(4, 8); !approx(got, 1) {
		t.Errorf("Pulse(4,8)=%v want 1 at midpoint", got)
	}
	// Symmetric: quarter and three-quarter points both sit at 0.5.
	if got := Pulse(2, 8); !approx(got, 0.5) {
		t.Errorf("Pulse(2,8)=%v want 0.5", got)
	}
	if got := Pulse(6, 8); !approx(got, 0.5) {
		t.Errorf("Pulse(6,8)=%v want 0.5", got)
	}
	// Never leaves [0,1] across a full cycle.
	for f := 0; f < 64; f++ {
		v := Pulse(f, 16)
		if v < 0 || v > 1 {
			t.Fatalf("Pulse(%d,16)=%v out of [0,1]", f, v)
		}
	}
}

func TestEaseInOut(t *testing.T) {
	if got := EaseInOut(-0.5); got != 0 {
		t.Errorf("EaseInOut clamps below 0, got %v", got)
	}
	if got := EaseInOut(1.5); got != 1 {
		t.Errorf("EaseInOut clamps above 1, got %v", got)
	}
	if got := EaseInOut(0.5); !approx(got, 0.5) {
		t.Errorf("EaseInOut(0.5)=%v want 0.5 (symmetric midpoint)", got)
	}
	// Monotonic non-decreasing across the domain.
	prev := -1.0
	for i := 0; i <= 20; i++ {
		t01 := float64(i) / 20
		v := EaseInOut(t01)
		if v < prev {
			t.Fatalf("EaseInOut not monotonic at t=%v: %v < %v", t01, v, prev)
		}
		prev = v
	}
}

func TestAlong(t *testing.T) {
	// A 4-frame period over an 8-cell path visits the first cell of each quarter.
	got := []int{
		Along(0, 4, 8),
		Along(1, 4, 8),
		Along(2, 4, 8),
		Along(3, 4, 8),
	}
	want := []int{0, 2, 4, 6}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Along step %d = %d, want %d", i, got[i], want[i])
		}
	}
	// Never indexes past the last cell, across a full period at fine resolution.
	for f := 0; f < 100; f++ {
		if idx := Along(f, 100, 5); idx < 0 || idx >= 5 {
			t.Fatalf("Along(%d,100,5)=%d out of [0,5)", f, idx)
		}
	}
	if Along(3, 4, 0) != 0 {
		t.Errorf("Along with empty path must be 0")
	}
}

func TestLevel(t *testing.T) {
	// Quantising into 4 buckets: this is what keeps a card's rendered bytes stable
	// between level-crossings so the renderer's per-line diff stays effective.
	cases := []struct {
		v     float64
		steps int
		want  int
	}{
		{0.0, 4, 0},
		{0.1, 4, 0},
		{0.25, 4, 1},
		{0.5, 4, 2},
		{0.75, 4, 3},
		{1.0, 4, 3}, // clamped to top bucket, never steps
		{1.5, 4, 3}, // over-range clamps
		{-1, 4, 0},  // under-range clamps
		{0.9, 1, 0}, // one bucket is always 0
	}
	for _, c := range cases {
		if got := Level(c.v, c.steps); got != c.want {
			t.Errorf("Level(%v,%d)=%d want %d", c.v, c.steps, got, c.want)
		}
	}
}
