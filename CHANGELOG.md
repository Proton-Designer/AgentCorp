# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Live org-chart TUI: centered Reingold-Tilford tree of the agent hierarchy,
  polling the `claude-peers` substrate once per second.
- Vitals HUD (agents, active/quiet, unmanaged, message-rate sparkline, uptime)
  and an activity ticker.
- Full lifecycle: hire (spawn → clear startup gates → bind by tty), message,
  fire (with child reparenting + notification), and disband (cascade).
- First-run consent flow for the development-channels warning, with the grant
  recorded locally and re-asked on a version change.
- Origin classification of inbound messages (known / unknown / forged) —
  surfaced, never blocked.
- Revive dead agents from their on-disk session memory — one (`z`) or all
  (`shift-Z`) at once.
- **Living-company visual layer** — a decoupled ~10 fps animation engine (frame
  clock independent of the 1 Hz data poll, cached base grid + sparse overlay) with
  a motion budget (`v`: off / calm / lively):
  - Message-flow pulses that ride the connector wires in a message's direction
    (real broker rows only; the pulse stops before the destination card).
  - Speech bubbles showing the selected agent's last message, honestly aged.
  - Breathing status LEDs and card borders, driven by the active/quiet signal.
  - A scrolling newswire by agent name over a heartbeat activity monitor.
  - An office / floor-plan view (`o`) and a mission-control dashboard (`g`).
  - A selected-card highlight on the chart, and a cinematic boot sequence.
  - Everything animated is a real substrate signal — the pretty view is as honest
    as the plain one.

- **Self-healing supervision** — automatic, policy-driven, budget-bounded fault
  tolerance over the memory-intact revival: one-for-one / one-for-all / rest-for-one
  restart policies, restart budgets, and escalation-to-human rather than autonomous
  cascade-kill (a deliberate divergence from OTP for expensive, non-idempotent LLM
  agents). Opt-in with `shift-S`; decide-and-show by default. Pure policy engine in
  `internal/supervision`.
- **The epistemic auditor** (`internal/audit` + a standalone `audit` tool) — a passive
  layer that detects verification theatre in an agent mesh: checks that pass regardless
  of correctness, measurements equal to their own cap, agreement between sources that
  share a blind spot, capabilities no test reaches through the real transport, and
  documented invariants with no test. Diagnostic, never a score. Validated by a
  cross-agent, blind-graded verification loop (see `docs/design/proof-carrying-claims.md`).

### Notes
- Pre-1.0. The read/display side is stable; the spawn/hire path is newer.
