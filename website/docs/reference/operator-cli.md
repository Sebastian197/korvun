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
`write_irreversible` and `critical` actions also require human approval.
With `approvals.enabled` set, that requirement PARKS the action as a
pending request you decide in the inbox below; without it — and if the
park itself fails — the call still dies with the honest
`approval_unavailable`. Grants without a ceiling (the root's standing
authority and the config-derived grants) behave exactly as before.

## The verifier (v0.14.0)

Since v0.14.0 every terminal outcome leaves a signed receipt on an
append-only hash chain, and the CLI carries the judge.

```sh
korvun receipt verify --config korvun.json rcpt_…
```

One receipt (or every receipt of an `act_…` id), re-judged offline
against the store file. What the ladder judges, in the order it judges it: the
canonical roundtrip, the hash recomputed, the signing key found in the
registry, the Ed25519 signature against that key, the key's validity
window, the chain link to its predecessor, then — when the receipt
seals an approval digest — the approval and its tombstone, and last the
coherence with the action row. `receipt verify` prints EVERY failure it
finds, one line each, in that order; `korvun ledger check` below prints
only the FIRST failure of the first receipt that fails. Each failure the ladder judges carries its name
(`hash_mismatch`, `signature_invalid`, `custody_mismatch`, …) rather
than a generic "invalid"; a receipt whose stored bytes do not parse is
refused with the read error and no ladder name.

```sh
korvun ledger check --config korvun.json
```

One partition's whole chain, structure first: a receipt deleted from
INSIDE the chain is denounced by its hole (`chain_seq_gap` with the
missing position), a cloned position as `chain_seq_duplicate`, then
every link through the same checks — the FIRST broken link stops the
verdict with its receipt id and reason. What it proves is that the
profile your config names is consistent with itself: a tail cut, a
chain re-signed with a key the attacker registered inside that profile,
and a config pointed at another store all survive, and each needs an
external anchor Korvun does not ship yet.

```sh
korvun receipt rotate-key --config korvun.json
```

Atomic retire-and-activate rotation of the profile's signing key. The
rotation act leaves its OWN receipt sealed with the NEW key; retired
keys are kept forever, so each era of the chain verifies with the key
of its era. Verification is read-only; the honest scope is documented:
the ledger is tamper-evident, never "immutable". The scope is not an
access boundary but a property: the commands prove the profile is
consistent with itself. The four things they do NOT prove: that the
profile is the one your history wrote, that the chain is complete, that
its keys are the authoritative ones, and that a row whose lookup column
changed storage class is present rather than absent. The project's
security notes carry them in full.

## The approvals inbox (v0.15.0)

When `approvals.enabled` is on, an action whose effect class demands a
human yes no longer dies with the honest `approval_unavailable`: it
PARKS as a pending request with a sealed preview.

**Since v0.15.0 you decide from TWO places, and they are the same
decision.** The Aprobaciones screen in Korvun Desktop lists what is
parked, shows the whole document and takes the yes behind a typed gate;
this page documents the CLI, which does the same thing from a terminal
and is the only way in on a headless server. Both touch the same store,
the same belts and the same claim that consumes the stored parameters
once: whichever decides first, the other is told so by name. That single
consumption has two known limits. In v0.15.0, a trigger that restores the
parameters inside the claim's own transaction defeats it; that is cured in
v0.15.1, the current release. And if another connection writes the
parameters back after the claim commits, while the action is still
APPROVED — a first execution still running, one whose close failed, or a
claim followed by a crash before the close — a second execute runs the
effect again; a restore after the action closed gets no second run. That
second limit is a known issue filed for v0.15.2.

The parking needs a BOUNDED brain: set `agent.effect_ceiling` on the
brain (for example `"write_reversible"`) — the missing cable landed
with this stage: absent means unbounded, exactly as before, and then
nothing parks.

```sh
korvun approvals list --config korvun.json
```

Every request with its status and expiry — consults go through the
read-only door: no schema migration, no crash recovery, and a sealed
connection that refuses every write at the SQLite level. It is not a
promise that the file on disk is untouched — the open itself creates a WAL
store's sidecars, and rewrites the journal header of a store left in
another mode.

```sh
korvun approvals show --config korvun.json apr_…
```

THE DIGEST you approve, first and prominent; then the full preview —
purpose, actor and delegation position, operation, resources, what
data leaves, cost, effect class and reversibility, the pinned law —
and the RAW parameters. They are kept only in the local store, and the
approvals API serves them too, on the admin server's address
(`observability.addr`, loopback by default).

```sh
korvun approvals approve --config korvun.json apr_…
korvun approvals reject --config korvun.json --comment "why" apr_…
```

Both are recorded operator acts with their own signed receipts.
Approving executes THE stored object — recovered whole, re-verified
against the approved digest, claimed atomically so two racing approvals
do not both obtain the parameters (not against a restore committed while
the action is still APPROVED, the known issue filed for v0.15.2) — and
reports the real outcome; the
receipt of an approved action seals its approval reference (canonical
v2), and `receipt verify` gains the `approval_mismatch` check.
Rejection, cancellation or expiry close the parked action with a
receipt and no execution path remains. Requests expire on their TTL
(default 1h, `approvals.ttl`), judged at the decision touch.

## Strict authority (v0.16.0)

Off unless the profile asks for it. With it on, an effectful start needs a
verified authenticated principal, one active signed intent and a complete active
signed authority chain, all judged inside the transaction that commits the
start. Turning it on is five operator acts, in this order, each leaving its own
signed receipt:

```bash
korvun intent create-v2   --config korvun.json --file intent.json
korvun intent activate-v2 --config korvun.json int_pedidos 1
korvun authority admin-issue --config korvun.json --file grant.json \
                             --reason "why this authority exists"
korvun intent bind        --config korvun.json --actor principal_brain_ops \
                          --channel console --grant grant_pedidos_root int_pedidos 1
korvun authority activate --config korvun.json --profile profile_ops \
                          --reason "why this profile goes strict"
```

The last command prints an `activation_digest`. It goes in the config, and the
profile is strict from the next boot:

```json
"authority": { "mode": "strict", "activation_digest": "sha256:…" }
```

`authority` also has `issue`, `delegate`, `revoke` and `import-v1`, each with an
`admin-` form (`admin-issue`, `admin-delegate`, `admin-revoke`). BOTH forms of
`revoke` require `--reason`, and so does every administrative form; what the
administrative ones add is the human operator recorded separately from the
grant's issuer. The ordinary `issue` requires the actor to be the intent's own
owner, and from the CLI the actor is always `principal_local_operator` — so on
an intent owned by anything else, ordinary `issue` refuses with
«action/sqlite: authority issuer mismatch» and `admin-issue` is the door.

**What the recipe above does not say, and needs.** On a profile upgraded from an
earlier release the first command refuses until the server has booted once, to
lift the store's schema. `grant.json` must carry the `intent_digest` of the
exact intent version, and no CLI verb prints it: today it is read from the store
by hand. And the shape of `intent.json` and `grant.json` is not documented yet,
while both parsers refuse unknown or duplicated fields. All three are filed.

**What an intent must say under strict mode.** `read_file`, `http_fetch` and
`webhook_call` are CLOSED WORLD there: they start only under terms that list the
resources, the data tags and the destinations they may touch. An intent with no
`allowed_resources` grants none, and the start refuses by name —
«action: resource out of authority scope», or «action: authority use
unresolved» when the arguments cannot be resolved at all — that literal text,
not a short code. A `read_file` path that is not absolute is unresolved, because
this layer does not know the jail root the tool would join it to. And the scope
belongs to the INTENT, not to the operation: once an intent lists any resource,
every operation WITHOUT a registered analyzer refuses too — `memory_note`
included — so an intent scoped for `read_file` silently closes the others.

**What the human sees.** A request parked under a strict profile carries the
`AUTORIDAD` block in the approval document: who asked, under which contract,
through which chain of principals, and the budget that remained WHEN IT WAS
PARKED — read from a signed snapshot and verified against that signature on
every read, not a live meter.

**`--grant`, and what happens without it.** The `--grant` above is what ties the
signed grant to the binding, and it is the flag that decides which authority the
profile resolves through. Its value is the `grant_id` inside the file you issued.
The bind is refused, before it writes anything, unless all of these hold:

- the grant is ACTIVE, and so is every grant above it in its chain;
- its subject is the `--actor` you name;
- **every grant in that chain carries the `--channel` you name**;
- the intent is active and matches the digest the grant was issued against.

The channel is on that list because a start checks it too: a binding written on
a channel the grant does not carry would be refused at every start, and there is
no reason to let you write one.

Bind WITHOUT `--grant` and the binding carries no grant, so a strict profile
resolves its authority through the config clause derived from the brain's tool
list while the grant you issued sits ACTIVE and unused. Nothing starts outside
the intent's scope either way — the clause path verifies the same terms — but
delegation, attenuated child grants and shared ancestor budgets only enter the
path of an execution through `--grant`.

**Binding again WITH `--grant` replaces the binding OF THE SAME SELECTOR rather
than failing:** the previous one is kept as REVOKED — it is the record of what
authorised yesterday's action — and the new one is written at the next revision.

The selector is `--actor` + `--channel` + `--conversation`, and that third part
matters more than it looks. `--conversation` is optional; omitted, it writes the
ANY-CONVERSATION binding, and a start falls back to that one only when the
conversation it is running has no binding of its own — a named conversation
always wins.

So the two directions are both narrower than they look, and neither replaces the
other:

- Binding WITH `--conversation` replaces only that conversation's binding. Every
  other conversation, and the any-conversation binding, are untouched.
- Binding WITHOUT it replaces only the any-conversation binding. **Every
  conversation that has a binding of its own keeps resolving through it**, which
  means the old grant keeps authorising them.

**There is no single command that replaces every binding of one actor and
channel.** If you are rotating a grant because it was compromised or attenuated,
re-bind each conversation that has its own binding, and the any-conversation one
too. Listing them is not possible from the CLI today; both gaps are filed.

The `revoked binding` line tells you the selector you named had a holder, and its
absence tells you that selector was free — it does NOT tell you whether other
selectors still hold the old grant.

The command names the replacement, and names the previous binding too when there
was one:

```
revoked binding bind_act_5f1ce33dab70ba5918800de9ad4bf067
binding bind_act_3fd7470b7d2e62d0857d8c36daa46b79 -> int_pedidos version 1 ACTIVE under grant grant_pedidos_root
```

A first bind on a free selector prints only the second line.

**Binding again WITHOUT `--grant` does NOT replace it.** That path is a plain
insert and the selector already has an ACTIVE row, so it stops on the database's
own uniqueness rule and prints it raw:

```
korvun intent bind: constraint failed: UNIQUE constraint failed: index 'execution_bindings_active_selector' (2067)
```

No binding is written and nothing is lost — the refused act is recorded in the
ledger as FAILED, which is what that record is for — but there is no way back to
the config clause from the command line today. Giving the grant-less path the
same replace-in-place behaviour is filed.

`--grant` with an empty value is a usage error, not a bind without a grant.

**If a bound grant is later revoked**, every start under that binding is refused
until you bind again **with another `--grant`**. The binding is not repaired for
you, and nothing else repairs it.

## Reading the trail

Receipts live in the action ledger next to every other recorded action.
Each row carries its identity columns — principal, intent, authority —
and its per-attempt evidence (provider, credential kind, subject). No
secret material is ever stored: credential KINDS are a finite enum by
construction.
