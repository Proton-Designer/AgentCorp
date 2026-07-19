package ui

import (
	"fmt"
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/store"
	"github.com/Proton-Designer/AgentCorp/internal/vitals"
)

// Regression benchmark for the animation render path (F0, anim_render.go).
// Methodology: 23qm3cgf, 2026-07-19. ensureBase is warmed once, outside the
// timed loop — it simulates the 1Hz-cached base and must never re-run inside
// it (renderAnimated calls ensureBase itself each frame too; that's correct
// and intentional, since ensureBase's own baseVersion check is what makes it
// a no-op after the first call — this benchmark exercises that exact
// short-circuit, not a hand-rolled substitute for it). Only what a real 100ms
// frame tick pays — buildOverlay + renderAnimated's composite+emit — is
// timed. Every node is set StatusActive, so buildOverlay's walk is the
// worst case for card count (every eligible card gets a breathing LED): the
// binding question is "does emit blow the frame budget," and understating
// the number of animated cells would understate the answer.
//
// dimANSI never returns "" for styActive (checked: all three breath levels
// map to a real ANSI code), so cycling m.frame across iterations always
// exercises the full composite path — no frame in this loop silently takes
// renderAnimated's empty-overlay fast path.

// benchNodes builds store.Node rows in one of two shapes, matching the
// shapes used to size the pre-F0 emit path so the numbers are comparable:
// flat (a manager fan-out, 6 reports each) or deep (branch-3, breadth-first).
func benchNodes(n int, deep bool) []store.Node {
	mk := func(id, parent string) store.Node {
		return store.Node{
			NodeID: id, ParentID: parent, Name: id, Role: "dev",
			Workdir: "/tmp", SpawnMode: "adopted", State: "alive",
			PeerID: "p-" + id, CreatedAt: "2026-01-01T00:00:00Z",
		}
	}
	nodes := []store.Node{mk("root", "")}
	remaining := n - 1
	if deep {
		queue := []string{"root"}
		for qi := 0; qi < len(queue) && remaining > 0; qi++ {
			parent := queue[qi]
			for i := 0; i < 3 && remaining > 0; i++ {
				id := fmt.Sprintf("n%d", n-remaining)
				nodes = append(nodes, mk(id, parent))
				queue = append(queue, id)
				remaining--
			}
		}
		return nodes
	}
	mgrCount := 0
	for remaining > 0 && mgrCount < 8 {
		mgr := fmt.Sprintf("mgr%d", mgrCount)
		nodes = append(nodes, mk(mgr, "root"))
		mgrCount++
		remaining--
		for i := 0; i < 6 && remaining > 0; i++ {
			nodes = append(nodes, mk(fmt.Sprintf("%s-r%d", mgr, i), mgr))
			remaining--
		}
	}
	return nodes
}

func benchmarkAnimated(b *testing.B, nodeCount int, deep bool) {
	s, err := store.Open(b.TempDir() + "/bench.db")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { s.Close() })
	for _, r := range benchNodes(nodeCount, deep) {
		if err := s.InsertNode(r); err != nil {
			b.Fatal(err)
		}
	}
	nodes, err := s.ListNodes()
	if err != nil {
		b.Fatal(err)
	}

	m := NewLive(s, nodes)
	m.width = 200
	m.motion = motionCalm

	statuses := map[string]vitals.Status{}
	for _, n := range nodes {
		statuses[n.Name] = vitals.StatusActive
	}
	m.live.statuses = statuses
	m.live.animating = true

	m.ensureBase() // one-time warm — off the timed loop, matches the 1Hz cadence
	if m.live.baseW < 1 || m.live.baseH < 1 {
		b.Fatal("empty base grid")
	}
	b.Logf("base grid %dx%d, %d nodes, %d active", m.live.baseW, m.live.baseH, len(nodes), len(statuses))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.frame = i // advances the breath phase exactly as the real frame clock would
		_ = m.renderAnimated()
	}
}

func BenchmarkAnimatedDemoScale(b *testing.B)      { benchmarkAnimated(b, 5, false) }
func BenchmarkAnimatedFortyNodesFlat(b *testing.B) { benchmarkAnimated(b, 40, false) }
func BenchmarkAnimatedFortyNodesDeep(b *testing.B) { benchmarkAnimated(b, 40, true) }
