# Korvun v0.15.0 — Beta: the human yes, in the window

The fifth release of the Execution Trust Layer era, and the one that brings
approvals. They are opt-in: with `approvals.enabled` set, an irreversible action
under a bounded ceiling PARKS as a request until a human decides it, in the
desktop window or with the operator CLI, or until it expires (default TTL one
hour), against the same store, the same belts and the same claim that
consumes the stored parameters once. That single consumption has two known
limits in this release, both reproduced at `61ae582`, the base of the v0.15.1
trains, whose Go code is byte-identical to this tag's. A trigger that restores
the parameters inside the claim's own transaction defeats it (two executions,
`runs=2`); fixed on master for v0.15.1. And another connection that writes the
parameters back after the claim commits, while the action is still APPROVED (a
first execution still running, one whose close failed, or a claim followed by
a crash before the close), followed by a second execute, runs the effect again
(`runs=2`); a restore after the action closed gets no second run. That second
limit is filed for v0.15.2.
If the park itself fails, or the request's provenance cannot be resolved, it
falls closed to the same denial. Without the setting that action is denied with
`approval_unavailable`, as it has been since v0.13.0; no earlier release could
park it or decide it.

## The screen

The Aprobaciones section lists what is parked and, on opening one, serves the
whole document: the digest in eight groups of eight, the operation, the origin,
the purpose, the principal, the reversibility, the tool cage, the pinned law
and the RAW parameters the model wrote.

**Why the literal and not the chat.** In the release ceremony the director typed
«hola-ceremonia» into the chat. The model wrote the call with
`{"message":"hola-ceremony"}`, the screen showed that literal before anyone
approved, and after the yes the receiver logged
`{"message":"hola-ceremony"}`. The stored action digest re-derives over
exactly those bytes and not over the text that was typed. What leaves is what
the model wrote, so the document shows that, not a paraphrase of the
conversation. The ceremony is narrated, not evidenced: the commit carries no
store, ledger or raw capture that binds this account to a real run (P2-11 under
Known issues).

**The yes is behind a typed gate.** You re-type the last six characters of the
digest; pasting does not arm it and neither does the browser's autofill. The no
is always one click away and Esc rejects while the request is open and
undecided. The two doors are deliberately unequal — measured in the browser at
6 720 px² against 16 640, with 354 px between them at the window's 1100 px, and
with Aprobar at half of Rechazar's width when they stack — because the cheap door
should be the big one.

**Untrusted bytes are rendered as escapes.** Every field of uncontrolled origin
goes through it, so a `U+202E` cannot make the document read `hooks.acme.io`
while the digest seals something else.

## What it refuses, and by name

Twenty-one named outcomes, each with its own operator text. The screen renders
BY NAME, never by the English of a body, and an answer it cannot read is said
to be unreadable instead of degraded into an empty list.

Three distinctions this release insisted on:

- **A row born without arguments** is not a row somebody emptied. The first
  re-derives its digest over the empty body; the second does not, and only the
  second can mean an execution already started somewhere else.
- **A tool that ran and said no** is a decided outcome with its receipt, not an
  unknown one — and a call that TIMED OUT is the opposite: it may have been
  delivered and its answer lost, so the ledger closes it `OUTCOME_UNKNOWN` and
  the window says it does not know.
- **A moved law** is permanent and repairable from the profile; the literal
  tells the operator that rejecting still works.

## What the store gained

A list door that serves every row it can serve — a row it cannot scan whole is
skipped, COUNTED and NAMED, where before one corrupt timestamp erased the
healthy rows too. A detail door that reads state, operation triple, parameters
and belts inside ONE transaction, so a legitimate CLI rejection landing
mid-read can never be painted as corruption. And typed sentinels, so a caller
names what refused instead of matching an English sentence.

## Four defects this train found in the tree and cured

- **A TOCTOU on the path that fires an irreversible effect.** The digest was
  compared against an operation triple read two transactions earlier, with the
  approval already consumed. It is now judged inside the claiming transaction,
  over the row that transaction read. **From the window**, the digest travelling
  there is the full digest the detail served to the screen; the six characters
  the operator types only arm the button and do not travel (P2-5 under Known
  issues). From `korvun approvals approve|execute` it is the column the command
  just read: that command takes no digest flag, so there is no re-typed string
  to carry.
- **A delivered POST recorded as a refusal.** `webhook_call` closed the ledger
  `FAILED` — a definite claim that the irreversible call did not happen — for
  four outcomes in which the host already had the body: an answer cut short, an
  answer over the size cap, a 3xx the cage will not follow, and **an HTTP error
  status**. All four now close `OUTCOME_UNKNOWN`.

  **This changes what an operator sees in the commonest case of all.** A
  webhook that answers 500 used to read «la acción se ejecutó y falló»; it now
  reads «no sabemos si el efecto llegó a ocurrir», and the ledger agrees. The
  error text is unchanged, so the 500 is still on screen and in the receipt —
  what is gone is the claim that nothing happened, which nobody could support.
  A refusal that stops the call before it leaves — a host off the allow-list,
  the private-network shield, a malformed body, a failed dial — still closes
  `FAILED`, and that is still the honest word for it.
- **The list fell over whole** on a single unparseable timestamp.
- **A refusal with no name** when the parked action moved out from under a
  decision: the whole transaction rolls back, so the answer says that and not
  a word about effects.

## What does NOT ship

- The approvals surface is mounted wherever an action store is open and an
  admin token resolves; **a profile without a token does not serve it at all**.
- The ceiling rule — a parkable tool ranked ABOVE the brain's ceiling — is
  implemented and **UNWATCHED**: no builtin tool declares the `critical` class
  today, so any mould for it would pass for the wrong reason. Filed.
- **Filed with their reproduction** for v0.15.1: `Detail` takes two reads and
  says so; `brain_gone` emitted after a committed decide carries a literal
  written for the branch before it; the same expired row answers `expired` by
  the read door and `already_decided` by the write door; `korvun approvals
  approve` has no `--digest` flag, so the CLI stands behind the column it read
  rather than behind a human's re-typed string; and `ApprovalListing` computes
  `PreviewReadable` and `ChannelKnown` that no consumer reads, so a corrupt
  preview and an empty channel reach the screen as the same empty fields.

## Known issues, filed to v0.15.1

Nothing here is softened. Each entry is a defect this release ships with, and
each carries the reproduction that found it.

**From the fifteenth external pass (8 P2 + 2 P3), reproduced in `docs/HANDOFF.md`
under the same IDs:**

- **P2-4.** A deadline that fired BEFORE the request was written closes
  `OUTCOME_UNKNOWN` on both paths — a TLS handshake that never completes proves
  the POST never left, and the ledger still says it does not know.
- **P2-5.** The digest the window sends is the one the detail served, not the six
  characters the operator typed; the chain is safe against parameter TOCTOU,
  but the declared provenance is not what the screen implies.
- **P2-6.** The same corrupt bytes are classified `evidence_corrupt` by
  `GetApproval` and left untyped by the decide path, so one corruption can be
  published as `unavailable`.
- **P2-7.** The approvals API rides `observability.addr`, which accepts
  `0.0.0.0`, while every decision receipt is signed
  `CredentialLoopbackInProcess`. The prose is cured in this release; the
  receipt is not.
- **P2-8.** `current_law_digest` is resolved from `cfg.Brains[0]`, so a second
  brain's request is shown the first brain's law.
- **P2-9.** Any SQL failure inside `transitionTx` is published as
  `already_closed`, over a transaction that rolled back and an action that is
  still approvable.
- **P2-10.** Four named outcomes (`not_found`, `unavailable`, `disabled`,
  `params_digest_mismatch`) reach the screen as «respuesta que esta pantalla no
  reconoce» when they arrive from approve or reject; and any 200 whose
  `outcome` is not exactly `failed` is painted as «Ejecutada».
- **P2-11.** The ceremony behind this release's screenshots is not verifiable
  from the commit: the canto narrates four receipts, `chain intact` and two
  verifications, but the tree carries no capture, ledger, raw output, database
  or hash that binds those sentences to real executions.
- **P3-12.** Approve and reject accept trailing JSON and unknown fields.
- **P3-13.** Two minor mismatches: a comment in
  `internal/controlapi/approvals_test.go` claims that the only emitter of
  `unknown_outcome` is the default of `nameInBandRule`, which the store never
  reaches, while `internal/app/approvals_adapter.go` also emits it from
  `runApproved` when a run's outcome is unknown; and the
  four post-delivery branches of `webhook_call` are covered in `internal/tool`,
  but no mould composes them across both execution paths.

**From this release's own passes:**

- `korvun approvals show` prints the raw parameters without escaping
  invisibles: the screen's alphabet is not the CLI's.
- The window publishes `not_started_params_held` for a claim refused by a moved
  authority. True for the trigger-driven shapes that were executed; a real race
  that commits `actions.state` between the prechecks and the claim was NOT
  reproduced, and its narration is unverified.
- The ceremony that produced this release's screenshots recorded two POSTs on
  its receiver with a body the ceremony never produced. The core's log holds
  four parked requests and zero executions, and the origin of those two POSTs
  is NOT identified.
- **The four class cures ship reviewed by nobody twice.** The internal adversary
  ran ONE pass over their diff and ended VETO MANTENIDO with nine findings; by
  the director's rule all nine were cured in that same commit and **no re-pass
  ran**, so the adversary never saw the cures. The external gate has not run on
  them either: it fires after this tag, on the published release, and its
  findings become the v0.15.1 plan. The train is IMPLEMENTED, not VERIFIED by
  either gate, and the marker commit says so in its own words.
- **`TestEventHook_publishesReceivedThenSent` (`internal/router`) fails
  intermittently on macOS.** Captured failing on the pull request while the same
  bytes passed on the rehearsal run of the same head, with twenty local `-race`
  runs green. **Not diagnosed.** Filed with its evidence in `docs/HANDOFF.md`.
- **One escape ships with no probing mutation, by construction.** The readable
  expiry is escaped, but that branch renders only when `parseExpiry` accepted
  the string as an instant, and an instant carries neither `<` nor an invisible
  — so no mutation can redden it. It is defence in depth, declared here rather
  than proven by a captured red. Every other guard of this release has its
  mutation.

**Observed, and not a Korvun defect:**

- The local model that drove the ceremony answered two of six requests in prose
  with no tool call, and nested its JSON body inside `message`. The screen
  prints that literally, which is its job; the model's unreliability is what a
  small local model does, not a defect of Korvun.

## Verification

```sh
korvun ledger check --config korvun.json
korvun receipt verify --config korvun.json rcpt_…
```

Every artifact of both families is covered by a checksums file signed keyless
with cosign, and the headless family also ships an SBOM per archive:

```sh
cosign verify-blob checksums.txt \
  --signature checksums.txt.sig \
  --certificate checksums.txt.pem \
  --certificate-identity-regexp 'https://github.com/Sebastian197/korvun/.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com

gh attestation verify checksums.txt --repo Sebastian197/korvun
```

(Desktop family: the same commands over `checksums-desktop.txt`.) Full
walkthrough: [`docs/packaging/INSTALL.md`](../packaging/INSTALL.md) and
[`SECURITY.md`](../../SECURITY.md).

The release is published only when BOTH families and their signatures are
present — the draft-until-complete rule: a red lane leaves an invisible draft
rather than a half-signed release.
