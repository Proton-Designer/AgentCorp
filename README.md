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

- **See the org.** A centered, top-down chart. Every agent shows its role, status, and a live one-line summary of what it's doing.
- **Steer it.** Hire an agent under any node, message any agent directly, restructure the tree, retire agents.
- **Survive restarts.** The org is durable. Close AgentCorp, reopen it, everything is still there.
- **Adopt what's already running.** Sessions AgentCorp didn't spawn show up as unmanaged; adopt them into the chart.

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

### It stands on a research preview

Message delivery rides Anthropic's **Channels** feature, which is a research preview and may change. All channel interaction is isolated behind one interface, but that's the foundation.

---

## Requirements

- **Go 1.25+** (to build)
- **tmux** — AgentCorp spawns each agent into its own tmux window
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
| `↑` `↓` | move the cursor |
| `space` | fold / unfold a subtree |
| `⏎` | focus a subtree (breadcrumb back out) |
| `i` | inspect the selected agent |
| `d` | density: cards ↔ compact |
| `h` | hire an agent under the selection |
| `m` | message the selected agent |
| `/` | search by name / role / status |
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

**Phases 1–3 complete.** The chart renders, polls the live substrate every second, reports real vitals, and the full hire → bind → message → fire → disband lifecycle works. ~180 tests, single 11MB binary.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for setup and the bar for a change, and
[ARCHITECTURE.md](ARCHITECTURE.md) for how the codebase is organized. Security
reports go through [SECURITY.md](SECURITY.md), not public issues.

## License

MIT — see [LICENSE](LICENSE).
