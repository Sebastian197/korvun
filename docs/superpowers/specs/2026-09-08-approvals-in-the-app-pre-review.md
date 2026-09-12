# v0.15.0 — Approvals in the app: pre-test adversarial review (step 9), v1

Train: **the four Control API endpoints and the screen that uses them.**
Base: `7c21902` plus the marker of PR #31. **Release requirement, not a
follow-up**: by the director's ruling of 2026-09-08 there is no v0.15.0
tag until the yes and the no can be given inside the window.

**Done means the ceremony**: the director sees the pending list, sees the
digest, approves and rejects, all inside the app.

## 0. State of the tree

| | |
|---|---|
| master | `7c21902` (#29 hygiene, #30 R15) |
| gate | was refusing master's tip; cured by PR #31, one commit, one file |
| this train's tree | nothing yet — this paper precedes RED |

## 1. Guarantees, literally

| # | Guarantee |
|---|---|
| G1 | The digest the app shows for a request is **byte-identical** to the one `korvun approvals show` prints for it. |
| G2 | **Approve-what-you-saw**: the approve call carries the digest the operator was shown; the server REFUSES if the stored request no longer re-derives it. |
| G3 | **One execution path**: the endpoint calls `app.ResolveApprovalLaw` → `app.BuildApprovalExecutorFromCage` → `app.ExecuteApprovedAction`, the same three the CLI calls. No second executor, no second claiming rule. |
| G4 | The no executes nothing and leaves its sealed receipt. |
| G5 | The endpoints mount only where the mutation surface mounts: a resolved admin token, `NO AUTH <=> LOOPBACK ONLY` intact. |
| G6 | Expiry is judged at the decision touch, from the API exactly as from the CLI. |
| G7 | With `approvals.enabled` false, the endpoints answer an **explicit disabled state** — never an empty list indistinguishable from "nothing pending". |

Not claimed: that the app becomes the only way to decide; that anything works while the core is stopped; that governance editing changes.

## 2. The production path, traced

| Layer | Fact | Source |
|---|---|---|
| transport | the desktop proxies `/api/*` to the core's ephemeral admin address | `internal/shell/proxy.go:25`, `isAdminPath` `:91` |
| credential | the shell generates a **per-cycle bearer** and injects it server-side; it never enters the DOM | `controller.go:181-195`, `proxy.go:70` |
| mount | the write surface mounts only inside `token != ""` | `internal/app/app.go:536-557` |
| store | the server already holds the action store, open, under the profile lock | `app.go:165`, `:330` |
| execution | already exported and shared with the CLI | `internal/app/approvals.go:136,191`; `internal/cli/approvals.go:278-296` |
| core stopped | the proxy answers `503 {"error":"core stopped"}` | `proxy.go:32-34` |

## 3. The three seams this train adds

| # | Seam | Director's ruling |
|---|---|---|
| S1 | `App` does not retain `*config.Config`, which `ResolveApprovalLaw` needs | retain it (or the resolved cage+law per brain) on `App` |
| S2 | `App` holds the action store as `io.Closer` with a type assertion at one site (`approvals.go:123`) | a narrow accessor; **the server's store, never a second writer in-process** |
| S3 | `controlapi` imports only `encoding/json` and `net/http`, by design | respected: **own DTOs at the edge**, and the digest contract is fixed there |

## 4. Attack matrix

| # | Attack | Expected | Evidence level |
|---|---|---|---|
| A1 | decided from the CLI a second before the app's approve | named loss, no second execution | multiple real connections |
| A2 | double POST (two clicks) | one execution, second a named loss | in-process HTTP + real store |
| A3 | the stored request mutated between show and approve | **refused by digest** (G2) | in-process HTTP + raw connection |
| A4 | approve an expired request | refused by name | in-process HTTP |
| A5 | any endpoint with no bearer | not mounted without a token; 401 with a wrong one | in-process HTTP |
| A6 | a local browser page POSTs to the loopback endpoint | refused: bearer required, not a cookie, and the proxy OVERWRITES any client Authorization | in-process HTTP + `proxy.go:70` |
| A7 | another local process with the token | it decides — the token IS the boundary | declared, to SECURITY.md |
| A8 | a huge pending list | bounded page, bounded body | in-process HTTP |
| A9 | reject comment with control bytes or 10 MB | bounded and sanitised at the edge | fuzz over the decode |
| A10 | core stopped | explicit state from the 503, never an empty list | desktop test |
| A11 | store busy (another connection holds it) | named as transient, never as "not found" | multiple real connections |
| A12 | `approvals.enabled` false | explicit disabled state (G7) | in-process HTTP + desktop test |

## 5. Failure taxonomy

`approved`, `rejected`, `already_decided`, `expired`, `digest_mismatch`,
`not_found`, `unavailable` (transient store), `disabled` (G7),
`forbidden` (no bearer).

`unavailable` is the one R15 could not force through the CLI's warm
connection and therefore withdrew. The HTTP path reopens the question:
it is forced here (A11) or it is filed here with its technical reason.
It is not left to be discovered.

## 6. Transaction and persistence boundaries

The decide-and-claim is already atomic in the store; this train adds no
transaction. What it adds is a second READER of the pending list, so what
the screen shows is a snapshot with no lock held — A1 and A3 exist
because of that window, and **G2 is the control for its dangerous half**.

No config writes. No schema. No migration.

## 7. Evidence plan

| Claim | Proof |
|---|---|
| each named outcome of §5 | one in-process HTTP mold each, against a real SQLite store, asserting the exact NAME **and the exact TEXT** (the R15 lesson: two note texts were rewritten mid-train and the suite stayed green because nothing watched the wording) |
| G1 | a test that runs the CLI's `show` and the endpoint over the SAME request and compares the digest byte for byte |
| G2 | the raw-connection mutation between show and approve |
| A1, A11 | multiple real connections, forced and observed — never an allowed-outcome test |
| the screen | the desktop harness, core stopped and running |
| every mold | its probing mutation, executed, with the red recorded |

## 8. Unresolved risks

1. The bearer is the boundary, not loopback. Today's equivalent is "any local process that can read the store file". Not obviously worse, not obviously the same — declared, and it goes to SECURITY.md.
2. The digest asymmetry: the API demands it (G2), the CLI approves by id. Unifying is filed; this train does not touch the CLI.
3. `unavailable`'s reachability through HTTP, until A11 forces it.

## 9. Blocked on the sixth law

The screen is user-visible: **no RED without a UX design approved by the
director**. The four endpoints are not user-visible and start now, in
parallel.

The design request carries three decisions, and only three:

1. **Where the digest sits**, and how reading it is made unavoidable before the buttons are reachable.
2. **What the confirmation of an irreversible yes looks like** — one click that fires an irreversible external effect is a product decision.
3. **How approve and reject are told apart visually**, so nobody hits the wrong button. Colour alone is not an answer.

## 10. Success criteria

- The four endpoints, each named outcome pinned with its probing mutation.
- G1 proven by byte comparison against the CLI's own output.
- G2 proven by the mutation-between-show-and-approve reproduction.
- A1 and A2 proven with real concurrency.
- The screen behind an approved design; the director's manual pass over the packaged build.
- **The public-truth sweep INVERTED in this same train**: every surface that says today "the decision lives in the CLI, the app does not expose approvals" is updated to the new reality, EN and ES.
- `make quality` green; desktop lanes green; adversary over the complete diff until VETO LEVANTADO; PR against master.
- **The ceremony script is rewritten from scratch against the screen** — never patched.
