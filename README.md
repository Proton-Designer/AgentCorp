# AgentCorp

**A company of AI agents you can see and steer.**

AgentCorp renders your running Claude Code sessions as a live org chart in the terminal — who exists, what they're doing, how they relate — and gives you the controls to hire, direct, restructure, and retire them without leaving the screen.

```
╭─ AgentCorp ──────────────────────────────────────────────────────────────────╮
  ● live  ·  7 agents  ·  1 active · 6 quiet  ·  ▂▂█▂▃▂▁▂▁▂  ·  up 2h14m

                                 ╭────────────╮
                                 │    ceo     │
                                 ╰────────────╯
                                        │
                        ┌───────────────┴───────────────┐
                 ╭────────────╮                  ╭────────────╮
                 │  lead-be   │                  │  lead-fe   │
                 ╰────────────╯                  ╰────────────╯
                        │                               │
                ┌───────┴───────┐               ┌───────┴───────┐
         ╭────────────╮  ╭────────────╮  ╭────────────╮  ╭────────────╮
         │backend-dev │  │   db-dev   │  │   ui-dev   │  │  reviewer  │
         ╰────────────╯  ╰────────────╯  ╰────────────╯  ╰────────────╯

  ◷ 09:41  lead-be → backend-dev  "take /bookings, gate on owner_id"
```

---

## Why

Coordinating several Claude Code sessions is already possible — [`claude-peers`](https://github.com/louislva/claude-peers-mcp) lets them discover and message each other. But it's **invisible**: the agents are anonymous OS processes, the org structure exists only in your head, and the only way to learn what anyone is doing is to run a tool and read a string.

AgentCorp is the company layer on top. It doesn't replace `claude-peers` — it makes it legible and operable.

**Why not subagent teams?** Subagents are ephemeral, orchestrator-controlled, and opaque to you. `claude-peers` sessions are **persistent, individually controllable, and directly addressable**. That difference is the whole product.

---

## What it does

- **See the org, in colour.** A centered, top-down chart where every card is painted by its live status (active / quiet / pending / dead), with switchable colour themes (`t`).
- **Inspect any agent.** Press `i` for a detail panel — role, cwd, uptime, peer id, message counts, self-reported summary, and its most recent traffic — that you can page through as you move.
- **Steer it.** Hire an agent (with a role template), message one directly, broadcast to a whole team, rename, reparent/move, fire, or disband a subtree.
- **Watch the flow.** A scrollable activity feed (`l`) of the company's message traffic, a live HUD (headcount, active/quiet, throughput sparkline, uptime), and an always-moving ticker.
- **Survive restarts.** The org is durable. Close AgentCorp, reopen it, everything is still there. Slow hires self-heal — a session that registers late binds automatically.
- **Adopt what's already running.** Sessions AgentCorp didn't spawn show up as unmanaged; adopt them into the chart (`a`).
- **Scope by company.** Link a directory to a company and AgentCorp shows only the sessions inside it — so one laptop running many unrelated Claude sessions stops crowding a single chart.
- **Export it.** Snapshot the org to JSON + a Markdown tree (`e`).

---

## The living company

The chart isn't a static diagram — it breathes. Everything that moves is driven by
a real substrate signal, never invented for the animation's sake, so the pretty
version is exactly as honest as the plain one.

- **Message-flow.** When one agent messages an adjacent one, a bright pulse travels
  *along the connector wire* in the message's direction — because a real broker row
  exists. It depicts transport only: the pulse stops at the wire's end and never
  enters the destination card, so "a message was sent" is never dressed up as "the
  other agent is acting on it."
- **Speech bubbles.** The selected agent shows what it actually said last — its most
  recent message, quoted, with that message's real age (`said 3s ago`). Past a
  threshold it flips to `quiet for 3m`, so a bubble appearing *now* can't imply the
  agent is speaking now. Its self-summary rides the bubble dimmed and marked — the
  substrate carries no timestamp for a summary, so it's never shown as a live quote.
- **Vital signs.** Active agents' status LEDs breathe; in lively mode their whole
  card border pulses, phase-staggered. The pulse is driven by the same
  message-recency active/quiet signal the HUD uses — the animation adds no new claim.
- **Newswire + heartbeat.** A broadcast-style band scrolls the recent feed by agent
  name, over a pulse monitor whose spikes track real message activity in time. A
  quiet company flatlines and scrolls nothing.
- **Views.** Toggle an **office / floor-plan** (`o`) — departments as walled rooms,
  agents as desks — or a **mission-control dashboard** (`g`) — the chart beside live
  VITALS / WIRE / ALERTS panels. Same data, re-budgeted into a control deck.
- **Motion budget.** One lever (`v`) cycles **off → calm → lively**, so the ambient
  motion stays tasteful rather than noisy — or perfectly still for a screenshot or a
  slow SSH link. A cinematic boot sequence plays on launch (any key skips it).

Performance is honest too: layout runs once per data tick (1 Hz); the ~10 fps frame
clock only composites a small overlay onto a cached grid, and brightness pulses
quantise to a few levels so a terminal's per-line diff still skips most rows.

---

## Self-healing supervision

A company that can revive a dead agent should be able to do it *itself*. AgentCorp
already resumes a dead agent's real session with its memory intact (`z` / `shift-Z`,
via `claude --resume`). Supervision turns that into automatic, policy-driven,
budget-bounded fault tolerance — Erlang/OTP's supervision trees, adapted honestly to
a substrate where a restart is expensive and non-idempotent.

- **Policies** per supervisor: one-for-one (revive just the dead child), one-for-all
  (a manager dies → its whole subtree), rest-for-one. The reporting hierarchy already
  encodes the tree.
- **Budgets:** a permanently-broken agent must not loop-revive and burn real API cost,
  so restarts are bounded per window; exceeding the budget **escalates to a human**,
  it never autonomously kills a healthy agent. That last part is a deliberate
  divergence from literal OTP — an LLM agent's context is a valuable conversation, not
  a cheap process, so we never cascade-kill a working one to satisfy the analogy.
- **Opt-in by default.** An armed supervisor auto-revives, which spawns real sessions,
  so the default is *decide-and-show*: the supervisor's decision is displayed and
  nothing is revived until you arm it (`shift-S`). Try it in `--demo` — one agent
  starts dead; arm supervision and watch it come back.

**What it does not claim.** A restart resumes the *same* session, but a resumed agent
continues non-deterministically, and revival **refuses** when the session's memory is
gone rather than silently spawning a different agent. It is honest retry-with-budget
wearing OTP's structure — not OTP's assumption that the restarted child converges on
the same result. And it recovers the agent's memory, not the world's: the codebase may
have moved while the agent was dead, and nothing tells it what changed.

---

## The epistemic auditor

The frontier problem in multi-agent systems isn't moving messages faster — it's that
**nothing an agent says can be checked without redoing it**, and the failures that
result are honest ones: a test that passes regardless of correctness, a measurement
equal to its own timeout, two sources "agreeing" because neither can disagree. The
auditor (`internal/audit`, and a standalone `audit` tool) is a passive layer that asks
*"could this **check** have caught anything,"* never the unanswerable *"is this
**claim** true."*

Five cheap, domain-general detectors flag verification that could not discriminate:
a measurement equal to a cap; a check that doesn't fail on a seeded break (the fig-leaf
killer); a result cheaper than predicted; agreement between sources that share a
dependency; and a capability no test reaches through the real transport. It is
[metamorphic testing](https://dl.acm.org/doi/10.1145/3143561) applied to agent
coordination — the technique is decades old; what's new is running it as a
**cross-agent, blind-graded verification loop**, where one agent's tool audits another
team's codebase and a third party grades the findings blind, with pre-registered
predictions on both sides.

Pointed at AgentCorp's own tests it found a documented invariant with no test, a
cascade-kill path never invoked, and edge branches that survive mutation. Pointed at a
peer team's TypeScript codebase — blind — it found a documented invariant no test
covered: a fix that had been made but never tested, so it could silently regress (the
peer team then closed it with a test that provably fails without the fix). It is a
**diagnostic, never a score**: at the precision honesty demands it surfaces "N to
investigate," and a scored detector is a gamed detector. The full design, the
refutation that shaped it, and the honest results (including what it misses and its
~11% precision on the invariant detector) are in
[`docs/design/proof-carrying-claims.md`](docs/design/proof-carrying-claims.md).

---

## Companies

By default `claude-peers` is a flat, machine-wide mesh: every Claude session on the laptop can see every other. That's noise the moment you're doing more than one thing. AgentCorp scopes the view to a **company**, which is just a directory subtree.

- On launch, AgentCorp walks up from the working directory to the nearest `.agentcorp/company.toml` — exactly how git finds the repo that owns a path. **Nearest wins**, so a subfolder can carry its own company without leaking into its parent's.
- Find one, and AgentCorp shows only sessions whose working directory belongs to that company, titles the console with its name, and keeps that company's org chart in the folder (`.agentcorp/agentcorp.db`).
- Find none, and AgentCorp offers to create one for this directory, or to run **unscoped** — every session on the machine, the original behavior — if you decline.

The definition is a two-line file meant to be shared:

```toml
# .agentcorp/company.toml
id   = "co-20260717T090000.000000"   # stable; how sessions recognize the same company
name = "Acme Corp"                    # the display name; edit freely
```

Commit `company.toml` so your whole team resolves into the same company; a generated `.agentcorp/.gitignore` keeps the local runtime store out of the repo. Any session started anywhere inside that directory tree — by AgentCorp or not — is automatically in the company.

---

## What it does *not* do

This section is deliberately as prominent as the feature list. AgentCorp is built on a substrate with real limits, and a tool that hides its limits is worse than one that names them.

### It cannot prevent forged messages

`claude-peers` has **no authentication**. The sender field is client-supplied and unverified — any local process can send a message claiming to be any agent, by talking to the broker directly.

**AgentCorp makes forged and unexpected messages _visible_. It does not prevent them.**

Real blocking would require either forking `claude-peers` (so there's code on the *receiving* side) or proxying the broker. AgentCorp does neither. A gate only AgentCorp honors isn't a gate — it's a convention an attacker declines to follow. What we ship is **detection**: messages that don't match a known org relationship get flagged for you.

### It cannot tell you if an agent is "working"

The substrate exposes exactly two live signals per agent: a free-text summary the agent writes itself (no schema, no cadence — it can go stale for an hour) and a heartbeat that ticks every 15 seconds **whether the agent is mid-task or asleep**.

We checked the alternatives. CPU on a working agent's process reads **0.0%** — busy and idle both sit in network wait. Process state, tty, and process tree carry nothing either.

So AgentCorp shows **active / quiet**, derived from message recency, which means exactly what it says: *this agent has been talking recently*. It does **not** show "working/idle", because that would be a guess wearing a glyph.

### Messages arrive at turn boundaries, not instantly

A message to a busy agent **cannot interrupt it**. During a tool call the model isn't executing at all — the harness is — so there's nothing to interrupt. Your message lands when the agent's current turn ends.

AgentCorp renders inbound messages as **queued**, never as a live interrupt. And "sent" is shown distinctly from "acted on": the substrate never acknowledges delivery, so send-success proves transport, not receipt.

### It bets on your terminal treating box-drawing as narrow

The chart is drawn with box-drawing characters (`─ │ ╭ ╮`), and those are
East-Asian-Ambiguous width in Unicode — a terminal *may* render them one cell wide
or two, depending on its `ambiguous-width` setting. AgentCorp assumes **narrow**,
which is the default in modern terminals (Ghostty, iTerm2, kitty, Alacritty) and in
tmux. That's a bet, not a dodge: a terminal configured to render ambiguous glyphs
*wide* will shear the chart's alignment. The animated overlays deliberately stick to
this same class (plus unambiguous block elements `▁▂▃` for the graphs), so nothing
tonight widened the exposure — but if your chart looks misaligned, check that
setting first.

Measured, so it's concrete rather than folklore: in a normal (non-CJK) locale the
status and role glyphs all resolve to width 1, so there's no misalignment with the
current glyph set. Under an East-Asian locale (`EastAsianWidth=true`), the round
markers (`●` active, `◆` researcher) become width 2 while the gear/spark stay 1 —
so the risk is real but locale-dependent, and it would hit those roles first. The
one scenario we could not test end-to-end here is a terminal actually configured to
render ambiguous glyphs wide; that remains an open, precisely-scoped gap rather than
a claim of safety.

### It stands on a research preview

Message delivery rides Anthropic's **Channels** feature, which is a research preview and may change. All channel interaction is isolated behind one interface, but that's the foundation.

---

## Requirements

- **Go 1.25+** (to build)
- **tmux** — AgentCorp spawns each agent into its own tmux window. Click an
  agent's card (or press Enter on it) to jump straight to its session; AgentCorp
  turns on the tmux mouse and focus-event options it needs for that on launch and
  restores them on exit, so there's nothing to add to your `~/.tmux.conf`.
- **[`claude-peers`](https://github.com/louislva/claude-peers-mcp)** installed and registered
- **Claude Code**, launched with channels enabled

## Install

```sh
go install github.com/Proton-Designer/AgentCorp/cmd/agentcorp@latest
```

Or build from source:

```sh
git clone https://github.com/Proton-Designer/AgentCorp && cd AgentCorp
go build ./cmd/agentcorp && ./agentcorp
```

## Keys

| Key | Action |
|---|---|
| `↑` `↓` / `j` `k` | move the cursor |
| `space` | fold / unfold a subtree |
| `i` | inspect the selected agent (page with `↑↓`) |
| `h` | hire an agent, then pick a role |
| `a` | adopt an unmanaged session |
| `m` | message the selected agent |
| `b` | broadcast to its whole team |
| `R` | rename the selected agent |
| `r` | move it under a new manager |
| `x` | fire the selected agent |
| `shift-D` | disband a subtree |
| `z` | revive a dead agent (resume its session) |
| `shift-Z` | revive ALL dead agents at once |
| `shift-S` | arm / disarm auto-supervision (self-healing) |
| `/` | find by name / role / status |
| `l` | activity feed (org message log) |
| `o` | toggle office / floor-plan view |
| `g` | toggle mission-control dashboard |
| `e` | export org snapshot (JSON + Markdown) |
| `t` | cycle colour theme |
| `v` | cycle motion (off / calm / lively) |
| `?` | help + colour legend |
| `q` | quit |

---

## A note on consent

AgentCorp spawns agents with development channels enabled, which Claude Code guards behind a warning you must accept — **every session, not once per machine**. Asking you to click that for every hire would make the tool unusable, so **AgentCorp accepts it on your behalf.**

That gate exists so a human consciously opts into a risky feature, and automating it defeats it by design. So AgentCorp asks you **once, on first run**, showing the same warning, and stating plainly that it will accept for every agent it spawns. If you'd rather not grant that, don't run AgentCorp — the tool is the consent.

We looked for a flag to suppress the prompt properly. There isn't one.

---

## Architecture

```
cmd/agentcorp          entry point
internal/
  store           sidecar DB — hierarchy, roles, node metadata (ours)
  broker          READ-ONLY reader for claude-peers' DB (never written)
  company         directory → company resolution + scoping — pure core
  snapshot        org → JSON + Markdown export — pure formatting
  layout          Reingold-Tilford tree positioning — pure, no I/O
  sync            the tick loop: poll → diff → reconcile → apply
  vitals          derived state — pure
  spawn           process launch — argv arrays, never shell strings
  lifecycle       tree surgery: reparent, disband — pure
  ui              paint + Bubble Tea
```

**The load-bearing idea:** the hierarchy is AgentCorp's fiction. `claude-peers` underneath is a flat mesh that knows nothing about parents, children, or roles. Every node is *two things* — a real OS process and a row in AgentCorp's own store — and every lifecycle operation is a real process action **plus** a metadata edit.

**Why so many pure packages:** `layout`, `vitals`, and `lifecycle` do no I/O. That's what makes the riskiest logic exhaustively testable — and the riskiest logic here is layout, which took three rewrites before a line of it shipped.

---

## Status

**Phases 1–3 complete, plus directory-scoped companies, a deep operability layer, and a full living-company visual layer.** The chart renders in colour, polls the live substrate every second, reports real vitals, and the full lifecycle works — hire (with role templates) → self-healing bind → message / broadcast → inspect → rename → move → fire → disband → revive-from-memory — scoped to the company that owns the launch directory, with adoption, an activity feed, themes, and snapshot export. On top sits a decoupled animation layer — message-flow on the wires, breathing vital signs, honest speech bubbles, a scrolling newswire + heartbeat, office and mission-control views, and a boot sequence — all driven by real substrate signals and gated by a motion budget. Single ~12MB binary.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for setup and the bar for a change, and
[ARCHITECTURE.md](ARCHITECTURE.md) for how the codebase is organized. Security
reports go through [SECURITY.md](SECURITY.md), not public issues.

## License

MIT — see [LICENSE](LICENSE).
