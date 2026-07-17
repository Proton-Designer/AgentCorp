## What this changes

## Why

## How I verified it
<!-- Tests are necessary but not sufficient for anything touching spawn/tmux.
     If you changed runtime behavior, say how you ran it and what you observed. -->

- [ ] `go test ./...` passes
- [ ] `go vet ./...` and `gofmt -l .` are clean
- [ ] For a bug fix: added a test that fails without this change
- [ ] Pure packages (layout/vitals/lifecycle) stayed I/O-free
- [ ] Nothing writes to `~/.claude-peers.db`
