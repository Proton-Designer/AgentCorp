package ui

import (
	"time"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/layout"
	"github.com/Proton-Designer/AgentCorp/internal/store"
)

// Message-flow overlay (F1). A real broker message from one agent to an adjacent
// one lights a bright pulse traveling ALONG the existing connector wire, in the
// message's direction. It depicts TRANSPORT — a message was sent, per a real
// broker row — and nothing more: the pulse stops at the end of the wire and never
// enters or reacts on the destination card, keeping the settled queued-vs-acted-on
// line (msg.DeliveryState) intact. Only adjacent (parent↔child) pairs animate,
// because only they share a single drawn wire; non-adjacent traffic has no direct
// line to ride and is left to the ticker/newswire.

// FlowWindow is how recently a message must have been sent for its edge to still
// be pulsing. A touch longer than the demo's send cadence so there's usually a
// pulse or two in flight, never a saturated chart.
const FlowWindow = 2500 * time.Millisecond

// flowFramePeriod is the frames per full head-to-tail traversal at FrameInterval
// (~1.4s), and flowTail is how many cells trail the head as a fading comet.
const (
	flowFramePeriod = 14
	flowTail        = 2
)

// flowSpec is one edge to animate: the parent and child node NAMES, whether the
// message traveled up (child→parent) rather than down, and when it was sent (for
// the recency fade). Names, not node pointers, because it's computed at the data
// tick before the tree is positioned; the render path resolves names to cells.
type flowSpec struct {
	parent, child string
	up            bool
	sentAt        time.Time
}

// computeFlows derives the animatable edges from the recent message stream. It
// keeps only the single newest message per edge (one pulse per wire, never a
// stack), only messages within window, and only pairs that are actually adjacent
// in the hierarchy.
func computeFlows(nodes []store.Node, msgs []broker.Message, now time.Time, window time.Duration) []flowSpec {
	peerToName := make(map[string]string, len(nodes))
	idToName := make(map[string]string, len(nodes))
	for _, n := range nodes {
		if n.PeerID != "" {
			peerToName[n.PeerID] = n.Name
		}
		idToName[n.NodeID] = n.Name
	}
	parentOf := make(map[string]string, len(nodes)) // childName → parentName
	for _, n := range nodes {
		if n.ParentID != "" {
			if pn, ok := idToName[n.ParentID]; ok {
				parentOf[n.Name] = pn
			}
		}
	}

	var flows []flowSpec
	seen := map[string]bool{}
	// msgs is oldest-first, so descending index is newest-first (time-descending).
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		t, ok := parseMsgTime(m.SentAt)
		if !ok {
			continue
		}
		if now.Before(t) {
			continue // a future timestamp (clock skew) is not "recent", just skip it
		}
		if now.Sub(t) > window {
			break // older than the window; everything earlier is older still
		}
		from, to := peerToName[m.FromID], peerToName[m.ToID]
		if from == "" || to == "" {
			continue
		}
		var parent, child string
		var up bool
		switch {
		case parentOf[to] == from: // parent → child (travels down the wire)
			parent, child, up = from, to, false
		case parentOf[from] == to: // child → parent (travels up)
			parent, child, up = to, from, true
		default:
			continue // not adjacent — no single wire to ride
		}
		key := parent + "\x00" + child
		if seen[key] {
			continue
		}
		seen[key] = true
		flows = append(flows, flowSpec{parent: parent, child: child, up: up, sentAt: t})
	}
	return flows
}

// edgePath returns the ordered connector cells from parent p down to child c,
// matching layout.Connectors' routing exactly (stem at parent centre → bus row →
// drop at child centre). The path deliberately ENDS at the last wire cell above
// the child's border — never a card cell — so a pulse riding it cannot appear to
// enter the destination.
func edgePath(p, c *layout.Node) [][2]int {
	pcx := p.X + p.W/2
	stemY := p.Y + p.H
	busY := stemY + 1
	childTopY := c.Y // == p.Y + p.H + vgap for the tree's uniform vgap
	cx := c.X + c.W/2

	var path [][2]int
	path = append(path, [2]int{pcx, stemY}) // stem
	path = append(path, [2]int{pcx, busY})  // onto the bus (or straight drop for a lone child)
	if cx != pcx {
		step := 1
		if cx < pcx {
			step = -1
		}
		for x := pcx + step; x != cx; x += step {
			path = append(path, [2]int{x, busY})
		}
		path = append(path, [2]int{cx, busY})
	}
	for y := busY + 1; y <= childTopY-1; y++ { // drop down to just above the child
		path = append(path, [2]int{cx, y})
	}
	return path
}

// parseMsgTime parses a broker/store timestamp the same way vitals does
// (RFC3339 with optional fractional seconds), kept local so the ui package
// doesn't reach into vitals' unexported helper.
func parseMsgTime(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
