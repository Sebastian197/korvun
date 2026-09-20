# Pre-test adversarial review — the approvals screen brought to its approved mockup

> **Fourth pass.** Base `b317b46` (master after #37; its tree differs from
> `c3a6d99` only in the verdict marker). Adversary passes so far, all VETO
> MANTENIDO:
> - pass 1: 2 P1, 8 P2, 3 P3;
> - pass 2: 1 P1, 7 P2, 4 P3;
> - pass 3: 2 P1, 4 P2, 4 P3;
> - pass 4: 0 P1, 3 P2, 3 P3 — the LAST paper pass, by the director's order of
>   2026-09-13. The train goes to red with the closed delta in §7.
>
> The director adjudicated five questions on 2026-09-13 (§1). Design authority:
> `design-drafts/approvals-screen-ux.html` (main repository), plates 01, 02, 03,
> 03b and 05, approved with `2026-09-08-approvals-screen-ux.md`.

## 0. What each pass saw

**Pass 1 found in the SHIPPED code:**
- The arming survives a presentational expiry, so a clock going back re-arms
  Aprobar.
- AS-63 runs dark twice, because `emulateMedia` does not move `data-theme`.
- Two scroll containers.
- The explicit `button` reset in `src/styles/index.css`.
- The focus ring grows Aprobar to 8448 px² against a cap of 8320.
- The stack ratio is 0.6250.
- Auto-repeat reaches the field.
- A corrupt `expires_at` becomes NaN.

**Pass 2 found:**
- The outline counted without its style.
- axe blind to borders.
- `Date.parse` lax and engine-dependent.
- In the shipped code, Esc with `isComposing=true` in the reason field rejects.
- A single scroller would hide the pinned bar.
- In the shipped code, Esc rejects from E9.
- AS-71 tautological.
- A 60 px stack gap once the arming leaves the doors row.

**Pass 3 found, and this pass folds:**
- **G4's per-character wording is false with the prefix inside it.** The
  ten-character prefix shares a character with the tail in both plates and in
  97.43 % of random digests. G4 becomes POSITIONAL and exact.
- **M10 stays green with its own mutation.** An explicit `.main` reset and
  Playwright's scroll-into-view before `click()` each land the scroll at 0.
  G10 drops the explicit reset; M10 activates [Volver a leer] without scrolling.
- **`Input.imeCommitComposition` does not exist in Chromium's CDP.**
- **G5e promised an `isComposing` guard on the arming keydown that CDP cannot
  reach.** The keydown after a composition arrives with `isComposing=false`. The
  claim is withdrawn: the code may keep the guard, but no guarantee is made
  (§6).
- **A transformed `::after`** paints and receives clicks outside a 168×40
  button whose computed pseudo box is 168×40. The doors now forbid painting
  pseudo-elements and transforms outright.
- **The bar sticks 24 px below `.main`'s top**, because of `padding-top`, and in
  the three measured geometries the arming row is never fully visible while under
  the bar. The rootMargin of pass 3 guarded an unreachable branch and is dropped
  from the design and from the claims (§6).
- **Edges of G7:**
  - 29 February of common and leap years;
  - plate 05's text skipping the date rule;
  - floor painting «0m 00s» during the last second, while Aprobar is still
    offered.
- **Two classification errors in pass 3.** `load()` has several callers (the
  open, [Volver a leer], [Reintentar] in illegible and refusal states, E6's
  re-read). And E9-bis is a POST refusal over a served document, not a state
  with no document.

**Seen by this pass while folding:**
- With `.approvals-doc` no longer scrolling, the `scroller` ref and its reset in
  `load()` have no scroll to reset. The cure removes both. G10 already stands on
  the loading state, and M10 attacks it.
- Ceiling the seconds (the cure for the last-second «0m 00s») moves the
  60-minute boundary: 3599.5 s remaining is 3600 s ceiled, which reads «1h 00m».
  The boundary is defined on the ceiled value.

Capture before any cure (Chromium, approvals harness, 1100×760; the temporary
probe `e2e/zz-probe-arming.spec.ts` was deleted once its output was captured and
is not part of the train):

```
PROBE inputStyle={"borderTop":"0px solid …","background":"rgba(0, 0, 0, 0)","padding":"0px","placeholder":"","rect":{"w":336,"h":24}}
PROBE bar="← Pendientes35cda6f9…324d60 · fijado mientras decides2026-09-13T16:00:08.93429Z"
PROBE listRow="tool/webhook_callIRREVERSIBLEtelegramapr_…35cda6f9…324d602026-09-13T16:00:08.93429Z"
```

The doors, the reason field and the missing cards are seen in
`/Users/sebastianmorenosaavedra/Desktop/korvun.nosync/design-drafts/ceremonia-v0150/decision-block-1100x760.png`
(gitignored, main repository only). Their computed styles were not printed by
that probe; M1 prints them. That image is a visual capture of the screen, not
evidence of any receipt, and it is not in the tree (P2-11 of the fifteenth
external pass, scoped 2026-09-19).

## 1. The director's decisions, literal (2026-09-13)

1. The arming dies with the presentational expiry: `typed` is cleared, not only
   the field unmounted; the mould forces the clock BACK and asserts approve
   absent AND `typed` empty.
2. AS-63 and the visibility mould switch theme by `data-theme`, with a
   light-only token mutation that goes red.
3. Arming as a full-width row ABOVE the reason field, as plates 03/03b.
   FR-UI-57 becomes: document → arming → reason → Rechazar → Aprobar. The doors
   keep their geometry.
4. The asymmetry is made true in the stack: Aprobar 50 % of the width, Rechazar
   100 %, mould at 900×700. The release sentence is not narrowed.
5. Detail and bar: «caduca en Xm Ys», «caduca en 1h 29m» past minute 60, the date
   when not the same UTC day. List row: «caduca · HH:MM:SSZ», no countdown.
   Corrupt `expires_at`: a named state, not NaN, and Aprobar withdrawn.

Plus the order that P2-4, P2-5 (the IME attack EXECUTED), and P2-7..10 of pass 1
are folded in full, and that the spec either gets one scroller or says two.

## 2. The literal guarantees

**G1 — every control is visible.**
- **Controls:**
  - Rechazar and Aprobar (Aprobar enabled, disabled, keyboard-focused and
    hovered);
  - the six arming cells, the reason field and [Volver a leer];
  - each document card;
  - the pinned bar.
- **Oracle:** each paints an edge with contrast **≥ 1.15:1** against its
  effective backdrop. The edge is a border of ≥ 1 px with `border-style` not
  `none`, or a background. The effective backdrop is the nearest opaque ancestor
  background, composited through every translucent layer between.
- **The bar** additionally has an opaque background (alpha 1), so the document
  never reads through it.
- **Sizes:** the doors per §7 of the spec; each cell 32×42; the reason field
  ≥ 34 px tall.
- **Clipping:** after `scrollIntoView({ block: 'center' })` on the control, its
  painted edge lies inside `.main`'s client rect, below the pinned bar's bottom
  edge, and no ancestor clips it.
- **How it is read:** computed style after `element.getAnimations()` is empty,
  in both themes selected by `data-theme` (and the stored choice) before load.

**G2 — the asymmetry is visible at every size.**
- **Structural rule for the doors:** on Rechazar and Aprobar, in every state,
  `::before` and `::after` have `content: none`; `transform`, `scale`,
  `translate`, `rotate`, `filter` and `clip-path` are `none`; `zoom` is `1`.
  The border box is taken from `getBoundingClientRect`, never from
  `offsetWidth`.
- **Painted extent** is the union of:
  - the border box;
  - the outline ring, counted only when `outline-style` is not `none`, grown by
    `outline-width + outline-offset` when that sum is positive;
  - every `box-shadow` that is not `inset`, grown by
    `max(|offset-x|, |offset-y|) + blur + spread`.
- **Oracle:** extent(Aprobar) ≤ 0.50 × extent(Rechazar).
- **Conditions:** at 1100×760 and 900×700, in four states — none, Aprobar
  keyboard-focused, Rechazar keyboard-focused, Aprobar hovered.

**G3 — the gate is six cells in a full-width row above the reason field.**
- **Order:** in the DOM and under Tab, document → arming → reason → Rechazar →
  Aprobar.
- **Cells:** each shows `typed[i]` when present and «–» otherwise.
- **Status:** beside the cells, «faltan N» / «✓ coincide» / «no coincide»
  (FR-UI-42).
- **Prefix:** the prefix text EQUALS the ten digest characters immediately before
  the tail (hex positions 49–58 of the 64, 1-based).
- **Focus indicator:** the focused field shows it ON THE CELLS. The cell row's
  edge contrast changes by ≥ 1.15:1 between focused and unfocused.

**G4 — no real character before it is typed (positional).** With `typed` of
length i:
- every cell j ≥ i renders exactly «–»;
- every cell j < i renders exactly `typed[j]`;
- the cells' `::before`/`::after` have `content: none`;
- the `<input>`'s `value` equals `typed`;
- its `placeholder` is absent or empty;
- every element in the arming row carries only the attributes `id`, `class`,
  `type`, `maxlength`, `autocomplete`, `inputmode`, `spellcheck`, `value`, `role`,
  `for`, `data-testid` and `aria-*`, and every one of them except `value` is
  byte-identical across two parked requests with different digests;
- the prefix is bound by G3's equality. An off-by-one slice that reaches into
  the tail breaks the equality.

The label and the status line are pinned by exact text: the label equals «Para
armar Aprobar, reteclea los seis últimos caracteres del digest», and the status
equals «faltan {6 − i}», «✓ coincide» or «no coincide», and nothing else.

**G5 — the autofocus never shortens the yes.**
- (a) At most once per document load. A load is the open, [Volver a leer],
  [Reintentar] or E6's re-read.
- (b) Trigger: IntersectionObserver with root `.main` and threshold 1.0 on the
  arming row. A document that fits the viewport fires at load, declared.
- (c) It moves focus only when `document.activeElement` is `body` or `.main`.
- (d) ~~`focus({ preventScroll: true })`: `.main.scrollTop` is equal before and
  after.~~ WITHDRAWN in §10: unobservable while the trigger requires the row
  fully in view.
- (e) The arming field counts a character only from a keydown with
  `repeat === false` and `keyCode !== 229`. `input`/`beforeinput` events and
  `insertText` never count.

**G6 — Esc has exactly one meaning.**
- **Active** only while the live document is painted with its decision block and
  no decision was sent. That includes the clock-expired and «caducidad ilegible»
  variants, which keep Rechazar and the Esc line of plate 05.
- **Inert** in every state FR-UI-53 lists, E9 included (a 200 detail painted as
  `Unreadable`).
- **Inert** for an Esc keydown with `isComposing === true` or with
  `keyCode === 229`.

**G7 — expiry, labelled and never a lie.**
- **Accepted shape:** `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{1,9})?Z$`, then
  calendar validity:
  - month 01–12;
  - day valid for the month, with 29 February only in Gregorian leap years;
  - hour 00–23, minute and second 00–59.

  Anything else non-empty is **illegible**. Nothing reaches `Date.parse`
  without passing the shape.
- **Remaining:** `s = ceil((expiry − now) / 1000)`.
- **Detail bar and CADUCIDAD card,** while `s > 0`:
  - `s < 3600` → «caduca en {floor(s/60)}m {s mod 60, two digits}s»;
  - `s ≥ 3600` → «caduca en {floor(s/3600)}h {floor((s mod 3600)/60), two digits}m»;
  - then « · » and the stored `HH:MM:SS` plus «Z», sliced from the STORED string;
  - with `YYYY-MM-DD ` before it when the stored UTC date differs from the
    window clock's UTC date;
  - label and time in ONE element.
- **At `s ≤ 0`:** the bar says «el reloj de la ventana dice 00:00 · » followed by
  the same time and date rule (plate 05). A one-shot timer armed for the expiry
  instant re-renders then, so in the foreground Aprobar is withdrawn at the
  expiry instant within timer resolution, not on the next 1 s tick. The card
  keeps the full stored string.
- **List row:** «caduca · » plus the same time and date rule, no countdown.
- **Empty `expires_at`:** «no caduca» (FR-UI-13).
- **Illegible:** «caducidad ilegible» plus the raw value through
  `escapeUntrusted`, in the bar, the card and the row. Aprobar withdrawn,
  `typed` cleared, Rechazar kept.
- **Outside the live document** — read refusals (E7, E9) and the post-POST
  states: the bar carries only [← Pendientes] and no countdown. E5 is the one
  exception by FR-UI-30: its literal prints the stored instant. An illegible
  stored value prints «caducidad ilegible» through `escapeUntrusted`, never raw.

**G8 — the arming dies with every presentational withdrawal.** From the first
render in which `canApprove` is false (`s ≤ 0`, illegible expiry), `typed` is
`''`. A later true (the clock going back) shows an empty gate and a disabled
Aprobar. Observed at level J after the render settles.

**G9 — one scroll container, and the bar stays pinned.** `.main` is the only
scrolling ancestor of the document, as the spec says. The bar is
`position: sticky` inside `.main` and lies fully inside `.main`'s client rect at
every `scrollTop` from 0 to the maximum.

**G10 — a re-read shows the new document from the top.** After [Volver a leer],
the document is painted with `.main.scrollTop === 0`.
- **The only mechanism:** the loading state replaces the document, the content
  shrinks, and the scroll clamps.
- **No explicit reset exists.** The `scroller` ref and its reset are removed with
  `.approvals-doc`'s scroll.

**G11 — the rest of the safety contract does not move.** Paste, drop and
autofill do not arm. Esc is inert in the list. No modal. No identity gradient on
either door. Red marks the effect, not the rejection. The stack keeps
**≥ 120 px** between Rechazar and Aprobar.

## 3. Attack matrix

| # | Attack | G | Required outcome |
|---|---|---|---|
| A1 | Strip one door's border/background | G1 | M1 red, control and computed pair named |
| A2 | Paint only an inner label span; the button box stays | G1, G2 | red |
| A3 | Arm, clock past expiry, clock BACK | G8 | withdrawal: approve null, `typed` `''`; after back: field `''`, «faltan 6», Aprobar disabled |
| A4 | `expires_at` = `"garbage"`, `"2030"`, `"1"`, `"0"`, `"2026-09-13 16:00:08"`, `"2026-09-13T16:00:08"`, `"2026-02-30T10:00:00Z"`, `"2026-02-29T10:00:00Z"`, `"2026-09-13T24:00:00Z"` | G7, G8 | each: «caducidad ilegible», no Aprobar, `typed` `''`, Rechazar enabled |
| A4b | `"2028-02-29T10:00:00Z"` | G7 | legal: labelled, Aprobar offered |
| A5 | `placeholder={tail}`; `data-tail`; CSS `content: attr()`; cell j ≥ i painting `tail[j]` dimmed; prefix sliced one position late | G3, G4 | M4 red per form |
| A6 | Hex key held on `body`, autofocus lands, repeats arrive | G5e | cells «–», «faltan 6» |
| A7 | CDP `Input.imeSetComposition` then `Input.insertText` of hex into the arming field | G5e | «faltan 6» |
| A7b | CDP `Input.dispatchKeyEvent` keydown, `windowsVirtualKeyCode: 229`, hex `key` | G5e | «faltan 6» (red today: `faltan 5`) |
| A8 | CDP composition open in the reason field (`compositionstart` observed), Esc; then `Input.insertText` closes it (`compositionend` observed), Esc | G6 | zero reject POSTs on the first Esc; one on the second |
| A8b | CDP keydown Escape with `windowsVirtualKeyCode: 229` in the reason field | G6 | zero reject POSTs |
| A9 | Operator typing in the reason field when the row comes into view | G5c | focus and text intact |
| A10 | Focus on Rechazar, Aprobar, [Volver a leer] or [← Pendientes] when the row comes into view | G5c | focus stays |
| A11 | Autofocus on every intersection; tab away and scroll again | G5a | one programmatic focus per load |
| A12 | [Volver a leer] keeps the old flag | G5a | the new load autofocuses once; arming empty |
| A13 | `focus()` without `preventScroll` | G5d | `scrollTop` unchanged |
| A14 | Keyboard ring on Aprobar; `box-shadow: 0 0 24px`; hover | G2 | extent ≤ 0.50 × Rechazar's |
| A14b | `::after` with `content:''` and any geometry, including `transform: scale(2.2)` | G2 | structural rule red |
| A15 | Stack at 900×700, both doors 100 % | G2 | Aprobar 50 %, ratio ≤ 0.50 |
| A16 | Light-only BORDER token (`--line` = `--card`) | G1 | M1 light red, dark green |
| A17 | Light-only TEXT token (`--tx2` = `--card`) | FR-UI-58 | AS-63 light red (`color-contrast`), dark green |
| A18 | Countdown from `requested_at`/stale `now`; label in a sibling; HH:MM:SS recomputed in local time | G7 | one element; the stored slice |
| A19 | Expiry on another UTC day, also at zero; 90 min; 65 min; 3599.5 s; 0.5 s | G7 | date printed in both texts; «1h 30m»; «1h 05m»; «1h 00m»; «0m 01s» with Aprobar offered |
| A20 | Clock at or past zero | G7 | plate 05 text; Aprobar withdrawn; Rechazar kept; Esc active |
| A21 | Countdown added to the list row | G7 | no «caduca en» in the row |
| A22 | `.approvals-doc` keeps its scroll; bar not sticky; bar translucent | G1, G9 | M9 red; M1 bar alpha red |
| A23 | Mid-transition read | G1 | read after animations settle |
| A24 | Arming kept between the doors | G3 | DOM and Tab ladder red |
| A25 | Esc on E9 (`present` with empty body; `parameters_state: "pepino"`) | G6 | zero reject POSTs |
| A26 | Re-read keeps the previous document painted; activation without scroll (`element.click()` inside `page.evaluate`) from `scrollTop` > 0 | G10 | M10 red: `scrollTop` > 0 |
| A27 | Stack gap left at 60 px | G11 | red: gap < 120 |

## 4. Moulds, probing mutations and evidence level

Levels:
- **B:** real Chromium via Playwright against the approvals harness (real core,
  real store, parked by the real factory).
- **B-CDP:** the same, through a Playwright CDP session.
- **J:** jsdom, in-process.

Born red against `b317b46` where the defect exists today: A3, A6, A7b, the first
Esc of A8, A8b, A14 (ring), A15, A25, A27, and M1's visibility. Each mutation runs
after green, with its red captured.

| Mould | Level | Attacks | Probing mutation that must turn it red |
|---|---|---|---|
| M1 visibility, per control, 1100×760, dark and light | B | A1, A2, A16, A22 (alpha), A23 | remove Rechazar's border and background; `--line` = `--card` light-only; paint Aprobar's label, not its box; bar background translucent |
| M2 asymmetry, 4 states × 2 sizes, structural rule, stack gap | B | A14, A14b, A15, A27 | drop the ring containment on Aprobar; `box-shadow: 0 0 24px` on Aprobar; `::after{content:'';inset:0;transform:scale(2.2)}` on Aprobar; `scale: 2.2` on Aprobar; `width: 100%` on Aprobar in the stack; stack gap 60 px |
| M3 six cells, order, prefix, focus indicator | B | A24, A5 (prefix) | five cells; cell not reflecting `typed`; arming back between the doors; focus indicator only on the hidden input; prefix sliced one late; status painting «faltan 6 · {tail}» |
| M4 positional oracle | B | A5 | `placeholder={tail}`; `data-tail={tail}`; `content: attr(data-tail)`; cell j ≥ i paints `tail[j]` dimmed; `data-digest-tail` added to a cell |
| M5 autofocus and repeat | B | A6, A9–A13 | ignore `activeElement`; no once-flag; flag not reset on reload; `focus()` without `preventScroll`; stop ignoring `e.repeat` |
| M6 IME, 229 | B-CDP | A7, A7b, A8, A8b | drop the `keyCode === 229` guard in `onKeyDown`; drop `isComposing` in the Esc listener; drop the 229 check in the Esc listener; count `input` events |
| M7 expiry text and shape | J (+B for the bar at 1100) | A4, A4b, A18–A21 | shape check replaced by `Date.parse`; leap rule removed; floor instead of ceil; countdown from `requested_at`; label in a sibling span; minute padding removed; date branch removed from the zero text; one-shot expiry timer removed (fake timers offset 400 ms from the tick) |
| M8 arming dies | J | A3 | do not clear `typed` when `canApprove` falls |
| M9 one scroller, pinned bar | B | A22 | `overflow-y: auto` on `.approvals-doc`; remove `position: sticky` |
| M10 re-read from the top | B | A26 | render the previous document during the re-read instead of «Consultando el almacén…» |
| M11 Esc inert on E9 | J | A25 | `escActive` back to `detail !== null && decision === null` |
| AS-63 retargeted | B | A17 | `--tx2` = `--card` light-only |

**Preconditions checked inside M6, not assumed.** A8 asserts that
`compositionstart` fired before the first Esc and `compositionend` before the
second. If CDP cannot produce either, the red phase STOPS and reports it. It is
never replaced by a synthetic `KeyboardEvent` (level J).

**Approved tests this pass touches, by the director's decisions:**
- AS-21 gains `typed` and the clock going back.
- AS-51 takes the new ladder.
- AS-55 takes the 50 % width and the 120 px gap.
- AS-63 switches theme by `data-theme` and gains its text-token mutation.
- AS-71 targets `.main`, activates without scrolling, and gains M10's mutation.
- The «between the two doors» comment and the `ArmingField` godoc are rewritten
  to the new wire.
- FR-UI-57 takes the new ladder.
- The spec names the sticky bar and keeps its single-scroller sentence true.

No other approved assertion changes. If one has to, the train stops and asks.

## 5. Expected failure taxonomy

Browser moulds name the control and print the computed pair, for example:
- `Rechazar[light]: edge 1.02:1 < 1.15 (border 0px none, background rgba(0,0,0,0) over #fafafc)`
- `Aprobar[focus]: extent 8448 > 0.50 × 16640`
- `Aprobar: ::after content '' (doors paint no pseudo-elements)`
- `autofocus: 2 programmatic focus events in one load`
- `Esc[E9]: 1 reject POST`

jsdom moulds keep their literal texts. Never «not visible» or «some error».

## 6. Unresolved risks, declared

- **WCAG 1.4.11.** The approved border tokens reach 1.16–1.29:1, not 3:1. G1's
  1.15:1 proves «painted and distinguishable by computed style», not compliance
  with non-text contrast. Raising the tokens is a design change for the director.
- **The window is not Chromium.** macOS runs WKWebView, Windows WebView2, Linux
  WebKitGTK. Only Chromium is installed for Playwright here; adding WebKit is a
  download that needs the director's yes.
  - `screencapture` and the director's manual pass judge PIXELS on macOS, not
    G5/G6 behaviour.
  - WebKit may fire `compositionend` before the keydown that closes a
    composition. If so, an Esc that cancels an IME in WKWebView arrives with
    `isComposing=false`; G6's 229 check is the second signal, and neither is
    verified there.
- **The `isComposing` guard on the arming keydown** may stay in the code and is
  NOT a guarantee. CDP cannot produce a keydown with `isComposing=true` into a
  controlled input: the composition dies on the reset. No mould watches it, so
  nothing claims it.
- **The bar may cover the arming row** at geometries not measured. Pass 3 found
  no overlap at 1100×760, 900×700 and 1100×560. The autofocus trigger does not
  discount the bar, and no claim covers other sizes.
- **The window clock** drives the countdown and the withdrawal. FR-UI-29 keeps
  the server as the only judge. A backgrounded WebView throttles timers, so the
  one-shot withdrawal may fire late there; not measured.
- **Reduced motion.** The moulds read after animations settle and do not assert
  behaviour under `prefers-reduced-motion`. No transition is added.
- **IntersectionObserver** with fractional geometry or zoom is not exercised.

## 7. Delta of pass 4 — closed, and the last paper pass

By the director's order of 2026-09-13: amend, do not rewrite; instrument defects
are corrected and declared but do not sustain a veto; at most two paper passes
per train (this train had already spent four). The train goes to red with this
delta. The paragraphs amended are exactly the ones below, each marked with the
finding it answers.

| Finding (pass 4) | Class | Amendment |
|---|---|---|
| P2-1 — the withdrawal rides the 1 s tick, so «exactly this instant» was false | PRODUCT (guarantee not met by the mechanism) | G7 «At `s ≤ 0`»: a one-shot timer at the expiry instant; M7 mutation removes it under fake timers offset from the tick; background throttling declared in §6 |
| P2-2 — `scale`, `translate`, `rotate` escape the structural rule | INSTRUMENT | G2 structural rule widened, border box from `getBoundingClientRect`; M2 mutation `scale: 2.2` |
| P2-3 — status line and label unwatched, a channel for the tail | PRODUCT (guarantee gap) | G4 exclusion replaced by exact-text pins; M3 mutation paints the tail in the status |
| P3-4 — A8b red today but not listed | INSTRUMENT | §4 born-red list |
| P3-5 — attribute substring guard is textual and flaky | INSTRUMENT | G4 attribute rule becomes an allow-list plus byte-identity across two digests; M4 mutation `data-digest-tail` |
| P3-6 — scroll method unfixed; the sticky bar can cover a control | INSTRUMENT | G1 clipping: `scrollIntoView({ block: 'center' })` and below the bar's bottom edge |

The veto of pass 4 rested on P2-1 and P2-3 (PRODUCT). Both are folded into
guarantees with a mould and a probing mutation. They are not adjudicated away.
Whether the folds hold is now for the moulds to say, not for another paper pass.

## 8. Amendments from the author's five-question check, before the red is handed over

By the director's order of 2026-09-13, the author answers five questions before
any handoff. Two answers were «yes» and are cured here by delta.

- **Q1 (a sentence wider than the code).** G7 said that no post-POST state shows
  an expiry. The code prints one in E5's literal («Caducó a las …», in
  `DecisionState`). The G7 bullet «Outside the live document» is amended.
- **Q4 (a sister door).** Five sites paint `expires_at`: the list row, the window
  clock, the bar, the CADUCIDAD card and E5's literal. E5 was not in the attack
  matrix, and it prints the stored value without `escapeUntrusted`. Added: a
  jsdom mould «G7 · E5 tras un POST con caducidad ilegible», born red.

## 9. Delta of pass 5 — closed

Pass 5 attacked only §7 and §8: VETO MANTENIDO on two [PRODUCT] findings.

| Finding (pass 5) | Class | Amendment |
|---|---|---|
| P2-1 — E5 reached by the READ door (`ReadRefusal`, 409 expired on the GET) printed the server's English sentence in the slot of the instant; §8 counted five sites that paint expiry and there are six | PRODUCT (a shipped literal that tells the operator something false) | G7 «Outside the live document»: E5 by the POST door prints the stored instant; E5 by the READ door has no stored instant and prints none — the title and the closing sentence only, never `answer.message` in that slot. New jsdom mould «G7 · E5 por la puerta de lectura», born red |
| P2-2 — the one-shot timer cannot be armed for 2^31 ms or more (≈ 24.855 days) and `approvals.ttl` has no ceiling | PRODUCT (the withdrawal guarantee fails for long TTLs, or loops) | G7 «At `s ≤ 0`»: the timer's delay is clamped to 2 147 483 647 ms and re-armed when it fires early; the 1 s tick remains the net for a wall-clock jump after arming, declared. New browser mould «G7 · no setTimeout is armed with a delay above 2147483647 ms», green today and guarding the cure; its mutation removes the clamp |
| P2-3 — G4's identity clause compared one digest with itself (two default parks) | INSTRUMENT | the second park carries other params; a precondition asserts the digests differ; M4 gains the mutation `aria-roledescription={tail}` (not `aria-label`, which would rename the field and fail the locator before the oracle) |
| P3-4 — G1's vertical clipping clause cannot hold for a card taller than `.main` (FR-UI-47 forbids its own scroll) | INSTRUMENT | G1: for an element taller than the band below the bar, only horizontal containment is judged, declared |
| P3-5 — `border-image-outset` and `-webkit-box-reflect` paint outside the border box | INSTRUMENT | G2 structural rule: `border-image-outset` is `0` and `-webkit-box-reflect` is `none` on the doors |

The six sites that paint an expiry, now all with a mould: list row, window clock,
bar, CADUCIDAD card, E5 by the POST door, E5 by the READ door.

## 10. Delta of the green — what the probing mutations found

Pass 6 lifted the veto (no [PRODUCT] finding). Its one [INSTRUMENT] finding is
folded: the READ-door E5 mould also asserts the absence of «DIGEST — YA NO
ACCIONABLE». The green ran 45 probing mutations. 43 turned their moulds red; the
other two are recorded here, and they change what the paper claims.

| Mutation | Result | Amendment |
|---|---|---|
| B9b — the once-per-load flag removed | NOT-RED: G5a scrolled away and back inside one frame, so the observer never saw the row leave and never re-fired | INSTRUMENT, cured: G5a waits until the row is out of view before scrolling back, and B9b is re-run |
| B9d — `focus()` without `preventScroll` | NOT-RED by construction: the trigger requires the row fully in view, so a focus without `preventScroll` scrolls nothing | G5d is WITHDRAWN as a guarantee. The code keeps `preventScroll: true`; nothing claims it |

Two sentences found false by the author's five questions on the green, corrected:
- the release note's «with 384 px between them»: AS-53 now measures
  `gap=354.0px`, because the cards add 28 px of padding and 2 px of border to the
  row; the note and FR-UI-48's capture say 354;
- two comments wider than their code: «never counts» in `ArmingField`, and
  «Every rule here» in `approvals.css`.

## 11. Delta of the adversary's review of the green diff

VETO MANTENIDO on two [PRODUCT] findings. Both are sisters the author's fourth
question missed.

| Finding | Class | Amendment |
|---|---|---|
| P2-1 — an illegible digest still offered Aprobar (disabled), the arming row and the autofocus, against FR-UI-15 «sin Aprobar»; present in `b317b46` too | PRODUCT | `canApprove` requires `isDigest`. New jsdom mould «FR-UI-15 · digest ilegible en el detalle» (empty and malformed digests); mutation: drop `isDigest` from `canApprove` |
| P2-2 — a long canonical literal (one unbroken token) overflowed its card; with `.main` as the only scroller, a sideways scroll took the sticky bar off screen — a regression of this train | PRODUCT | untrusted text wraps (`overflow-wrap: anywhere`) in document cards, the bar, state lines and list rows. New guarantee G9b: `.main` has no horizontal overflow with a 400-character body, a 300-character purpose and a 200-character channel, in the list and in the document, and the bar stays inside `.main` after a sideways scroll attempt. New browser mould «G9b»; mutation: drop the wrapping rule |
| P2-3 — G5b's «fully in view» trigger had no mould that goes red | INSTRUMENT | new browser mould «G5b · a row only partly in view does not take the focus», with a precondition that the row is half in view; mutation: threshold 0 and `intersectionRatio > 0` |
| P3-4 — G1 and G9 could not see the overflow | INSTRUMENT | covered by G9b |
| P3-5 — AS-44's title promises «sin Aprobar» and asserts only the list | INSTRUMENT | AS-44 is an approved test and is not edited without the director; the document half is now watched by the new FR-UI-15 mould. The title is filed |

## 12. Delta of the pass on the cures

VETO LEVANTADO: the two [PRODUCT] cures of §11 hold. Four P3, folded:

| Finding | Class | Amendment |
|---|---|---|
| P3-1 — a long start error in «El núcleo está parado» (a `<p>` of the state, outside the wrap rule) pushed `.main` 4024 px sideways | PRODUCT, P3 | the wrap rule covers every paragraph of a state (`.approvals-state p`); new browser mould «G9b · a long start error in the stopped-core state wraps»; mutation Y5 drops that selector |
| P3-2 — G9b watched body, purpose and channel only, while the canto said it watched the bar spans and state lines | INSTRUMENT | the canto now says exactly what is moulded: body, purpose, channel (list and document) and the state paragraphs; the bar's `span`s and `.approvals-state-line` are covered by the rule without a mould of their own, declared |
| P3-3 — G5b's 50 % precondition let any trigger above 0.5 pass | INSTRUMENT | the precondition leaves all but the last 3 px in view and asserts at least 90 %; mutation Y4 moves the trigger to 0.9 |
| P3-4 — «The title is filed» while AS-44 was filed nowhere | INSTRUMENT | filed in `docs/HANDOFF.md` |

**STOP out of the delta, for the director.** The parameters paragraph renders with
`white-space: normal`: two consecutive spaces in the canonical literal paint as
one, while the digest seals both. That contradicts FR-UI-68 and the screen's own
sentence «Cambia un solo carácter de lo que se ve abajo y este digest es otro
digest». Captured by the adversary with `{"body":"pagar  100 EUR",…}`: textContent
keeps two spaces, the rendered text shows one. Not cured in this train without
the director's adjudication.

## 13. The §12 stop, entered by the director (2026-09-13)

«§4 ENTRA EN ESTE TREN». The document's paragraphs render with `white-space:
pre-wrap`, so two consecutive spaces in an untrusted field are painted as two,
as the digest seals them (FR-UI-68). The rule covers every paragraph of the
document cards, not only the parameters: the other card fields are the sisters
of the same class.

- Mould: browser «FR-UI-68 · two consecutive spaces in the parameters and the
  purpose are painted as two» (`{"body":"pagar  100 EUR",…}`, purpose
  `avisar  al webhook`; `innerText` equals `textContent`, with a precondition that
  the stored text carries two spaces).
- Mutation Z1, the rule removed: RED, `approval-parameters: painted text equals the text`.
- It already happened in `b317b46`: executed against that tree, the mould fails
  with painted `"pagar 100 EUR"` against stored `"pagar  100 EUR"` (capture in the canto).

## 14. Pass on the §4 delta — VETO LEVANTADO

No [PRODUCT] finding sustains the veto. Three P3 are declared in the canto and not
cured in this train: the list row still collapses repeated spaces in the
operation ([PRODUCT], filed in `docs/HANDOFF.md`); the FR-UI-68 mould does not
watch the OPERACIÓN card ([INSTRUMENT]); and its oracle is `innerText`, which keeps
a long run of spaces that `pre-wrap` hangs at the line end ([INSTRUMENT]).
