# Governed tools + skills — operator guide

> Governing decisions: ADR-0021 (the agent loop), ADR-0041 (governance,
> shadow mode, the network shield, caged tools, skills). Config reference:
> `docs/CONFIGURATION.md` → `brains[].agent`.

Korvun's agent brains can use tools — and the **policy engine is the
gatekeeper**: you decide which tool each brain may use, on which channel,
in which mode, and every use, rehearsal, and denial is audited exactly like
routing decisions. Skills are markdown files that teach a brain WHEN to use
its tools; they never grant anything.

## The toolset

| Tool | What it does | Reach | House class |
|------|--------------|-------|-------------|
| `time` | Current UTC time. | none (pure) | — |
| `echo` | Returns its args. | none (pure) | — |
| `calc` | Bounded arithmetic. | none (pure) | — |
| `read_file` | Reads a text file. | ONLY under your configured `root` (symlink escapes die at the resolved-path check); size-capped. | **sensitive** (local models only) |
| `http_fetch` | HTTP GET. | ONLY your `allow_hosts`; response-capped; redirects only to listed hosts, hop-capped. | **network** |
| `webhook_call` | HTTP POST of a JSON payload — your no-code tool factory (n8n flows, home automation, any webhook). | ONLY your `allow_hosts`; response-capped; hard timeout; NO redirects. | **network** |

Shell execution does not exist and will not resolve — by decision, not
omission (ADR-0021 §8).

A caged tool cannot exist without its cage: listing `read_file` without the
`read_file` config block is a boot error, never a default.

## The gatekeeper

Grants are per-brain, tri-state:

- **`allow`** — the tool is announced to the model and executes.
- **`shadow`** — the tool is ANNOUNCED but NEVER executes: the model's
  intention is recorded (audited as `tool_shadowed`) and the model receives
  an honest simulation observation. **Use shadow to watch a brain's real
  judgment before trusting it**: grant `shadow`, watch the Activity feed or
  `/tools`, and when you like what you see, hot-apply the grant to `allow` —
  no restart.
- **`deny`** — neither announced nor executable.

Restrictions always win over the granted mode: a channel restriction, the
sensitivity rule (a `sensitive` tool never reaches a cloud-model brain), and
the cages apply even to `allow`. A brain with NO `governance` block is
ungoverned: every listed tool allowed on every channel — exactly the
pre-governance behavior.

## The network shield (private brains)

A brain declared `"sensitivity": "private"` gets the shield on its network
tools: `http_fetch` and `webhook_call` may only reach PRIVATE addresses
(loopback, RFC1918, IPv6 ULA/link-local) — AND still only allow-listed
hosts. The check runs on the RESOLVED IP at connection time, so DNS
rebinding and redirects toward the public internet die at the socket with
nothing sent. The shield restricts, never widens: a public host on your
allow-list is still denied (`private_network_shield` in the audit).

## The audit — and `/tools`

Every use, shadow, and denial lands on three surfaces: structured logs,
Prometheus metrics (`korvun_tool_calls_total{tool,outcome}` + a latency
histogram), and the Activity feed (`tool_used` / `tool_shadowed` /
`tool_denied` events — metadata only: brain, tool, channel, outcome, rule,
latency; NEVER the tool's arguments).

From the desktop chat's console channel, send **`/tools`** to get the
gatekeeper report of that conversation's brain: its effective grants (with
channel restrictions, sensitivity, and the shield) and the recent tool
activity. It is a system response — no model involved.

## Skills — teaching WHEN

A skill is a directory with a `SKILL.md` (the open AgentSkills format):

```markdown
---
name: home-assistant
description: Teaches when to call the home automation webhook. Use when the user asks to turn things on or off.
---

# Home automation

When the user asks to switch a device, call webhook_call with the
n8n flow URL and a JSON body like {"device": "...", "state": "on"}.
Confirm to the user what was switched.
```

Rules (validated at load):

- The directory name MUST equal the frontmatter `name` (1–64 chars,
  lowercase letters/digits/single hyphens).
- `description` is required (1–1024 chars): say what AND when.
- Frontmatter parsing is deliberately FLAT: first-level `key: value` lines
  only (`name`, `description`, `license`, `compatibility`,
  `allowed-tools`); nested blocks like `metadata:` are skipped with
  tolerance. Keep required fields on single lines.
- `allowed-tools` is recorded but NEVER grants anything — skills are
  documentation; the gatekeeper's decision is final.
- `SKILL.md` is capped at 64 KiB. A malformed skill is skipped with a
  warning at boot (the log names the violation); it never stops Korvun.

Point the brain at the directory with `skills_dir`. Skill names and
descriptions always join the agent's system prompt; bodies are included
under `skills_body_budget` total runes (default 8192) — oversized bodies
are omitted with a warning, the skill stays listed.

## A worked example

```json
{
  "name": "casa",
  "sensitivity": "private",
  "policy": {"kind": "priority"},
  "models": [{"provider": "ollama", "model_id": "llama3.2", "locality": "local"}],
  "agent": {
    "tools": ["time", "calc", "read_file", "webhook_call"],
    "governance": [
      {"tool": "time", "mode": "allow"},
      {"tool": "calc", "mode": "allow"},
      {"tool": "read_file", "mode": "allow", "channels": ["console"]},
      {"tool": "webhook_call", "mode": "shadow"}
    ],
    "read_file": {"root": "/home/chano/korvun-notes"},
    "webhook_call": {"allow_hosts": ["192.168.1.20:5678"]},
    "skills_dir": "/home/chano/korvun-skills"
  }
}
```

This brain: reads notes only from one folder and only from the console
channel; REHEARSES the n8n webhook in shadow (watch `/tools`, then promote
to `allow` with a hot apply); and — being private — could never reach a
public address through a network tool even if the allow-list said so.
