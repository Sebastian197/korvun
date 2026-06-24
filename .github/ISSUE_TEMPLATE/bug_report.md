---
name: Bug report
about: Report a reproducible problem in Korvun
title: "bug: "
labels: bug
assignees: ""
---

## Description

A clear and concise description of the bug.

## Steps to reproduce

1. Config used (redact secrets — never paste tokens or API keys)
2. Command run (e.g. `./korvun -config ...`)
3. What you sent / did
4. ...

## Expected behavior

What you expected to happen.

## Actual behavior

What actually happened. Include relevant log lines (structured `slog` output),
with any secrets removed.

## Environment

- Korvun version / commit:
- OS and architecture (e.g. linux/arm64, windows/amd64, darwin/arm64):
- Go version (`go version`):
- Channel(s) and model provider(s) involved:

## Additional context

Anything else that helps — minimal reproduction, related ADRs, etc.

> ⚠️ Do not include secrets (bot tokens, API keys) anywhere in this issue. If a
> secret was exposed, revoke it first.