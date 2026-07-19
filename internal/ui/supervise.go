package ui

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Proton-Designer/AgentCorp/internal/resume"
	"github.com/Proton-Designer/AgentCorp/internal/store"
	"github.com/Proton-Designer/AgentCorp/internal/supervision"
)

// The supervision integration (invention #2): AgentCorp's already-shipped
// memory-intact revival (claude --resume) turned into automatic, policy-driven,
// budget-bounded fault tolerance. The pure decision engine lives in
// internal/supervision; this file is the live wiring.
//
// Two honesty constraints govern it, both load-bearing:
//   - It is DECIDE-AND-SHOW by default, ACT only on opt-in. Auto-reviving spawns a
//     real Claude session — real cost — so a dead agent is reported and the
//     supervisor's decision is shown, but nothing is revived until the operator
//     turns supervision on ('S'). The default never spends money on the operator's
//     behalf.
//   - Restart is not idempotent recovery. `claude --resume` brings back the SAME
//     session with its real transcript, but a resumed agent continues non-
//     deterministically, and Flow.Revive REFUSES when the session's memory is gone
//     rather than silently spawning a fresh, different agent. So this is honest
//     retry/resume-with-budget wearing OTP's structure — not the idempotent-restart
//     assumption OTP's analogy would smuggle in.

const superviseMaxEvents = 6

// runSupervision is the tick-loop hook. It finds nodes that newly died THIS tick,
// asks the pure policy engine what to do, records the decisions for display, and —
// only when supervision is enabled — queues the honest revive actions. It never
// autonomously kills a healthy node; ActionKillAndRestart is surfaced, never
// auto-executed (it carries the same real-cost/real-destruction weight as disband).
func (m *Model) runSupervision(nodes []store.Node, now time.Time) {
	cur := make(map[string]bool)
	for _, n := range nodes {
		if n.State == "dead" {
			cur[n.NodeID] = true
		}
	}
	var newlyDead []string
	for id := range cur {
		if !m.live.deadSet[id] {
			newlyDead = append(newlyDead, id)
		}
	}
	m.live.deadSet = cur
	if len(newlyDead) == 0 {
		return
	}

	policies, _ := m.live.st.ListSupervisionPolicies()
	history, _ := m.live.st.ListRestartEvents()
	plan := supervision.Evaluate(nodes, newlyDead, history, policies, now)

	nameOf := func(id string) string {
		for _, n := range nodes {
			if n.NodeID == id {
				return n.Name
			}
		}
		return id
	}
	for _, d := range plan.Decisions {
		m.pushSuperEvent(fmt.Sprintf("%s → %s (%s)", nameOf(d.NodeID), d.Action, d.Reason))
		if d.Action == supervision.ActionRevive && m.live.superviseOn {
			m.live.pendingRevives = append(m.live.pendingRevives, d.NodeID)
		}
	}
}

func (m *Model) pushSuperEvent(s string) {
	m.live.superEvents = append(m.live.superEvents, s)
	if n := len(m.live.superEvents); n > superviseMaxEvents {
		m.live.superEvents = m.live.superEvents[n-superviseMaxEvents:]
	}
}

// superviseCmd dispatches any queued revives as a background command (never
// blocking the UI), stamping a RestartEvent for the budget and honouring the same
// memory-gone refusal as manual revive. Returns nil when there is nothing to do or
// supervision is off / unwired (demo has no hire flow, so it decides-and-shows only).
func (m Model) superviseCmd() tea.Cmd {
	if m.live == nil || len(m.live.pendingRevives) == 0 {
		return nil
	}
	ids := m.live.pendingRevives
	m.live.pendingRevives = nil
	if !m.live.superviseOn {
		return nil
	}
	st := m.live.st
	// Demo has no hire flow: heal synthetically via Store.Revive (dead → pending,
	// reconnecting) so the supervisor's action is visible without spawning anything.
	// Honest — a real session would resume via claude --resume, not this shortcut.
	if m.live.demo {
		revived := 0
		for _, id := range ids {
			if err := st.Revive(id); err == nil {
				revived++
			}
		}
		return func() tea.Msg {
			return actionResultMsg{text: fmt.Sprintf("supervisor: reviving %d (demo — resuming session)", revived)}
		}
	}
	if m.live.hireFlow == nil {
		return nil
	}
	flow := m.live.hireFlow
	return func() tea.Msg {
		home, _ := os.UserHomeDir()
		revived, skipped := 0, 0
		nodes, _ := st.ListNodes()
		byID := make(map[string]store.Node, len(nodes))
		for _, n := range nodes {
			byID[n.NodeID] = n
		}
		for _, id := range ids {
			row, ok := byID[id]
			if !ok || row.State != "dead" {
				continue
			}
			if row.SessionID == "" || !resume.Exists(home, row.Workdir, row.SessionID) {
				skipped++ // memory gone — never fake a revive
				continue
			}
			_ = st.InsertRestartEvent(id, time.Now(), "supervision")
			deadline := flow.BindTimeout + 60*time.Second
			if deadline < 90*time.Second {
				deadline = 90 * time.Second
			}
			ctx, cancel := context.WithTimeout(context.Background(), deadline)
			if _, err := flow.Revive(ctx, row); err == nil {
				revived++
			} else {
				skipped++
			}
			cancel()
		}
		return actionResultMsg{text: fmt.Sprintf("supervisor: revived %d, skipped %d (memory gone)", revived, skipped)}
	}
}

// renderSupervision draws the supervision status strip: whether auto-heal is armed,
// and the recent decisions. Shown only when there is something to say.
func (m Model) renderSupervision() string {
	if m.live == nil || len(m.live.superEvents) == 0 {
		return ""
	}
	state := "off (decide-and-show)"
	if m.live.superviseOn {
		state = "ARMED (auto-heal)"
	}
	out := "\n  " + wrapANSI("▎ supervisor", styActive) + " — " + state + "\n"
	for _, e := range m.live.superEvents {
		out += "    " + truncate(e, m.width-6) + "\n"
	}
	return out
}
