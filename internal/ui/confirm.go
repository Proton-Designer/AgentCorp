package ui

import (
	"fmt"
	"strings"

	"github.com/Proton-Designer/AgentCorp/internal/store"
	"github.com/Proton-Designer/AgentCorp/internal/vitals"
)

// confirmKind distinguishes the two destructive actions. They are deliberately
// separate: firing a manager must NEVER cascade by default (spec §6.3), and a
// single keystroke must never be able to end six agents' work.
type confirmKind int

const (
	confirmFire    confirmKind = iota // reparents children, kills one node
	confirmDisband                    // kills an entire subtree
)

// confirmState is a pending destructive action awaiting the operator's answer.
type confirmState struct {
	kind       confirmKind
	victim     store.Node
	casualties []store.Node // disband only: everything that dies
	moves      int          // fire only: how many children get reparented
	newParent  string       // fire only: where they land
	statuses   map[string]vitals.Status
}

// renderConfirm draws the confirmation dialog.
//
// The disband dialog enumerates EVERY process that will die and flags the ones
// currently active. This is the requirement from §6.3, and the reason is that
// "disband lead-be's team" tells you nothing about what you're destroying —
// the operator needs the list, not the intent.
func renderConfirm(c *confirmState, width int) string {
	var b strings.Builder

	switch c.kind {
	case confirmFire:
		b.WriteString("╭─ Fire " + c.victim.Name + "? ")
		b.WriteString(strings.Repeat("─", max(0, 44-len(c.victim.Name))) + "╮\n")
		b.WriteString("│\n")
		b.WriteString("│  Terminates 1 session: " + c.victim.Name + "\n")
		if c.moves > 0 {
			// The whole point of reparent-not-cascade: say plainly that the
			// children survive, so the operator isn't guessing.
			b.WriteString(fmt.Sprintf("│  %d report(s) survive and move to: %s\n", c.moves, c.newParent))
		} else {
			b.WriteString("│  No reports to reassign.\n")
		}
		b.WriteString("│\n")
		b.WriteString("│  [ f ] fire      [ esc ] cancel\n")
		b.WriteString("╰" + strings.Repeat("─", 52) + "╯\n")

	case confirmDisband:
		b.WriteString("╭─ Disband " + c.victim.Name + "'s team? ")
		b.WriteString(strings.Repeat("─", max(0, 36-len(c.victim.Name))) + "╮\n")
		b.WriteString("│\n")
		b.WriteString(fmt.Sprintf("│  This terminates %d session(s):\n", len(c.casualties)))
		active := 0
		for _, n := range c.casualties {
			mark := ""
			if c.statuses[n.NodeID] == vitals.StatusActive {
				mark = "  ⚠ active"
				active++
			}
			b.WriteString(fmt.Sprintf("│    • %-16s %s%s\n", n.Name, n.Role, mark))
		}
		b.WriteString("│\n")
		if active > 0 {
			b.WriteString(fmt.Sprintf("│  %d agent(s) are active and will lose their work.\n", active))
			b.WriteString("│\n")
		}
		b.WriteString("│  [ D ] disband (shift-D)     [ esc ] cancel\n")
		b.WriteString("╰" + strings.Repeat("─", 52) + "╯\n")
	}
	return b.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
