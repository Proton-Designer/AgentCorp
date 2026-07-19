package ui

import (
	"testing"
	"time"

	"github.com/Proton-Designer/AgentCorp/internal/store"
)

func superviseTestModel(t *testing.T) (Model, *store.Store) {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/s.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	m := New(nil)
	m.live = &liveState{st: s}
	return m, s
}

func TestRunSupervisionDecidesOnNewlyDead(t *testing.T) {
	m, _ := superviseTestModel(t)
	nodes := []store.Node{
		{NodeID: "1", Name: "boss", State: "alive"},
		{NodeID: "2", Name: "worker", ParentID: "1", State: "dead"},
	}
	m.runSupervision(nodes, time.Now())

	if len(m.live.superEvents) == 0 {
		t.Fatal("a newly-dead node must produce a supervisor decision")
	}
	// deadSet now records the dead node, so a second identical tick is a no-op.
	before := len(m.live.superEvents)
	m.runSupervision(nodes, time.Now())
	if len(m.live.superEvents) != before {
		t.Errorf("an already-dead node must not re-fire a decision each tick")
	}
}

func TestSuperviseCmdGatedByOptIn(t *testing.T) {
	m, _ := superviseTestModel(t)
	m.live.pendingRevives = []string{"2"}
	// Off by default: no command, and the queue is not drained into an action.
	m.live.superviseOn = false
	if cmd := m.superviseCmd(); cmd != nil {
		t.Errorf("supervision off must not dispatch a revive command")
	}

	// Armed + demo: the demo synthetic-revive path returns a command.
	m.live.superviseOn = true
	m.live.demo = true
	m.live.pendingRevives = []string{"2"}
	if cmd := m.superviseCmd(); cmd == nil {
		t.Errorf("armed supervision with a pending revive must dispatch a command")
	}
}

func TestRunSupervisionQueuesReviveOnlyWhenArmed(t *testing.T) {
	// A SUPERVISED node (has a parent) gets ActionRevive; a parentless root would
	// escalate/alert instead, which is a different, correct outcome.
	nodes := []store.Node{
		{NodeID: "1", Name: "boss", State: "alive"},
		{NodeID: "2", Name: "worker", ParentID: "1", State: "dead"},
	}

	m, _ := superviseTestModel(t)
	m.live.superviseOn = false
	m.runSupervision(nodes, time.Now())
	if len(m.live.pendingRevives) != 0 {
		t.Errorf("with supervision off, nothing should be queued for revival")
	}

	m2, _ := superviseTestModel(t)
	m2.live.superviseOn = true
	m2.runSupervision(nodes, time.Now())
	if len(m2.live.pendingRevives) == 0 {
		t.Errorf("armed supervision should queue the dead node for revival")
	}
}
