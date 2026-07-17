# Security Policy

## Reporting a vulnerability

Please report security vulnerabilities **privately**, not through public issues.

Use GitHub's [private vulnerability reporting](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability)
(the **Report a vulnerability** button under the repository's **Security** tab),
or email the maintainer directly.

We'll acknowledge your report within a few days and keep you updated as we work
on a fix.

## Scope — what AgentCorp does and does not defend

AgentCorp is a client for `claude-peers`, which has **no authentication**. Some
properties are inherited from that substrate and are documented limitations,
not vulnerabilities:

- **AgentCorp cannot prevent forged messages.** Any local process can talk to the
  `claude-peers` broker directly and send a message claiming to be any agent.
  AgentCorp *surfaces* unmatched-origin messages; it does not block them. Blocking
  would require changes upstream in `claude-peers`.
- **AgentCorp spawns agents with `--dangerously-load-development-channels`.** This is
  the mechanism the whole tool is built on, disclosed in the first-run consent
  screen. It is opt-in by running AgentCorp.

Reports we *do* want:

- **Command injection** through any operator input (agent names, briefs,
  working directories) that reaches a shell. The spawn path uses argv arrays
  specifically to prevent this; a bypass is a real vulnerability.
- **Writes to `~/.claude-peers.db`** or any path that lets AgentCorp corrupt the
  substrate other sessions depend on.
- **Path traversal** or arbitrary file write via prompt files, config, or
  workdir handling.
- Anything that makes AgentCorp execute code an operator did not intend.

## Supported versions

AgentCorp is pre-1.0. Security fixes are applied to the latest release only.
