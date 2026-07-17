package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Proton-Designer/AgentCorp/internal/vitals"
)

// sparkChars renders throughput buckets. Deliberately ASCII-safe blocks from
// the Block Elements range: all are unambiguously single-width, unlike the
// geometric shapes (◆ ● ○) that UI-7 warns about.
var sparkChars = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// sparkline renders counts as a fixed-width bar strip.
//
// Scales to the local max, not an absolute: the shape of recent traffic is
// what an operator reads, and a fixed scale would flatten every real org into
// a single row of ▁. An all-zero series renders flat rather than dividing by
// zero.
func sparkline(counts []int) string {
	if len(counts) == 0 {
		return ""
	}
	maxv := 0
	for _, c := range counts {
		if c > maxv {
			maxv = c
		}
	}
	var b strings.Builder
	for _, c := range counts {
		if maxv == 0 {
			b.WriteRune(sparkChars[0])
			continue
		}
		idx := c * (len(sparkChars) - 1) / maxv
		b.WriteRune(sparkChars[idx])
	}
	return b.String()
}

// glyphFor maps a node status to its status glyph.
//
// NOTE (UI-7): these are geometric shapes and several are East-Asian-Ambiguous
// — they may render double-width in some terminals. Width accounting must go
// through displaywidth, never len([]rune). Tracked, not yet done.
func glyphFor(s vitals.Status) string {
	switch s {
	case vitals.StatusActive:
		return "●"
	case vitals.StatusQuiet:
		return "○"
	case vitals.StatusDead:
		return "×"
	default:
		return "·"
	}
}

// hudLine renders the vitals strip.
//
// Every field here is something the substrate can actually tell us. There is
// deliberately no working/idle breakdown: it is not derivable, and a HUD that
// reports a number it guessed is worse than one that omits it (spec §5.2).
func hudLine(s vitals.Summary, spark string, stale bool) string {
	live := "● live"
	if stale {
		live = "⚠ stale"
	}
	parts := []string{
		live,
		fmt.Sprintf("%d agents", s.Alive),
		fmt.Sprintf("%d active · %d quiet", s.Active, s.Quiet),
	}
	if s.Unmanaged > 0 {
		// Unmanaged is the common case on a real box (AD-6), so it is a
		// first-class readout, not an exception report.
		parts = append(parts, fmt.Sprintf("%d unmanaged", s.Unmanaged))
	}
	if s.Dead > 0 {
		parts = append(parts, fmt.Sprintf("%d dead", s.Dead))
	}
	if spark != "" {
		parts = append(parts, spark)
	}
	if s.Uptime > 0 {
		parts = append(parts, fmtDuration(s.Uptime))
	}
	return strings.Join(parts, "  ·  ")
}

func fmtDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("up %dh%02dm", h, m)
	}
	return fmt.Sprintf("up %dm", m)
}
