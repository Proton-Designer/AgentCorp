package ui

import (
	"testing"
	"time"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/layout"
	"github.com/Proton-Designer/AgentCorp/internal/store"
)

func flowNodes() []store.Node {
	return []store.Node{
		{NodeID: "1", Name: "CEO", PeerID: "pC", CreatedAt: "2026-01-01T00:00:00Z"},
		{NodeID: "2", Name: "backend", ParentID: "1", PeerID: "pB", CreatedAt: "2026-01-01T00:00:01Z"},
		{NodeID: "5", Name: "intern", ParentID: "2", PeerID: "pI", CreatedAt: "2026-01-01T00:00:02Z"},
	}
}

func at(now time.Time, ago time.Duration) string {
	return now.Add(-ago).UTC().Format(time.RFC3339Nano)
}

func TestComputeFlowsDirectionAndAdjacency(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	msgs := []broker.Message{
		{ID: 1, FromID: "pB", ToID: "pC", SentAt: at(now, 10*time.Second)},  // too old (> window)
		{ID: 2, FromID: "pC", ToID: "pB", SentAt: at(now, 1200*time.Millisecond)}, // CEO→backend, down
		{ID: 3, FromID: "pI", ToID: "pB", SentAt: at(now, 500*time.Millisecond)},  // intern→backend, up
		{ID: 4, FromID: "pC", ToID: "pI", SentAt: at(now, 200*time.Millisecond)},  // CEO→intern, NOT adjacent
	}
	flows := computeFlows(flowNodes(), msgs, now, FlowWindow)

	// The non-adjacent CEO→intern and the too-old backend→CEO must be absent.
	if len(flows) != 2 {
		t.Fatalf("want 2 flows (CEO↔backend down, backend↔intern up), got %d: %+v", len(flows), flows)
	}
	byEdge := map[string]flowSpec{}
	for _, f := range flows {
		byEdge[f.parent+">"+f.child] = f
	}
	down, ok := byEdge["CEO>backend"]
	if !ok || down.up {
		t.Errorf("CEO→backend should be a downward flow, got %+v (ok=%v)", down, ok)
	}
	up, ok := byEdge["backend>intern"]
	if !ok || !up.up {
		t.Errorf("intern→backend should be an upward flow parented at backend, got %+v (ok=%v)", up, ok)
	}
}

func TestComputeFlowsKeepsNewestPerEdge(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	msgs := []broker.Message{
		{ID: 1, FromID: "pC", ToID: "pB", SentAt: at(now, 2000*time.Millisecond)},
		{ID: 2, FromID: "pC", ToID: "pB", SentAt: at(now, 300*time.Millisecond)}, // newer, same edge
	}
	flows := computeFlows(flowNodes(), msgs, now, FlowWindow)
	if len(flows) != 1 {
		t.Fatalf("one edge should collapse to a single (newest) flow, got %d", len(flows))
	}
	if got := now.Sub(flows[0].sentAt); got > 400*time.Millisecond {
		t.Errorf("kept the older message; age %v should be ~300ms", got)
	}
}

func TestEdgePathStopsAboveChild(t *testing.T) {
	// Position a two-node tree so the path geometry is real.
	root := &layout.Node{ID: "CEO", W: cardW, H: cardH, Children: []*layout.Node{
		{ID: "backend", W: cardW, H: cardH},
	}}
	layout.Position(root, hgap, vgap)
	child := root.Children[0]
	path := edgePath(root, child)
	if len(path) == 0 {
		t.Fatal("edge path must not be empty")
	}
	// Every path cell must sit strictly above the child's top border row — a pulse
	// riding it can never touch the destination card.
	for _, cell := range path {
		if cell[1] >= child.Y {
			t.Errorf("path cell %v is at/below child top y=%d — it would enter the card", cell, child.Y)
		}
	}
	// The first cell is the stem directly under the parent's centre.
	wantX := root.X + root.W/2
	if path[0][0] != wantX || path[0][1] != root.Y+root.H {
		t.Errorf("path should start at the parent stem (%d,%d), got %v", wantX, root.Y+root.H, path[0])
	}
}
