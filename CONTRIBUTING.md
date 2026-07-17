# Contributing to AgentCorp

Thanks for your interest. AgentCorp is a terminal console for running a company of
AI agents on top of [`claude-peers`](https://github.com/louislva/claude-peers-mcp).
This guide covers how to get set up and what we expect from a change.

## Getting started

```sh
git clone https://github.com/Proton-Designer/AgentCorp && cd AgentCorp
go build ./cmd/agentcorp
go test ./...
```

Requirements: **Go 1.22+** and **tmux** (AgentCorp spawns agents into tmux windows).
`claude-peers` and Claude Code are needed to run against a live substrate, but
not to build or run the test suite.

## The bar for a change

AgentCorp has one non-negotiable principle: **it never tells the operator something
it can't back up.** Forged messages are surfaced, not claimed-blocked. Agent
status is `active`/`quiet` (derived from real signals), never a guessed
"working". Delivery is shown as "queued", never "delivered", because the
substrate never acknowledges receipt. If your change makes the tool assert
something it can't verify, it won't be merged.

Concretely, we ask that every change:

1. **Comes with a test that fails without it.** Bug fixes especially — the PR
   should include a test that fails on `main` and passes with the fix.
2. **Keeps the pure packages pure.** `internal/layout`, `internal/vitals`, and
   `internal/lifecycle` do no I/O by design — that's what makes the riskiest
   logic exhaustively testable. Don't add I/O to them; put it at the edges.
3. **Never writes to `~/.claude-peers.db`.** That database belongs to the
   substrate and is read-only to us. Only `internal/msg` and the pane-kill path
   touch the substrate at all, and only in the documented ways.
4. **Passes `go vet` and `gofmt`.** CI enforces both.

## Verifying behavior, not just tests

For anything touching the spawn/hire/tmux path, run it for real once —
`go build ./cmd/agentcorp && ./agentcorp` inside a tmux session — and confirm the
behavior you changed. Several of this project's sharpest bugs were invisible to
green tests and only surfaced by running the thing. "It typechecks" and "the
tests pass" are necessary, not sufficient.

## Submitting

- Branch from `main`, keep changes focused, one logical change per PR.
- Write a clear PR description: what changed, why, and how you verified it.
- Reference any issue the PR closes.

## Reporting bugs

Open an issue with the bug template. For anything involving spawned sessions or
tty binding, include the output of the bottom status line and, if you can, the
relevant rows from `~/.config/agentcorp/agentcorp.db` (`sqlite3 ~/.config/agentcorp/agentcorp.db
'SELECT name,state,peer_id,bind_tty,spawn_ref FROM nodes'`) — that's usually
where a hire failure explains itself.

## Security

Please do **not** open a public issue for security vulnerabilities. See
[SECURITY.md](SECURITY.md) for how to report them privately.
