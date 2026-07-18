package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/store"
	"github.com/Proton-Designer/AgentCorp/internal/vitals"
)

// renderInspect draws the detail panel for the selected agent: everything the
// substrate and store actually know about it, plus derived vitals and its most
// recent traffic. Honest by construction — every line is a value we can show,
// never a guess (no "working", no invented role confidence for adopted nodes).
func (m Model) renderInspect() string {
	sel := m.selected()
	if sel == nil || m.live == nil {
		return ""
	}
	row, ok := m.nodeRowByName(sel.ID)
	if !ok {
		return "  (no record for " + sel.ID + ")\n"
	}

	status := vitals.NodeStatus(row, m.live.peers, m.live.msgs, time.Now(), ActivityWindow)
	peer, hasPeer := m.peerByID(row.PeerID)

	width := m.width - 4
	if width < 40 {
		width = 40
	}
	if width > 72 {
		width = 72
	}

	var rows []string
	add := func(label, val string) {
		if val == "" {
			return
		}
		rows = append(rows, fmt.Sprintf("  %-9s %s", label, val))
	}

	add("state", string(status)+"  ·  "+row.State)
	add("role", roleLabel(row))
	if row.PeerID != "" {
		add("peer", row.PeerID)
	} else {
		add("peer", "(unbound)")
	}
	if hasPeer {
		add("cwd", peer.CWD)
		if peer.TTY != "" {
			add("tty", peer.TTY)
		}
		if s := strings.TrimSpace(peer.Summary); s != "" {
			add("summary", "\""+truncate(s, width-12)+"\"")
		}
	} else if row.Workdir != "" {
		add("cwd", row.Workdir)
	}
	add("spawn", spawnLabel(row))
	if up := uptimeOf(row, peer, hasPeer); up != "" {
		add("uptime", up)
	}
	sent, recv := msgCounts(m.live.msgs, row.PeerID)
	add("msgs", fmt.Sprintf("%d sent · %d recv", sent, recv))

	// Recent traffic — the last few messages this agent was part of.
	if row.PeerID != "" {
		recent := recentMessages(m.live.msgs, row.PeerID, 3)
		for i, line := range recent {
			label := ""
			if i == 0 {
				label = "recent"
			}
			rows = append(rows, fmt.Sprintf("  %-9s %s", label, truncate(line, width-12)))
		}
	}

	return panel(sel.ID, styleForStatus(status), rows, width)
}

// panel wraps titled content in a rounded, status-colored box.
func panel(title string, s cellStyle, rows []string, width int) string {
	color := colorEnabled && ansiFor(s) != ""
	paint := func(text string) string {
		if !color {
			return text
		}
		return ansiFor(s) + text + ansiReset
	}

	top := "  " + paint("╭─ "+title+" "+strings.Repeat("─", max(0, width-len([]rune(title))-5))+"╮")
	bottom := "  " + paint("╰"+strings.Repeat("─", width-2)+"╯")

	var b strings.Builder
	b.WriteString(top + "\n")
	for _, r := range rows {
		b.WriteString(r + "\n")
	}
	b.WriteString(bottom + "\n")
	b.WriteString("  esc/i close · ↑↓ inspect others\n")
	return b.String()
}

// peerByID finds a live broker peer by id.
func (m Model) peerByID(id string) (broker.Peer, bool) {
	if id == "" || m.live == nil {
		return broker.Peer{}, false
	}
	for _, p := range m.live.peers {
		if p.ID == id {
			return p, true
		}
	}
	return broker.Peer{}, false
}

// roleLabel names a node's role, marking adopted nodes as guesses — we don't
// control their system prompt, so their role is our label, not their truth.
func roleLabel(n store.Node) string {
	if n.SpawnMode == "adopted" {
		return n.Role + "  (adopted — role is a guess)"
	}
	return n.Role
}

// spawnLabel describes how the node came to be.
func spawnLabel(n store.Node) string {
	switch n.SpawnMode {
	case "adopted":
		return "adopted (not ours)"
	case "":
		return ""
	default:
		if n.SpawnRef != "" {
			return n.SpawnMode + " " + n.SpawnRef
		}
		return n.SpawnMode
	}
}

// uptimeOf reports how long the node has existed, from its created_at.
func uptimeOf(n store.Node, _ broker.Peer, _ bool) string {
	t, ok := parseUITime(n.CreatedAt)
	if !ok {
		return ""
	}
	return fmtDuration(time.Since(t) + time.Minute) // reuse the HUD's "up Xm" form
}

// msgCounts returns how many messages the peer sent and received.
func msgCounts(msgs []broker.Message, peerID string) (sent, recv int) {
	if peerID == "" {
		return 0, 0
	}
	for _, mm := range msgs {
		if mm.FromID == peerID {
			sent++
		}
		if mm.ToID == peerID {
			recv++
		}
	}
	return sent, recv
}

// recentMessages returns up to n of the most recent messages involving peerID,
// newest first, each rendered as a direction arrow + counterpart + text.
func recentMessages(msgs []broker.Message, peerID string, n int) []string {
	var involved []broker.Message
	for _, mm := range msgs {
		if mm.FromID == peerID || mm.ToID == peerID {
			involved = append(involved, mm)
		}
	}
	// msgs arrive oldest-first from the store; take the tail.
	if len(involved) > n {
		involved = involved[len(involved)-n:]
	}
	out := make([]string, 0, len(involved))
	for i := len(involved) - 1; i >= 0; i-- {
		mm := involved[i]
		if mm.FromID == peerID {
			out = append(out, "→ "+shortPeerID(mm.ToID)+": "+mm.Text)
		} else {
			out = append(out, "← "+shortPeerID(mm.FromID)+": "+mm.Text)
		}
	}
	return out
}

// parseUITime parses a store timestamp (RFC3339) for display math.
func parseUITime(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
