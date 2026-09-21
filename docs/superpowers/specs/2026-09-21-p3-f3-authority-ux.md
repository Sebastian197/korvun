# Piece 3, phase 3: approval authority block UX

> **Status:** approved by Chano through the explicit implementation commission
> of 2026-09-21. The commission fixed the location, facts, source, escaping,
> word `máximo`, jsdom coverage, and real-browser coverage before RED.
> **Law:** `CLAUDE.md` UX-DESIGN-FIRST and MANOS-DE-CHANO. This document is the
> text mockup of the one visible change; it introduces no new control.

## What the operator sees

The pending approval document keeps its current order through `ORIGEN`. Directly
below it, and before `LA LEY QUE LO EXIGIÓ`, it gains this read-only section:

```text
┌──────────────────────────────────────────────────────────────────────┐
│ ORIGEN                                                               │
│ Pay the supplier invoice                                             │
│ principal_agent_accounts                                             │
├──────────────────────────────────────────────────────────────────────┤
│ AUTORIDAD                                                            │
│                                                                      │
│ QUIÉN PIDIÓ                                                          │
│ principal_alice                                                      │
│                                                                      │
│ BAJO QUÉ CONTRATO                                                    │
│ int_supplier_payments_v3                                             │
│ Pay approved supplier invoices                                       │
│                                                                      │
│ CADENA                                                               │
│ operator_chano → agent_accounts → agent_payments                      │
│                                                                      │
│ PRESUPUESTO ANTES DE ESTE INTENTO                                    │
│ máximo 2 inicios                                                     │
├──────────────────────────────────────────────────────────────────────┤
│ LA LEY QUE LO EXIGIÓ                                                 │
└──────────────────────────────────────────────────────────────────────┘
```

The values are the exact stored authorization snapshot for this request:

- `QUIÉN PIDIÓ`: authenticated requester principal.
- `BAJO QUÉ CONTRATO`: exact intent id followed by its signed purpose.
- `CADENA`: ordered principal chain, root/operator first and executing agent
  last. It is not reconstructed from display names or current grants.
- `PRESUPUESTO ANTES DE ESTE INTENTO`: the minimum remaining start capacity
  across the intent and applicable grants when this snapshot was recorded.
  Finite capacity says `máximo N inicios`; unlimited says
  `máximo sin límite declarado`.

Every stored value is rendered through the existing `escapeUntrusted` path.
The arrows and labels are trusted UI literals; they are never embedded into a
single untrusted HTML string.

Legacy and non-strict pending requests have no authority snapshot. Their
existing document remains byte-for-byte unchanged and no empty `AUTORIDAD`
section is shown. This is a compatibility fact, not a claim that they passed
strict authority.

## Complete interaction cycle

The block has no new interaction. It opens and closes with the existing
approval detail. `Volver a leer la petición` fetches the stored document again
and resets the existing decision arming state. Approve and reject keep their
current cycles.

The budget line is evidence, not a live meter. Re-reading the pending request
returns the same parked snapshot. Approval still causes a transaction-local
recheck; another committed start or a revocation can therefore make approval
fail even when the displayed snapshot had capacity.

## On-screen error states

- A present authority object missing requester, intent id, intent purpose,
  chain, budget kind, or finite value is an unreadable response. The existing
  unreadable state appears with `Volver a intentar`; no partial block renders.
- A chain with an empty element or the wrong wire type is unreadable.
- A finite budget below zero or above JavaScript's safe integer range is
  unreadable.
- Invisible or bidirectional characters in any value are printed as
  `<U+XXXX>` by `escapeUntrusted`; they do not produce a separate error.
- A start-time revocation, exhaustion, or corrupt authority returns a named
  server refusal through the existing terminal decision surface. This block
  does not pre-announce that approval will succeed.

## Empty and loading states

The list, loading document, stopped core, missing surface, empty queue, and
legacy detail states are unchanged. The authority block is not painted until
the complete detail response has passed wire validation.

## What this does not do

- It does not edit, issue, delegate, revoke, or replenish authority.
- It does not show current live budget after other actions run.
- It does not promise that the pending request can still start.
- It does not expose signatures, raw canonical bytes, signing keys, or debit
  history.
- It does not infer human identity beyond the authenticated principal recorded
  by phase 1.

## Acceptance pass

- **Mockup:** approved by the director's 2026-09-21 commission, which required
  this block under `ORIGEN` and fixed its four facts and `máximo` wording.
- **Packaged build:** not yet run. It is required before any tag that contains
  this phase and will be recorded in the canto; this implementation task does
  not authorize a tag.

`[NEEDS CLARIFICATION]`: none.
