package ui

import (
	"strings"
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/store"
)

func TestNewDemoRendersPopulatedChart(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/demo.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	for _, n := range []store.Node{
		{NodeID: "1", Name: "CEO", Role: "lead", State: "alive", PeerID: "demo-1", Workdir: "/d", SpawnMode: "adopted", CreatedAt: "2026-07-18T00:00:00Z"},
		{NodeID: "2", Name: "backend", Role: "engineer", ParentID: "1", State: "alive", PeerID: "demo-2", Workdir: "/d", SpawnMode: "adopted", CreatedAt: "2026-07-18T00:00:01Z"},
	} {
		if err := s.InsertNode(n); err != nil {
			t.Fatal(err)
		}
	}
	nodes, _ := s.ListNodes()
	peers := func() ([]broker.Peer, error) { return []broker.Peer{{ID: "demo-1"}, {ID: "demo-2"}}, nil }
	msgs := func() ([]broker.Message, error) { return nil, nil }

	m := NewDemo(s, nodes, peers, msgs, "Demo Co")
	v := m.View()
	for _, want := range []string{"Demo Co", "CEO", "backend"} {
		if !strings.Contains(v, want) {
			t.Fatalf("demo view missing %q:\n%s", want, v)
		}
	}
}
