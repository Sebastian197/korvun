# ADR-0002: Defer WhatsApp channel until core is in place

> **Status:** accepted (deferral decision)
> **Date:** 2026-06-13
> **Deciders:** Sebastián Moreno Saavedra

## Context

WhatsApp is a target channel for Korvun. However, integrating it differs
fundamentally from Telegram and carries external dependencies and design
implications that justify deferring its implementation until the core of
the system (router, model abstraction, policy engine, brains — Stages 3
through 7) is working.

Key facts as of 2026 that drive this decision:

- **Cloud API is the only path for new integrations.** As of October 2025,
  Meta deprecated the on-premise WhatsApp API. New integrations must use the
  WhatsApp Cloud API, hosted entirely on Meta's servers. There is no
  self-hosted option.
- **Account model requires Meta verification.** A business must own its own
  WhatsApp Business Account (WABA). Onboarding requires a verified Meta
  Business Manager account (typically 1–3 business days), a dedicated phone
  number, public display-name setup, and message templates that must be
  pre-approved by Meta before they can be sent.
- **Per-message pricing.** Since July 2025, Meta charges per delivered
  message, with categories (Marketing, Utility, Authentication, Service).
  Service replies inside the 24-hour window are free; templates are charged.

## Decision

**Defer the WhatsApp channel.** Do not implement it until Stages 3–7 (router,
models, policy engine, brains) are complete. This is a conscious deferral, not
a cancellation.

### Why deferred (not cancelled)

1. **External dependency outside the team's control.** WABA verification,
   dedicated number, and Meta template pre-approval introduce lead times and
   gating that do not depend on development effort. Telegram, by contrast,
   needs only a bot token.
2. **Conflicts with the self-hosted principle.** The Cloud API runs on Meta's
   servers; WhatsApp messages leave the machine to Meta by design. This has a
   direct consequence for the policy engine (see below).
3. **Per-message cost.** The paid, per-message model affects Korvun's
   cost-analytics features and must be modelled when implemented.

## Consequence for the policy engine (Stages 5–6) — act on this now

Even though WhatsApp is not implemented yet, its "data leaves to a third party"
nature MUST inform the design of the channel model and the policy engine:

- The channel abstraction should carry a privacy/egress attribute so a policy
  can know that a given channel exposes data to a third party.
- A privacy-routing policy MUST NEVER treat a WhatsApp message as
  "private/local", because the message has already transited Meta's servers.
- This attribute should be designed in Stages 5–6 so that adding WhatsApp later
  requires no rework of the policy engine.

## Technical choice to make when executed (two paths)

- **Path A — Meta Cloud API directly.** No intermediary, no markup, but the
  project manages the webhook, templates, and verification itself. More
  control, more work. Most coherent with Korvun's self-hosted, control-first
  ethos.
- **Path B — BSP (Business Solution Provider, e.g. Twilio, 360dialog).**
  Faster to go live, but adds markup cost and an extra dependency.

**Preliminary recommendation:** Path A (direct Cloud API), to avoid the
intermediary. To be confirmed against Meta's current documentation (via
Context7 / official docs) when the work is actually scheduled.

## Target scope when implemented

Equivalent to "Telegram complete" where the WhatsApp API allows it:

- Text inbound/outbound (always available)
- Media (image, audio, video, document) — supported
- Locations — supported
- Interactive messages (buttons, list messages) — different model from
  Telegram; via templates / interactive message types
- Webhook lifecycle (real reception via Meta's webhook) — supported

**Known divergences from Telegram (to confirm at execution time):** message
editing and reactions are not exposed the same way as in Telegram; "commands"
do not exist as a native concept. Document what is viable when implemented.

## Alternatives Considered

- **Implement WhatsApp now, in parallel with Telegram.** Rejected: it is the
  scope-explosion risk the master document explicitly warns against, gated by
  external (Meta) timelines, and would divert effort from Korvun's actual
  differentiator (the policy engine). Deferring loses nothing and de-risks the
  schedule.
- **Use an unofficial WhatsApp library (reverse-engineered Web protocol).**
  Rejected: violates WhatsApp's terms of service and risks the phone number
  being banned at any time. Not a foundation to build a product on.
