# Releases

Every Korvun release ships **signed binaries for six platforms** (Linux,
macOS, Windows × x86-64, ARM64) plus the desktop apps: each artifact is
covered by a cosign-signed checksum manifest and an SBOM. How to download and
verify is in the [install guide](/guide/install).

**All releases live on GitHub:**
[github.com/Sebastian197/korvun/releases](https://github.com/Sebastian197/korvun/releases)

## The story so far

| Release | What it brought |
|---|---|
| [v0.9.2](https://github.com/Sebastian197/korvun/releases/tag/v0.9.2) | **Current release — the Beta (patch)** — the four use-breaking findings of the exhaustive UX audit: canvas cables can be disconnected, a provider change clears the orphan warmup, the reload trail is logged and its cutover contract pinned, and model health reaches the UI (builder badge + chat warning). |
| [v0.9.1](https://github.com/Sebastian197/korvun/releases/tag/v0.9.1) | **The Beta (patch)** — fixes-only patch from the desktop app audit: the Builder loses its embedded token gate, learns the openai-compatible provider truthfully, desktop configs always carry the admin block, the canvas answers (model-drop hint, panel close paths), the desktop logs to a file, and the webhook channel gains its wizard breadcrumb. |
| [v0.9.0](https://github.com/Sebastian197/korvun/releases/tag/v0.9.0) | **The Beta** — the universal model gateway: any OpenAI-compatible endpoint, cloud or local, becomes a first-class model by config alone — same policy engine, same declared-locality privacy, same governed tools (native tool calling included), with quota told apart from rate limits, redirects refused, and the API key never surfacing. Full changes are recorded in the GitHub release notes. |
| [v0.8.0](https://github.com/Sebastian197/korvun/releases/tag/v0.8.0) | Governed memory closes the last beta piece: bounded notes through the governed `memory_note` tool, deliberate `/recall` of a session's tail, `/notes` for the operator. Full changes are recorded in the GitHub release notes. |
| [v0.7.0](https://github.com/Sebastian197/korvun/releases/tag/v0.7.0) | **The operator console + governed tools & skills** — the Chat tab (takeover, sessions, direct chat), tri-state tool grants with shadow rehearsal, the network shield, markdown skills, native tool calling with an honest fallback. |
| [v0.6.0](https://github.com/Sebastian197/korvun/releases/tag/v0.6.0) | **The visual builder canvas** — drag channels, brains, and models from a palette, validator-checked cables, the privacy exclusion drawn as a gray dashed cable, persona per brain, hot apply. |
| [v0.5.0](https://github.com/Sebastian197/korvun/releases/tag/v0.5.0) | **The generic webhook channel** — POST JSON in, replies out to your URL; fail-closed Bearer auth, conversation identity, honest 503 on saturation. |
| [v0.4.0](https://github.com/Sebastian197/korvun/releases/tag/v0.4.0) | **Korvun Desktop** — the gateway behind a native window: first-run onboarding, secrets in the OS keychain, the builder embedded. |
| [v0.3.0](https://github.com/Sebastian197/korvun/releases/tag/v0.3.0) | **The Discord channel** — Gateway inbound with resume/reconnect, REST outbound, mentions blocked by default, a complete anti-loop family. |
| [v0.2.0](https://github.com/Sebastian197/korvun/releases/tag/v0.2.0) | **Resilience + the CLI** — boot warmup for local models, generous per-attempt timeouts, retry with differentiated fallback; `serve`, `config check`, `status`. |

Release notes are published on each GitHub release — this page stays a map,
not a mirror.
