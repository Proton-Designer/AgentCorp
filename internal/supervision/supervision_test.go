package supervision

import (
	"testing"
	"time"

	"github.com/Proton-Designer/AgentCorp/internal/store"
)

func node(id, parent, state, createdAt string) store.Node {
	return store.Node{
		NodeID: id, ParentID: parent, Name: id, Role: "dev",
		Workdir: "/tmp", SpawnMode: "adopted", State: state, CreatedAt: createdAt,
	}
}

var fixedNow = mustParse("2026-07-19T12:00:00Z")

func mustParse(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func decisionFor(plan Plan, nodeID string) (Decision, bool) {
	for _, d := range plan.Decisions {
		if d.NodeID == nodeID {
			return d, true
		}
	}
	return Decision{}, false
}

// --- Strategy grouping ------------------------------------------------------

func TestOneForOneRestartsOnlyTheDeadNode(t *testing.T) {
	nodes := []store.Node{
		node("boss", "", "alive", "t0"),
		node("a", "boss", "dead", "t1"),
		node("b", "boss", "alive", "t2"),
	}
	plan := Evaluate(nodes, []string{"a"}, nil, nil, fixedNow)

	da, ok := decisionFor(plan, "a")
	if !ok || da.Action != ActionRevive {
		t.Fatalf("a: got %+v, want ActionRevive", da)
	}
	if _, ok := decisionFor(plan, "b"); ok {
		t.Fatal("b (sibling) must not appear in a one-for-one plan")
	}
}

func TestOneForAllRestartsDeadAndAliveSiblingsWithCorrectActions(t *testing.T) {
	nodes := []store.Node{
		node("boss", "", "alive", "t0"),
		node("a", "boss", "dead", "t1"),
		node("b", "boss", "alive", "t2"),
		node("c", "boss", "alive", "t3"),
	}
	policies := []store.Policy{{NodeID: "boss", Strategy: store.OneForAll, MaxRestarts: 3, WindowSeconds: 300}}
	plan := Evaluate(nodes, []string{"a"}, nil, policies, fixedNow)

	da, _ := decisionFor(plan, "a")
	if da.Action != ActionRevive {
		t.Fatalf("a (dead): got %v, want ActionRevive", da.Action)
	}
	db, ok := decisionFor(plan, "b")
	if !ok || db.Action != ActionKillAndRestart {
		t.Fatalf("b (alive, swept in): got %+v, want ActionKillAndRestart", db)
	}
	dc, ok := decisionFor(plan, "c")
	if !ok || dc.Action != ActionKillAndRestart {
		t.Fatalf("c (alive, swept in): got %+v, want ActionKillAndRestart", dc)
	}
	if len(plan.Decisions) != 3 {
		t.Fatalf("want exactly 3 decisions (a,b,c), got %d: %+v", len(plan.Decisions), plan.Decisions)
	}
}

func TestRestForOneExcludesEarlierSiblingsIncludesLater(t *testing.T) {
	nodes := []store.Node{
		node("boss", "", "alive", "t0"),
		node("early", "boss", "alive", "2026-07-19T00:00:00Z"),
		node("d", "boss", "dead", "2026-07-19T01:00:00Z"),
		node("late", "boss", "alive", "2026-07-19T02:00:00Z"),
	}
	policies := []store.Policy{{NodeID: "boss", Strategy: store.RestForOne, MaxRestarts: 3, WindowSeconds: 300}}
	plan := Evaluate(nodes, []string{"d"}, nil, policies, fixedNow)

	if _, ok := decisionFor(plan, "early"); ok {
		t.Fatal("early sibling (created before the dead node) must be excluded from rest-for-one")
	}
	dd, ok := decisionFor(plan, "d")
	if !ok || dd.Action != ActionRevive {
		t.Fatalf("d (dead): got %+v, want ActionRevive", dd)
	}
	dl, ok := decisionFor(plan, "late")
	if !ok || dl.Action != ActionKillAndRestart {
		t.Fatalf("late sibling (created after): got %+v, want ActionKillAndRestart", dl)
	}
}

// --- Budgets -----------------------------------------------------------------

func TestBudgetRespectedUnderLimit(t *testing.T) {
	nodes := []store.Node{
		node("boss", "", "alive", "t0"),
		node("a", "boss", "dead", "t1"),
	}
	history := []store.RestartEvent{
		{NodeID: "a", At: fixedNow.Add(-time.Minute)},
		{NodeID: "a", At: fixedNow.Add(-2 * time.Minute)},
	}
	policies := []store.Policy{{NodeID: "boss", Strategy: store.OneForOne, MaxRestarts: 3, WindowSeconds: 300}}
	plan := Evaluate(nodes, []string{"a"}, history, policies, fixedNow)

	da, ok := decisionFor(plan, "a")
	if !ok || da.Action != ActionRevive {
		t.Fatalf("2 prior restarts, budget 3: got %+v, want ActionRevive", da)
	}
}

func TestBudgetExceededEscalatesInsteadOfRevive(t *testing.T) {
	nodes := []store.Node{
		node("boss", "root", "alive", "t0"),
		node("root", "", "alive", "t-1"),
		node("a", "boss", "dead", "t1"),
	}
	history := []store.RestartEvent{
		{NodeID: "a", At: fixedNow.Add(-time.Minute)},
		{NodeID: "a", At: fixedNow.Add(-2 * time.Minute)},
		{NodeID: "a", At: fixedNow.Add(-3 * time.Minute)},
	}
	policies := []store.Policy{{NodeID: "boss", Strategy: store.OneForOne, MaxRestarts: 3, WindowSeconds: 300}}
	plan := Evaluate(nodes, []string{"a"}, history, policies, fixedNow)

	da, ok := decisionFor(plan, "a")
	if !ok || da.Action != ActionEscalate {
		t.Fatalf("3 prior restarts, budget 3 (at limit): got %+v, want ActionEscalate", da)
	}
	if da.SupervisorID != "boss" {
		t.Fatalf("SupervisorID = %q, want boss", da.SupervisorID)
	}
}

func TestBudgetWindowExpiryExcludesOldRestarts(t *testing.T) {
	nodes := []store.Node{
		node("boss", "root", "alive", "t0"),
		node("root", "", "alive", "t-1"),
		node("a", "boss", "dead", "t1"),
	}
	// 3 restarts, but all outside a 300s window -- must not count against budget.
	history := []store.RestartEvent{
		{NodeID: "a", At: fixedNow.Add(-10 * time.Minute)},
		{NodeID: "a", At: fixedNow.Add(-11 * time.Minute)},
		{NodeID: "a", At: fixedNow.Add(-12 * time.Minute)},
	}
	policies := []store.Policy{{NodeID: "boss", Strategy: store.OneForOne, MaxRestarts: 3, WindowSeconds: 300}}
	plan := Evaluate(nodes, []string{"a"}, history, policies, fixedNow)

	da, ok := decisionFor(plan, "a")
	if !ok || da.Action != ActionRevive {
		t.Fatalf("all restarts outside window: got %+v, want ActionRevive", da)
	}
}

// --- Escalation ---------------------------------------------------------------

func TestEscalationReachingRootAlsoAlerts(t *testing.T) {
	nodes := []store.Node{
		node("root", "", "alive", "t0"), // root itself is the supervisor here
		node("a", "root", "dead", "t1"),
	}
	history := []store.RestartEvent{
		{NodeID: "a", At: fixedNow.Add(-time.Minute)},
		{NodeID: "a", At: fixedNow.Add(-2 * time.Minute)},
		{NodeID: "a", At: fixedNow.Add(-3 * time.Minute)},
	}
	policies := []store.Policy{{NodeID: "root", Strategy: store.OneForOne, MaxRestarts: 3, WindowSeconds: 300}}
	plan := Evaluate(nodes, []string{"a"}, history, policies, fixedNow)

	foundEscalate, foundAlert := false, false
	for _, d := range plan.Decisions {
		if d.NodeID == "a" && d.Action == ActionEscalate {
			foundEscalate = true
		}
		if d.NodeID == "a" && d.Action == ActionAlertRoot {
			foundAlert = true
		}
	}
	if !foundEscalate {
		t.Fatal("want an ActionEscalate decision")
	}
	if !foundAlert {
		t.Fatal("escalation reached root (no grandparent) -- want an ActionAlertRoot too")
	}
}

func TestDeadRootAlertsDirectlyNoPolicyConsulted(t *testing.T) {
	nodes := []store.Node{node("root", "", "dead", "t0")}
	plan := Evaluate(nodes, []string{"root"}, nil, nil, fixedNow)

	if len(plan.Decisions) != 1 {
		t.Fatalf("want exactly 1 decision, got %d: %+v", len(plan.Decisions), plan.Decisions)
	}
	d := plan.Decisions[0]
	if d.Action != ActionAlertRoot || d.SupervisorID != "" {
		t.Fatalf("got %+v, want ActionAlertRoot with empty SupervisorID (no supervisor exists)", d)
	}
}

// --- Multiple simultaneous deaths ----------------------------------------------

func TestMultipleSimultaneousDeathsNoOverlapAreIndependent(t *testing.T) {
	nodes := []store.Node{
		node("boss1", "root", "alive", "t0"),
		node("boss2", "root", "alive", "t0"),
		node("root", "", "alive", "t-1"),
		node("a", "boss1", "dead", "t1"),
		node("b", "boss2", "dead", "t1"),
	}
	plan := Evaluate(nodes, []string{"a", "b"}, nil, nil, fixedNow)

	da, _ := decisionFor(plan, "a")
	db, _ := decisionFor(plan, "b")
	if da.Action != ActionRevive || db.Action != ActionRevive {
		t.Fatalf("independent deaths: a=%+v b=%+v, want both ActionRevive", da, db)
	}
}

func TestSimultaneousDeathSweptIntoAnotherGroupIsNotDoubleDecided(t *testing.T) {
	nodes := []store.Node{
		node("boss", "root", "alive", "t0"),
		node("root", "", "alive", "t-1"),
		node("a", "boss", "dead", "t1"),
		node("b", "boss", "dead", "t2"), // ALSO independently listed as dead this tick
	}
	policies := []store.Policy{{NodeID: "boss", Strategy: store.OneForAll, MaxRestarts: 3, WindowSeconds: 300}}
	// Both a and b listed as dead -- a's one-for-all group already includes b.
	plan := Evaluate(nodes, []string{"a", "b"}, nil, policies, fixedNow)

	count := 0
	for _, d := range plan.Decisions {
		if d.NodeID == "b" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("b appears %d times in the plan, want exactly 1 (no double-decision)", count)
	}
}

// --- Determinism ---------------------------------------------------------------

func TestEvaluateIsDeterministic(t *testing.T) {
	nodes := []store.Node{
		node("boss", "root", "alive", "t0"),
		node("root", "", "alive", "t-1"),
		node("a", "boss", "dead", "t1"),
		node("b", "boss", "dead", "t2"),
		node("c", "boss", "dead", "t3"),
	}
	p1 := Evaluate(nodes, []string{"c", "a", "b"}, nil, nil, fixedNow)
	p2 := Evaluate(nodes, []string{"a", "b", "c"}, nil, nil, fixedNow)

	if len(p1.Decisions) != len(p2.Decisions) {
		t.Fatalf("non-deterministic decision count: %d vs %d", len(p1.Decisions), len(p2.Decisions))
	}
	for i := range p1.Decisions {
		if p1.Decisions[i] != p2.Decisions[i] {
			t.Fatalf("non-deterministic at index %d: %+v vs %+v", i, p1.Decisions[i], p2.Decisions[i])
		}
	}
}

// --- Default policy --------------------------------------------------------

func TestDefaultPolicyAppliesWithNoExplicitRow(t *testing.T) {
	nodes := []store.Node{
		node("boss", "root", "alive", "t0"),
		node("root", "", "alive", "t-1"),
		node("a", "boss", "dead", "t1"),
		node("b", "boss", "alive", "t2"),
	}
	// No policies passed at all.
	plan := Evaluate(nodes, []string{"a"}, nil, nil, fixedNow)

	// DefaultPolicy is OneForOne, so b must not be swept in.
	if _, ok := decisionFor(plan, "b"); ok {
		t.Fatal("default policy must be one-for-one; b must not appear")
	}
	da, ok := decisionFor(plan, "a")
	if !ok || da.Action != ActionRevive {
		t.Fatalf("a: got %+v, want ActionRevive", da)
	}
}

// --- CreatedAt ordering with mixed timestamp precision --------------------

func TestCreatedAtLexicographicOrderHoldsAcrossMixedPrecisionTimestamps(t *testing.T) {
	// Real data in this codebase includes both bare-second and
	// millisecond-precision RFC3339 timestamps (spec fixtures use
	// "2026-07-16T00:00:00Z"; the live broker produces
	// "2026-07-17T02:14:40.469Z"). Confirm lexicographic comparison still
	// orders these correctly relative to each other -- a naive assumption
	// here would be the same class of bug this project hit before with tty
	// normalization.
	nodes := []store.Node{
		node("boss", "root", "alive", "t0"),
		node("root", "", "alive", "t-1"),
		node("early", "boss", "alive", "2026-07-16T00:00:00Z"),
		node("d", "boss", "dead", "2026-07-17T02:14:40.469Z"),
		node("late", "boss", "alive", "2026-07-18T00:00:00.000Z"),
	}
	policies := []store.Policy{{NodeID: "boss", Strategy: store.RestForOne, MaxRestarts: 3, WindowSeconds: 300}}
	plan := Evaluate(nodes, []string{"d"}, nil, policies, fixedNow)

	if _, ok := decisionFor(plan, "early"); ok {
		t.Fatal("early (bare-second timestamp, before d) must be excluded")
	}
	if _, ok := decisionFor(plan, "late"); !ok {
		t.Fatal("late (millisecond timestamp, after d) must be included")
	}
}

// --- Degenerate cases -------------------------------------------------------

func TestOneForAllWithNoOtherChildrenDegeneratesToOneForOne(t *testing.T) {
	nodes := []store.Node{
		node("boss", "root", "alive", "t0"),
		node("root", "", "alive", "t-1"),
		node("a", "boss", "dead", "t1"),
	}
	policies := []store.Policy{{NodeID: "boss", Strategy: store.OneForAll, MaxRestarts: 3, WindowSeconds: 300}}
	plan := Evaluate(nodes, []string{"a"}, nil, policies, fixedNow)

	if len(plan.Decisions) != 1 {
		t.Fatalf("want exactly 1 decision (a has no siblings), got %d: %+v", len(plan.Decisions), plan.Decisions)
	}
}

func TestUnknownDeadNodeIDIsIgnored(t *testing.T) {
	nodes := []store.Node{node("root", "", "alive", "t0")}
	plan := Evaluate(nodes, []string{"does-not-exist"}, nil, nil, fixedNow)
	if len(plan.Decisions) != 0 {
		t.Fatalf("want no decisions for an unknown node id, got %+v", plan.Decisions)
	}
}

func TestDanglingParentReferenceAlertsRootRatherThanCrashing(t *testing.T) {
	nodes := []store.Node{node("a", "does-not-exist", "dead", "t1")}
	plan := Evaluate(nodes, []string{"a"}, nil, nil, fixedNow)

	if len(plan.Decisions) != 1 {
		t.Fatalf("want exactly 1 decision, got %+v", plan.Decisions)
	}
	if plan.Decisions[0].Action != ActionAlertRoot {
		t.Fatalf("got %+v, want ActionAlertRoot", plan.Decisions[0])
	}
}
