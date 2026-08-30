---
sidebar_position: 2
---

# Operator CLI: intents and grants

Since v0.12.0 the `korvun` binary carries the operator's authority tools:
**intent contracts** (what you authorize, with limits) and **authority
grants** (who may act under an intent, with less). They work against the
same local database the server uses, with brief, WAL-safe access — you can
run them while Korvun is serving.

Every mutation leaves an identified receipt in the action ledger: you as
the principal, in-process loopback evidence, and a finite audit rule —
`operator` for your acts, `attenuation_violated` for a delegation the wall
refused. Refusals are recorded too: the trail says why.

## Intents

An intent contract states an authorized outcome and its limits: a purpose
in words, an operation set, a coarse resource set, an optional action
budget and an optional validity window.

```sh
korvun intent create --config korvun.json \
  --purpose "test week" \
  --operations calc,time \
  --max-actions 10 \
  --expires 2026-09-06T20:00:00Z
```

Prints the new id (`int_…`) in `DRAFT`. Flags: `--purpose` and
`--operations` (comma-separated) are required; `--resources` defaults to
`*`; `--max-actions 0` means unlimited; `--valid-from` defaults to now;
no `--expires` means no expiry.

```sh
korvun intent activate --config korvun.json int_…
korvun intent revoke   --config korvun.json int_…
korvun intent list     --config korvun.json
korvun intent show     --config korvun.json int_…
```

The lifecycle is fail-closed and walked from the STORED state:
`DRAFT → ACTIVE → EXPIRED | REVOKED`. Terminal is terminal — re-activating
a revoked intent fails honestly, and the failed attempt leaves its receipt.
`show` includes the contract digest: a deterministic hash of the TERMS
(status excluded — revoking closes a contract's life, it does not rewrite
its identity).

## Grants

A grant gives one principal bounded authority under an intent.

```sh
korvun grant issue --config korvun.json \
  --intent int_… \
  --subject principal_brain_default \
  --operations calc \
  --max-actions 5 \
  --depth 1
```

The intent must be IN FORCE at the issuing instant: a `DRAFT` intent
denies with `intent_inactive`, an expired window with `intent_expired` —
the clock wins over a stale status, and the refusal is recorded.

```sh
korvun grant delegate --config korvun.json \
  --parent grant_… \
  --subject principal_ch_telegram \
  --operations calc
```

Delegation passes authority on — and **authority can only shrink**. The
child inherits the parent's intent, expiry and budget unless narrowed by
flags, is issued by the parent's subject with depth `parent − 1`, and must
be a subset of its parent in EVERY dimension: operations, resources,
budget, expiry, validity window, depth. A widening child is denied naming
the widened dimension and never touches the disk. The same wall governs
the kernel itself — the operator's CLI holds no special power here.

```sh
korvun grant revoke --config korvun.json grant_…
```

A revoked grant delegates nothing anymore (`authority_revoked`).

### Effect ceilings (v0.13.0)

A grant can carry an **effect ceiling**: the highest consequence class
its authority may reach, on the ladder `pure < read_external <
write_reversible < write_compensatable < write_irreversible < critical`.

```sh
korvun grant issue --config korvun.json \
  --intent int_… \
  --subject principal_brain_default \
  --operations calc \
  --effect-ceiling read_external
```

Delegation must shrink here too: the child inherits the parent's ceiling
unless narrowed, and a child reaching ABOVE it is denied naming
`effect_ceiling` — the tenth attenuation dimension, judged by the same
validator everywhere. Under a ceilinged (bounded) grant,
`write_irreversible` and `critical` actions also require human approval —
which, until the approval workflow ships, dies with the honest
`approval_unavailable`. Grants without a ceiling (the root's standing
authority and the config-derived grants) behave exactly as before.

## Reading the trail

Receipts live in the action ledger next to every other recorded action.
Each row carries its identity columns — principal, intent, authority —
and its per-attempt evidence (provider, credential kind, subject). No
secret material is ever stored: credential KINDS are a finite enum by
construction.
