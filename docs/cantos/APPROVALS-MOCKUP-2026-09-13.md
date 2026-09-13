# Canto — the approvals screen brought to its approved mockup (2026-09-13)

Train `v0150-pretag-fixes`, base `b317b46`. Director's option B of 2026-09-13,
born from the v0.15.0 release ceremony: the arming field could not be seen, and
the expiry was painted without a label.

Pre-test paper: `docs/superpowers/specs/2026-09-13-approvals-screen-to-mockup-pretest.md`
(six adversary passes; the veto was lifted in pass 6 on the delta of pass 5).

## The ceremony, executed by the director's hand

Captured from the ceremony's own store, on a copy with its md5 unchanged:

```
$ korvun ledger check --config <copy>
ledger main: 4 receipts, chain intact
$ korvun receipt verify --config <copy> rcpt_c63f0f88055f01d610c7ac1bff708a87
receipt rcpt_c63f0f88055f01d610c7ac1bff708a87 (seq 0): OK
$ korvun receipt verify --config <copy> rcpt_dc4f062c085aaaf82818231fa91884ff
receipt rcpt_dc4f062c085aaaf82818231fa91884ff (seq 2): OK
```

| seq | receipt | operation | outcome |
| --- | --- | --- | --- |
| 0 | `rcpt_c63f0f88…` | decide | SUCCEEDED |
| 1 | `rcpt_59ef1911…` | webhook_call | SUCCEEDED |
| 2 | `rcpt_dc4f062c…` | decide | SUCCEEDED |
| 3 | `rcpt_bf73a8ef…` | webhook_call | REJECTED |

The screen shows the receipt of the DECISION; the effect closes in its own
receipt. The execution receipt's `result_digest` is `sha256` of `{"ok":true}`.

The literal the model wrote differed from the chat. The director typed
«hola-ceremonia»; the receiver logged `{"message":"hola-ceremony"}`. The stored
action digest of the approved request re-derives, with the tree's own
`action.Digest`, over `http://127.0.0.1:5678/hook {"message":"hola-ceremony"}`
(`sha256:acb86c4e…ee7cc074`). The rejected request's digest re-derives over
«hola-ceremonia».

The earlier attempt refused by the cage (`127.0.0.1:5678` off the allow-list)
closed `FAILED` in `rcpt_5bf3e1e8…` after its decision `rcpt_928f406a…`. The
screen named the refusal with its receipt and did not claim uncertainty about
the decision.

## Guarantee → test that breaks it → mutation executed → level

Levels: **J** jsdom with fetch stubbed; **B** real Chromium via Playwright against
the approvals harness (real core, real store, requests parked by the real
factory); **B-CDP** the same through a CDP session. None of this is WKWebView,
WebView2 or WebKitGTK.

| Guarantee | Test | Mutation executed → red captured | Level |
| --- | --- | --- | --- |
| G8 the arming dies with the presentational expiry | AS-21 (amended) | J1 `setTyped('')` removed → `expected '9f1b60' to be ''` | J |
| G7 shape and calendar before any parse | «caducidad ilegible por forma y por calendario» | J2 `Date.parse` accepted → `Unable to find … caducidad ilegible`; J3 leap rule removed → same | J |
| G7 ceil | «redondeo hacia arriba» | J4 floor → `Unable to find … caduca en 1h 00m · 14:59:59Z` | J |
| G7 label and time in one element | «menos de 60 min»; «the bar says» | J6 / B15 sibling span → not found / `toHaveLength` | J, B |
| G7 minute padding | «desde 60 min» | J7 → `Unable to find … 1h 05m` | J |
| G7 date of another UTC day | «otro día UTC» | J8 → `Unable to find … 2026-09-09 02:00:00Z` | J |
| G7 withdrawal at the instant | «la retirada llega en el instante» | J9 timer removed → `expected <button> to be null` | J |
| G7 timer clamped to setTimeout's range | «no setTimeout is armed with a delay above 2147483647 ms» | B14 clamp removed → `delays beyond setTimeout range` | B |
| G7 E5 by the POST door | «E5 tras un POST con caducidad ilegible» | J13 illegible branch removed → `Unable to find … /caducidad ilegible/` | J |
| G7 E5 by the READ door | «E5 por la puerta de lectura» | J12 server sentence back → red | J |
| G6 Esc inert on E9 | «Esc inerte en E9» | J10 `escActive` ignores what is painted → `got 1` | J |
| G6 Esc of an IME composition / keyCode 229 | «Esc de una composición IME» (J); «an Esc that belongs to an IME composition» (B-CDP) | J11, J11b, B10b `Esc during a composition`, B10c `Esc with keyCode 229` | J, B-CDP |
| G5e repeat and 229 do not arm | «una tecla repetida» (J); G5a, G5e (B) | J14, J14b `expected '9' to be ''`; B9e; B10a `keydown with keyCode 229` | J, B, B-CDP |
| G5a one programmatic focus per load | G5a | B9b once-flag removed → red (after the mould was cured, see below); B9c flag not reset → red | B |
| G5c autofocus does not take the focus | G5c | B9a `activeElement` ignored → `toBeFocused() failed` | B |
| G1 the listed controls paint a visible edge | G1 dark, G1 light | B1 Rechazar stripped → `Rechazar[dark]: edge 1.00:1`; B2 light-only border token → red in light only | B |
| G2 asymmetry, four states, two sizes; structural rule | G2 1100×760, G2 900×700; AS-55 | B3 ring containment → `8448 > 0.50 × 16640`; B4 glow; B5 `::after`; B5b `scale 2.2`; B6 full width in the stack; B6b gap 60 | B |
| G3 six cells, ladder, prefix, focus on the cells | G3; AS-51; «textos exactos» | B7a five cells; B7b reason before the arming → `arming row above the reason field`; B7c indicator off the cells; J15 tail in the status; J16 cells not reflecting; J17 prefix one early | B, J |
| G4 no untyped character in attributes or pseudo-content | G4 | B8a placeholder; B8b `data-tail`; B8c pseudo-content; B8d `aria-roledescription={tail}` → `attributes must not depend on the digest` | B |
| G9 one scroller, sticky bar | G9 | B11a `.approvals-doc` scrolls → red; B11b not sticky → `bar inside .main at bottom` | B |
| G10 a re-read from the top | AS-71 (retargeted) | B12 previous document kept → red | B |
| FR-UI-58 AA in light | AS-63 (theme by `data-theme`) | B13 light-only text token → `light: color-contrast (6)` | B |
| FR-UI-15 an illegible digest offers no Aprobar and no arming row (found by the adversary's review of the diff) | «FR-UI-15 · digest ilegible en el detalle» | X1 `isDigest` dropped from `canApprove` → `approve «»: expected <button> to be null` | J |
| G9b long untrusted tokens wrap; `.main` does not scroll sideways (found by the same review) | G9b | X2 wrapping rule dropped → `list: .main horizontal overflow` | B |
| G5b the trigger is the row fully in view | G5b (precondition: at least 90 % but not fully in view) | X3 threshold 0 and `intersectionRatio > 0` → `no focus while partly in view`; Y4 trigger at 0.9 → same | B |
| FR-UI-68 what is painted is what is sealed: repeated spaces are not collapsed (entered by the director from the adversary's out-of-delta stop) | «FR-UI-68 · two consecutive spaces in the parameters and the purpose are painted as two» | Z1 `white-space: pre-wrap` removed → `approval-parameters: painted text equals the text` | B |
| G9b state paragraphs wrap (found by the pass on the cures) | «G9b · a long start error in the stopped-core state wraps» | Y5 `.approvals-state p` dropped from the rule → `stopped-core state: .main horizontal overflow` | B |

**51 mutations, 50 red.** The runner restored every source byte-identical after
each one (`mutations-result.txt`, `mutations-b9b.txt`, `mutations-x.txt`,
`mutations-y.txt`, `mutations-z.txt`).

- **B9b first ran NOT-RED.** G5a scrolled away and back inside one frame, so the
  observer did not see the row leave. The mould was cured (it waits until the row
  is out of view) and B9b re-ran RED.
- **B9d stayed NOT-RED by construction.** `focus()` without `preventScroll` scrolls
  nothing when the trigger already requires the row fully in view. The guarantee
  G5d is withdrawn; the code keeps `preventScroll: true` and nothing claims it.

## Final green, over the finished tree

```
vitest run            Test Files  43 passed (43)   Tests  410 passed (410)
playwright test       48 passed (1.4m)   (real builder built for the canvas specs)
                      — before the FR-UI-68 mould existed; after it, the two
                      approvals specs: 28 passed. The full suite runs in the
                      ensayo rehearsal.
prettier --check      All matched files use Prettier code style!
tsc --noEmit          exit 0          eslint .   exit 0
```

The canvas specs (`canvas-header-sp6`, `canvas-sp4`) failed in an earlier local run
(`canvas-surface` not found) while `web/builder/dist` was the committed
placeholder; in this run the real builder was built first, as CI does
(`frontend.yml`, «Build embedded builder dist»), and both passed. The builder's
and the chrome's `dist` placeholders were restored afterwards.

## FR-UI-68 already failed in the base

The FR-UI-68 mould, executed against `b317b46`'s tree (worktree at `990e4d5`, whose
tree is byte-identical to `b317b46`'s), with the base's own `Approvals.tsx` and
`approvals.css`:

```
Error: approval-parameters: painted text equals the text
Expected: "{\"body\":\"pagar  100 EUR\",\"url\":\"https://hooks.acme.io/pedidos\"}"
Received: "{\"body\":\"pagar 100 EUR\",\"url\":\"https://hooks.acme.io/pedidos\"}"
1 failed
```

So the collapse of repeated spaces shipped with #36; this train did not introduce
it. That is executed, not deduced.

## Measured in the browser

```
AS-52 measured: approve=6720.0px2 reject=16640.0px2
AS-53 measured: reject={"x":275,…,"width":260,"height":64} approve={"x":889,…,"width":168,"height":40} gap=354.0px
AS-55 measured: reject={…,"width":582,"height":64} approve={…,"width":291,"height":40} vertical gap=120.0px
```

The release note said «384 px between them»; the cards' padding and border
brought it to 354, and the note now says so.

## Declared, not verified

- WKWebView, WebView2 and WebKitGTK; a real system IME; throttled background
  timers.
- WCAG 1.4.11: the approved border tokens reach 1.16–1.29:1, not 3:1. G1's 1.15:1
  proves a painted, distinguishable edge by computed style, not non-text
  contrast compliance.

## The adversary's last verdicts, and what they left declared

- Paper: VETO LEVANTADO in pass 6, on the delta of pass 5.
- Complete green diff: VETO MANTENIDO on two [PRODUCT] findings (illegible digest
  with Aprobar; a long literal taking the bar off screen), cured.
- Pass on those cures: VETO LEVANTADO, four P3 folded.
- Pass on the director's §4 (FR-UI-68): VETO LEVANTADO. Three P3 left, declared
  here and not cured in this train:
  - [PRODUCT] the list row still collapses repeated spaces in the operation,
    which the digest seals; the document's OPERACIÓN card paints them. No
    decision is taken in the list. Filed in `docs/HANDOFF.md`.
  - [INSTRUMENT] the FR-UI-68 mould watches the parameters and the purpose; a
    rule narrowed to those two paragraphs would leave the OPERACIÓN card
    collapsing unseen.
  - [INSTRUMENT] the mould's oracle is the engine's text (`innerText`). With a
    long run of spaces, `pre-wrap` hangs them at the line end and the next word
    starts the following line: the text is kept, the eye does not see the run.

## The five questions, on the green

1. **Does any sentence claim more than the code? NO, after curing.**
   - **Search:** `grep` of all/every/each/never/always/only/exactly (and
     todos/cada/nunca/siempre/solo) over added lines in code, tests and docs.
   - **Scoped down:**
     - «never a paraphrase» → «not a paraphrase» (release note);
     - «never counts» → «does not count» (`ArmingField`);
     - «Every rule here» (`approvals.css`);
     - «384 px» → the measured 354;
     - «every control» → «the listed controls» (G1 title);
     - «a held key never arms» and the G5c/G5e titles;
     - «never let the observer» and «is never armed past» (comments in the browser spec).
   - **Kept, each exact against its code or mould:**
     - «the only scroll container» (G9 measures it);
     - «29 February in Gregorian leap years only» (the code's rule);
     - «Each test names the guarantee» in `e2e/approvals-mockup.spec.ts` (every title there starts with its G). Not a claim about AS-44, whose title («sin Aprobar») is wider than its list-only assertions; AS-44 is approved and not edited; its title is filed in `docs/HANDOFF.md`.
2. **Does any comment cite something that does not exist? NO.**
   - **Checked:** `design-drafts/approvals-screen-ux.html` (main repository),
     `src/styles/index.css`, the paper, `src/views/approvals.css`, FR-UI-11/39/40/48/49/51/57
     in the spec.
   - **Deleted probe:** the paper now says `e2e/zz-probe-arming.spec.ts` was
     deleted after its capture.
3. **Does any mould enter through a private function? NO.** The jsdom moulds
   render `<Approvals>`; the browser moulds enter by the harness (`POST
   /__test/park`, the bindings bridge). None imports an internal helper.
4. **Did I cure one door of a class with several? It was YES twice; cured.**
   The adversary's review of the diff found two sisters this answer had missed:
   - **Illegible class:** the illegible DIGEST still offered Aprobar and the
     arming row (FR-UI-15), next to the illegible expiry this train cured. Cured
     and moulded («FR-UI-15 · digest ilegible en el detalle»).
   - **Long untrusted text:** a 400-character body overflowed with `.main` as the
     single scroller. The sisters are every field of uncontrolled origin: body,
     purpose, channel, list row, the bar's raw illegible expiry, the state lines
     and the state paragraphs (a long start error). All wrap by one CSS rule.
     G9b moulds the body, purpose and channel in the list and the document, and
     a long start error in the stopped-core state. The bar's `span`s and
     `.approvals-state-line` are covered by the same rule without a mould of
     their own — declared, not claimed.
   - **Still listed, and holding:** six expiry sites, four `Unreadable`
     branches, two key handlers for IME/229, theme, and the channels for the
     tail.
5. **Did any cure enter without its mould and its captured red mutation? NO.**
   50 of 51 mutations are red with their lines above. The remaining one (B9d) is the
   withdrawn G5d, which no longer claims anything.
