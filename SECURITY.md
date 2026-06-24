# Security Policy

## Reporting a vulnerability

**Please do not report security vulnerabilities through public GitHub issues,
pull requests, or discussions.** Public disclosure before a fix puts every user
at risk.

Instead, report privately through one of:

- **GitHub Security Advisories** — preferred. Go to the repository's
  **Security → Advisories → Report a vulnerability** tab to open a private
  advisory ([draft a new one](https://github.com/Sebastian197/korvun/security/advisories/new)).
- **Email** — `morenosebastian117@gmail.com` with the subject prefixed
  `[korvun-security]`.

Please include, as far as you can:

- the affected version, commit, or branch;
- a description of the vulnerability and its impact;
- steps to reproduce (a minimal proof of concept is ideal);
- any known mitigations or workarounds.

Do **not** include real secrets (API keys, tokens) in your report. If a secret
was exposed, revoke it first, then describe the exposure without pasting the
value.

## Response expectations

This is currently a single-maintainer project, so timelines are best-effort:

| Stage | Target |
|-------|--------|
| Acknowledge receipt | within 5 business days |
| Initial assessment / severity triage | within 10 business days |
| Fix or mitigation plan | depends on severity and complexity, communicated after triage |

We will keep you informed of progress and coordinate a disclosure timeline with
you. With your consent, we are happy to credit you once a fix is released.

## Supported versions

Korvun is in pre-1.0, staged development. Until a `1.0.0` release, only the
latest `master` receives security fixes; there are no maintained release
branches yet. This table will be updated once versioned releases begin.

| Version | Supported |
|---------|-----------|
| `master` (latest) | ✅ |
| any tagged pre-release | ⚠️ best-effort, upgrade to latest |

## Scope and design notes

- **Secrets are environment-only by reference.** Korvun never reads secrets from
  argv, the config file, logs, or error messages (ADR-0010 §3). Reports about
  secrets leaking into any of those surfaces are in scope and welcome.
- Input is validated at every channel boundary; the dispatch policy engine can
  keep sensitive payloads on local-only models.
- Supply-chain hygiene: dependencies are minimized and justified by ADRs;
  `govulncheck` and `gosec` run in CI, and an SPDX SBOM is produced per build.