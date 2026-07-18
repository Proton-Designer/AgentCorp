package ui

import (
	"fmt"
	"strings"
)

// feedPage is how many messages the activity feed shows at once.
const feedPage = 12

// renderFeed draws the scrollable company activity feed: every message in the
// company's traffic, newest first, with peers resolved to their node names.
// The list is already company-scoped (m.live.msgs is filtered per tick), so
// this shows this company's conversation, not the whole machine's.
func (m Model) renderFeed() string {
	if m.live == nil {
		return "  (no live session)\n"
	}
	msgs := m.live.msgs
	names := m.peerNameIndex()

	var b strings.Builder
	total := len(msgs)
	// Clamp the scroll offset so it can't run past the end.
	maxOffset := total - feedPage
	if maxOffset < 0 {
		maxOffset = 0
	}
	off := m.feedOffset
	if off > maxOffset {
		off = maxOffset
	}

	shown := 0
	header := fmt.Sprintf("  activity — %d messages · ↑↓ scroll · esc close", total)
	b.WriteString(header + "\n")
	if total == 0 {
		b.WriteString("  (no messages yet)\n")
		return b.String()
	}
	// Newest first: walk from the end, skipping `off` most-recent entries.
	for i := total - 1 - off; i >= 0 && shown < feedPage; i-- {
		mm := msgs[i]
		from := feedName(names, mm.FromID)
		to := feedName(names, mm.ToID)
		line := fmt.Sprintf("  %s  %s → %s: %s", feedTime(mm.SentAt), from, to, mm.Text)
		b.WriteString(truncate(strings.TrimRight(line, " "), m.width-2) + "\n")
		shown++
	}
	return b.String()
}

// peerNameIndex maps peer ids to the node name that carries them, so the feed
// reads "ceo → worker" instead of raw peer ids.
func (m Model) peerNameIndex() map[string]string {
	out := map[string]string{msgSenderConsole: "you"}
	if m.live == nil {
		return out
	}
	nodes, err := m.live.st.ListNodes()
	if err != nil {
		return out
	}
	for _, n := range nodes {
		if n.PeerID != "" {
			out[n.PeerID] = n.Name
		}
	}
	return out
}

// feedName resolves a peer id to a name, falling back to a short id.
func feedName(names map[string]string, id string) string {
	if n, ok := names[id]; ok {
		return n
	}
	return shortPeerID(id)
}

// feedTime renders a broker timestamp as HH:MM, or a short raw fallback.
func feedTime(sentAt string) string {
	if t, ok := parseUITime(sentAt); ok {
		return t.Local().Format("15:04")
	}
	if len(sentAt) >= 5 {
		return sentAt[:5]
	}
	return "--:--"
}
