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

### Notes
- Pre-1.0. The read/display side is stable; the spawn/hire path is newer.
