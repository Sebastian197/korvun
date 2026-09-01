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

## The verifier (v0.14.0)

Since v0.14.0 every terminal outcome leaves a signed receipt on an
append-only hash chain, and the CLI carries the judge.

```sh
korvun receipt verify --config korvun.json rcpt_…
```

One receipt (or every receipt of an `act_…` id), re-judged offline against the store file with seven named
checks: canonical roundtrip, hash recompute, Ed25519 signature against
the REGISTERED public key, the key's validity window, the chain link to
the predecessor, and coherence with the action row. Every failure
carries its name (`hash_mismatch`, `signature_invalid`,
`custody_mismatch`, …) — never a generic "invalid".

```sh
korvun ledger check --config korvun.json
```

The whole chain, structure first: a deleted receipt is denounced by its
hole (`chain_seq_gap` with the missing position), a cloned position as
`chain_seq_duplicate`, then every link through the same seven checks —
the FIRST broken link stops the verdict with its receipt id and reason.

```sh
korvun receipt rotate-key --config korvun.json
```

Atomic retire-and-activate rotation of the profile's signing key. The
rotation act leaves its OWN receipt sealed with the NEW key; retired
keys are kept forever, so each era of the chain verifies with the key
of its era. Verification is read-only; the honest scope is documented:
the ledger is tamper-evident, never "immutable" — the operator controls
storage and keys.

## The approvals inbox (v0.15.0)

When `approvals.enabled` is on, an action whose effect class demands a
human yes no longer dies with the honest `approval_unavailable`: it
PARKS as a pending request with a sealed preview, and the CLI is where
you decide.

The parking needs a BOUNDED brain: set `agent.effect_ceiling` on the
brain (for example `"write_reversible"`) — the missing cable landed
with this stage: absent means unbounded, exactly as before, and then
nothing parks.

```sh
korvun approvals list --config korvun.json
```

Every request with its status and expiry — consults go through the
read-only door (no migration, no recovery, nothing written).

```sh
korvun approvals show --config korvun.json apr_…
```

THE DIGEST you approve, first and prominent; then the full preview —
purpose, actor and delegation position, operation, resources, what
data leaves, cost, effect class and reversibility, the pinned law —
and the RAW parameters (loopback only: they exist nowhere else).

```sh
korvun approvals approve --config korvun.json apr_…
korvun approvals reject --config korvun.json --comment "why" apr_…
```

Both are recorded operator acts with their own signed receipts.
Approving executes THE stored object — recovered whole, re-verified
against the approved digest, claimed atomically so racing approvals
cannot fire the effect twice — and reports the real outcome; the
receipt of an approved action seals its approval reference (canonical
v2), and `receipt verify` gains the `approval_mismatch` check.
Rejection, cancellation or expiry close the parked action with a
receipt and no execution path remains. Requests expire on their TTL
(default 1h, `approvals.ttl`), judged at the decision touch.

## Reading the trail

Receipts live in the action ledger next to every other recorded action.
Each row carries its identity columns — principal, intent, authority —
and its per-attempt evidence (provider, credential kind, subject). No
secret material is ever stored: credential KINDS are a finite enum by
construction.
