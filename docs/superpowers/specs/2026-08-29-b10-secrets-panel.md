# B10 — the Secrets panel in Ajustes: Design Spec

> **Status:** approved for TDD. UX contract: `design-drafts/ola2-designs.md`
> §2 — APROBADO POR CHANO 2026-08-23 (two visual-finish rounds sealed:
> Settings card, in-row editing/confirmation, house buttons; keychain
> error text sealed; unreadable-config notice names korvun.json +
> [Abrir carpeta] + [Reintentar]; loading = row skeleton). Governing:
> ADR-0010 (env-var NAMES, never values), ADR-0035 (shell bindings),
> ADR-0024 §1 discipline applied to secrets. External-docs note: stdlib +
> existing internal packages + existing frontend stack only.

## Goal

Ajustes gains a SECRETOS card: one row per secret NAME the config
references, with presence (env/keychain/absent), write-only update, and
confirmed delete — over the existing keychain bindings. The star case:
a boot failing on an absent secret is fixable from the panel with the
core STOPPED (the new name-only binding). THE invariant: no value is
ever shown, returned, logged, or carried by any new surface — write-only
by construction. Out of scope (sealed): editing the config (names come
FROM it), showing values ever.

## Functional requirements

- **FR-B10-1 (Go)** — `Desktop.ListSecretNames()` loads the config at the
  controller's known path (parse-only, no validation — a config that
  parses but fails validation still yields useful rows) and returns the
  DEDUPLICATED, deterministic list of referenced secret names:
  `channels[].token_env`, `channels[].webhook.outbound_token_env`,
  `brains[].models[].api_key_env`, `admin.token_env`. Names only — the
  return type cannot carry a value. Unreadable/unparseable config →
  error (the panel's notice state).
- **FR-B10-2 (Go)** — `Desktop.OpenConfigFolder()` opens the config
  file's directory in the OS file manager (GOOS-switched opener, seam
  injectable for tests). Powers the sealed [Abrir carpeta] fix.
- **FR-B10-3 (frontend discovery)** — core running: names come from
  `GET /api/config` through the shell proxy via a PURE extractor
  (`secretNamesOfConfig`, mirror of FR-B10-1); core stopped: the
  binding. Same rows either way. Loading = row skeleton (sealed).
- **FR-B10-4 (rows)** — name · presence dot + label (the wizard nuance:
  env WINS over keychain and the row says so) · [Actualizar…]/[Guardar…]
  opening an in-row masked one-shot field + Guardar (`SetSecret`; the
  field EMPTIES the instant of the call — wizard discipline) ·
  [×] → in-row confirmation → `DeleteSecret`. A name failing the
  binding's shape gate shows honest "no comprobable" presence.
- **FR-B10-5 (errors)** — keychain failure shows the SEALED text («El
  llavero del sistema rechazó la operación. Reintenta; si persiste,
  desbloquéalo en Acceso a Llaveros.») + [Reintentar]; discovery failing
  on BOTH paths shows the sealed notice naming korvun.json +
  [Abrir carpeta] + [Reintentar]. No error message ever includes a value.
- **FR-B10-6 (the invariant, test-negative)** — a typed value never
  appears in the DOM after Guardar, never rides any read binding
  (`CheckSecretPresence` takes the NAME only), and no new surface
  returns one. Empty config → the sealed empty-state copy.

## Acceptance scenarios

- **AS-1** a config referencing all four kinds of names lists exactly
  those rows, deduplicated, core stopped (binding) AND running (proxy).
- **AS-2** writing a value through a row calls SetSecret(name, value),
  empties the field immediately, and re-checks presence.
- **AS-3** presence renders from booleans only; env+keychain shows the
  env-wins nuance.
- **AS-4** delete asks in-row, then DeleteSecret; cancel touches nothing.
- **AS-5** keychain error → sealed text + retry; unreadable config →
  sealed notice + Abrir carpeta.
- **AS-6 (negative)** after typing and saving `s3cr3t-valor`, the string
  exists NOWHERE in the document; CheckSecretPresence was never called
  with it.

## Success criteria

`make quality` + `desktop-frontend-check` green, coverage floors intact;
zero changes to the control API (no new HTTP surface — values CANNOT
transit it by construction).

## Decisions folded in

- Parse-only discovery in Go (names from an invalid-but-parseable config
  beat an error — the panel exists for broken boots).
- OpenConfigFolder via os/exec GOOS switch with injectable opener seam.
- Names failing the presence shape-gate degrade to "no comprobable" —
  honest, never a crash.

## `[NEEDS CLARIFICATION]`

None — the three open points of the UX draft were sealed (error text,
unreadable-config notice, skeleton).
