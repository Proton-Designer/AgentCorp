package snapshot

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/store"
)

func sampleRows() []store.Node {
	return []store.Node{
		{NodeID: "1", Name: "ceo", Role: "lead", State: "alive", PeerID: "boss"},
		{NodeID: "2", Name: "worker", Role: "engineer", ParentID: "1", State: "alive", PeerID: "w1"},
		{NodeID: "3", Name: "intern", Role: "researcher", ParentID: "2", State: "pending"},
	}
}

func TestBuildResolvesParentNames(t *testing.T) {
	s := Build("Galaxy", "2026-07-18T06:00:00Z", sampleRows())
	byName := map[string]Node{}
	for _, n := range s.Nodes {
		byName[n.Name] = n
	}
	if byName["worker"].Parent != "ceo" {
		t.Fatalf("worker parent = %q, want ceo (resolved from id)", byName["worker"].Parent)
	}
	if byName["ceo"].Parent != "" {
		t.Fatalf("ceo should have no parent, got %q", byName["ceo"].Parent)
	}
}

func TestJSONRoundTrips(t *testing.T) {
	s := Build("Galaxy", "2026-07-18T06:00:00Z", sampleRows())
	data, err := s.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var back Snapshot
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("snapshot JSON doesn't round-trip: %v", err)
	}
	if back.Company != "Galaxy" || len(back.Nodes) != 3 {
		t.Fatalf("round-trip lost data: %+v", back)
	}
}

func TestMarkdownIsNestedTree(t *testing.T) {
	s := Build("Galaxy", "2026-07-18T06:00:00Z", sampleRows())
	md := s.Markdown()
	if !strings.Contains(md, "# AgentCorp — Galaxy") {
		t.Fatalf("missing titled header:\n%s", md)
	}
	if !strings.Contains(md, "3 agents") {
		t.Fatalf("missing count:\n%s", md)
	}
	// worker is nested under ceo (deeper indent), intern under worker.
	ceoIdx := strings.Index(md, "**ceo**")
	workerIdx := strings.Index(md, "**worker**")
	internIdx := strings.Index(md, "**intern**")
	if !(ceoIdx < workerIdx && workerIdx < internIdx) {
		t.Fatalf("tree order wrong:\n%s", md)
	}
	if !strings.Contains(md, "  - **worker**") {
		t.Fatalf("worker not indented under ceo:\n%s", md)
	}
}
