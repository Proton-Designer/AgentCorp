# Architecture

AgentCorp is a terminal console that renders a company of persistent Claude Code
sessions as a live, operable org chart. This document describes how it's put
together and the invariants that hold it up.

## The load-bearing idea

**The hierarchy is AgentCorp's fiction. The mesh underneath is flat.**

`claude-peers` — the substrate AgentCorp builds on — knows nothing about parents,
children, or roles. Its broker stores only `{id, pid, cwd, git_root, tty,
summary}` per session. So every node in AgentCorp is **two things**:

1. A real OS process — an interactive `claude` session in a tmux pane.
2. A row in AgentCorp's own SQLite store — `{node_id, role, parent_id, peer_id, …}`.

Bound together by the peer id. **Every lifecycle operation is a real process
action plus a metadata edit**, and the two are reasoned about independently.
This is why, for example, firing a manager doesn't orphan its reports: the
children are independent processes; reparenting is purely a metadata edit (and
AgentCorp then messages the children so their behavior stays coherent with the new
chart).

## Package layout

```
cmd/agentcorp            entry point + first-run consent
internal/
  store             sidecar SQLite — hierarchy, roles, node state machine (ours)
  broker            READ-ONLY reader for claude-peers' DB + pure reconcile
  layout            Reingold-Tilford tree positioning — pure, no I/O
  sync              the tick loop: poll → diff → reconcile → apply
  vitals            derived state (active/quiet, throughput) — pure
  spawn             process launch — argv arrays, never shell strings
  lifecycle         tree surgery: reparent, disband — pure
  hire              orchestration: spawn → clear gates → bind; + consent
  msg               the one place AgentCorp writes to the substrate; origin classify
  ui                paint + Bubble Tea program
```

## Why so many pure packages

`layout`, `vitals`, and `lifecycle` do no I/O — tree in, result out. That
boundary is deliberate: it's what makes the riskiest logic exhaustively
table-testable without a live system. The hardest single piece is the layout
algorithm (a centered org-chart has no off-the-shelf terminal implementation),
and keeping it pure is what let it be verified against tens of thousands of
randomized trees.

## Data flow

```
                 ┌─────────────────────────────────────────┐
   ~/.claude-    │  sync.Tick (every 1s)                   │
   peers.db  ───▶│    broker.ListPeers (read-only)         │
   (substrate)   │    sync.ListPanes  (tmux)               │
                 │    broker.Reconcile ─► store writes     │
                 └───────────────┬─────────────────────────┘
                                 │ tea.Msg
                                 ▼
   ~/.config/    ┌─────────────────────────────────────────┐
   agentcorp/agentcorp.db ─│  ui.Model: BuildTree → layout → paint   │
   (ours)        │  vitals (HUD) · status glyphs           │
                 └─────────────────────────────────────────┘
```

## Key invariants

- **The substrate DB is read-only.** Only `internal/msg` writes to it, and only
  to insert a message row (the mechanism agents use to talk). Nothing else
  touches it. `internal/broker` opens it `mode=ro`, enforced by a standing test.
- **A failed poll is *unknown*, never *empty*.** A transient tmux or broker
  failure must never cascade into mass tombstoning of the org. The tick loop
  refuses to write on a failed poll, and the UI marks the view stale rather than
  redrawing as if the company vanished.
- **State transitions are guarded.** `pending → alive` is the only path into
  `alive` (via a bind); `dead` is only reached via a tombstone that also stamps
  the death time. Nodes are tombstoned, never hard-deleted, so a dead node still
  renders and its children keep a valid parent.
- **Binding is by tty.** A spawned session's peer id doesn't exist until it
  registers ~1s later, so AgentCorp matches the new peer to its pending node by the
  tmux pane's tty. (The tty is normalized across the `/dev/`-prefix boundary,
  and captured *after* the pane's process is replaced, since that reallocates
  the pty.)
- **The UI never overstates.** Inbound messages render as *queued*, not
  delivered (the substrate never acknowledges receipt). Agent status is
  *active*/*quiet* from message recency, not a guessed *working*/*idle*. Forged
  messages are *surfaced*, not blocked.

## Substrate dependency

Message delivery rides Anthropic's **Channels** feature (a research preview).
All channel-dependent behavior is isolated so a change to that feature affects
one seam rather than the whole codebase. See the README's "What it does not do"
section for the full set of substrate limitations AgentCorp documents rather than
papers over.
