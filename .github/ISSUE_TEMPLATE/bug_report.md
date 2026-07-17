---
name: Bug report
about: Something isn't working
labels: bug
---

**What happened**
A clear description of the bug.

**What you expected**

**Steps to reproduce**
1.
2.

**Environment**
- OS:
- Go version (`go version`):
- tmux version (`tmux -V`):
- Running inside tmux? (required for hire/fire)

**If it's a hire/spawn failure**
Paste the bottom status line, and:
```
sqlite3 ~/.config/crew/crew.db 'SELECT name,state,peer_id,bind_tty,spawn_ref FROM nodes'
```
