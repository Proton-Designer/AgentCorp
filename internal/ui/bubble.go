package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Proton-Designer/AgentCorp/internal/anim"
	"github.com/Proton-Designer/AgentCorp/internal/vitals"
)

// Speech bubble (F2). The selected agent gets a bubble showing what it actually
// said last — its most recent broker message, with the real age of that message
// in the title so a bubble appearing NOW can never imply the agent is speaking
// now. Its self-summary (set_summary) rides the bottom border, dimmed and marked
// with a ~, because the broker carries no timestamp for a summary: we cannot
// claim it is fresh, so we never render it as a live quote. This is the honest
// core of the "agents talking" idea — real words, truthfully aged.

// bubbleMaxW bounds the bubble so a long message can't stretch the footer across
// a wide terminal; the content is truncated to fit.
const bubbleMaxW = 64

// speechAgeStale is when a last message is old enough that the bubble should read
// as history, not conversation — past it the title says "quiet for …" instead of
// framing the quote as recent.
const speechAgeStale = 90 * time.Second

// latestUtterance returns the text and age of the most recent message this peer
// SENT (never received — a message to a peer says nothing about its own voice).
func (m Model) latestUtterance(peerID string) (text string, age time.Duration, ok bool) {
	if m.live == nil || peerID == "" {
		return "", 0, false
	}
	now := time.Now()
	for i := len(m.live.msgs) - 1; i >= 0; i-- {
		msg := m.live.msgs[i]
		if msg.FromID != peerID {
			continue
		}
		t, parsed := parseMsgTime(msg.SentAt)
		if !parsed {
			return msg.Text, 0, true
		}
		age := now.Sub(t)
		if age < 0 {
			age = 0
		}
		return msg.Text, age, true
	}
	return "", 0, false
}

// summaryFor returns a peer's self-reported summary, or "".
func (m Model) summaryFor(peerID string) string {
	if m.live == nil {
		return ""
	}
	for _, p := range m.live.peers {
		if p.ID == peerID {
			return p.Summary
		}
	}
	return ""
}

// renderSpeechBubble draws the selected agent's bubble, or "" when there's no
// live selection. It sits where the plain "selected: X" line used to, turning a
// bare label into the agent's actual voice.
func (m Model) renderSpeechBubble() string {
	if m.live == nil {
		return ""
	}
	sel := m.selected()
	if sel == nil {
		return ""
	}
	name := sel.ID
	peerID := m.live.nameToPeer[name]
	status := m.statusMap()[name]

	quote, age, hasMsg := m.latestUtterance(peerID)
	summary := summaryOneLine(m.summaryFor(peerID))

	inner := bubbleMaxW
	if w := m.width - 4; w < inner {
		inner = w
	}
	if inner < 12 {
		return "  " + name // too narrow for a bubble; degrade to a label
	}

	// Title: who's speaking + the honest age of what they said.
	title := name
	switch {
	case !hasMsg:
		title += " · no messages yet"
	case age >= speechAgeStale:
		title += " · quiet for " + shortAge(age)
	default:
		title += " · said " + shortAge(age) + " ago"
	}

	// Body: the real quote (or a gentle placeholder when the agent hasn't spoken).
	// ASCII quotes only — curly quotes are East-Asian-Ambiguous and would shear
	// the box width on a terminal that renders them wide.
	body := "- quiet -"
	if hasMsg {
		body = "\"" + quote + "\""
	} else if summary != "" {
		body = "~ " + summary // nothing said yet: show what it says it's doing, marked as such
	}

	border := styleForStatus(status)
	// Resolve the border colour once; an active agent's bubble breathes on the
	// same honest signal as its LED, so the bubble and the card pulse together.
	borderANSI := ""
	if colorEnabled {
		borderANSI = ansiFor(border)
		if m.motion.animates() && status == vitals.StatusActive {
			lvl := anim.Level(anim.Pulse(m.frame, ledBreathPeriod), ledBreathLevels)
			if a := dimANSI(border, lvl); a != "" {
				borderANSI = a
			}
		}
	}

	var b strings.Builder
	writeBubbleLine(&b, "top", title, inner, borderANSI)
	writeBubbleLine(&b, "mid", body, inner, borderANSI)
	// Bottom border carries the dim self-summary (no freshness claimed).
	sub := ""
	if summary != "" && hasMsg {
		sub = "~ " + summary
	}
	writeBubbleLine(&b, "bot", sub, inner, borderANSI)
	return b.String()
}

// writeBubbleLine emits one rounded-box row. kind is "top"/"mid"/"bot". Text is
// truncated to the inner width; top/bot embed the text in the border rule, mid
// pads it between verticals. borderANSI is the pre-resolved escape ("" = none).
func writeBubbleLine(b *strings.Builder, kind, text string, inner int, borderANSI string) {
	open, close := "", ""
	if borderANSI != "" {
		open, close = borderANSI, ansiReset
	}
	// Every row is exactly inner+2 cells wide: an edge glyph, an `inner`-wide
	// interior (spaces for the body, ─ rule for the borders), and an edge glyph.
	b.WriteString("  ")
	switch kind {
	case "mid":
		t := fitRunes(text, inner-1) // leave the leading space
		interior := " " + t + strings.Repeat(" ", inner-1-len([]rune(t)))
		b.WriteString(open + "│" + close)
		b.WriteString(interior)
		b.WriteString(open + "│" + close)
	default:
		lc, rc := "╭", "╮"
		if kind == "bot" {
			lc, rc = "╰", "╯"
		}
		label := ""
		if text != "" {
			label = " " + fitRunes(text, inner-3) + " "
		}
		rule := "─" + label + strings.Repeat("─", inner-1-len([]rune(label)))
		b.WriteString(open + lc + rule + rc + close)
	}
	b.WriteByte('\n')
}

// summaryOneLine flattens a possibly multi-line summary to a single spaced line.
func summaryOneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// fitRunes truncates to n runes with an ellipsis, rune-safe.
func fitRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

// shortAge renders a compact relative duration: "3s", "4m", "2h".
func shortAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
}
