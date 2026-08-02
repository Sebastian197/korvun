---
layout: home

hero:
  name: Korvun
  text: One binary. Your models. Your rules.
  tagline:
    Self-hosted AI messaging gateway, multi-model router, and multi-brain
    orchestrator — one Go binary, from a Raspberry Pi to the cloud.
  image:
    src: /brand/korvun-logo-hero.svg
    alt: Korvun — the K terminal mark
  actions:
    - theme: brand
      text: Get started
      link: /guide/quickstart
    - theme: alt
      text: View on GitHub
      link: https://github.com/Sebastian197/korvun

features:
  - title: Messaging gateway
    details:
      Telegram, Discord, and generic webhook channels in one process — a
      real message enters, is routed, and the reply goes back.
    link: /channels/telegram
    linkText: Channel guides
  - title: Multi-model router
    details:
      Local Ollama and cloud Groq behind one model contract, with fan-out,
      sequential, and retry coordinators.
  - title: Multi-brain orchestrator
    details:
      Route each conversation to a brain with its own models, policy, and
      persona.
  - title: Dispatch policy engine
    details:
      Privacy-, cost-, and consensus-aware policies decide which models see
      a message and which answer wins.
  - title: No-code visual builder
    details:
      Compose channels, brains, and models on a canvas — the privacy
      exclusion is visible, and changes hot-reload.
    link: /guide/builder
    linkText: Meet the builder
  - title: Korvun Desktop
    details:
      The same core in a native window, packaged for macOS, Windows, and
      Linux.
    link: /guide/install
    linkText: Install
  - title: Self-hosted, single binary
    details:
      One static Go binary per platform — signed releases with SBOM for 6
      platforms, cosign-verified.
    link: /releases/
    linkText: Releases
---

<!-- The launch clip (FR-LAND-2): committed, same-origin, click-to-play.
     public/ assets are referenced root-absolute — VitePress rewrites them
     under the '/korvun/' base at build (a bare relative path here is
     treated as a module import and breaks the build; found empirically,
     the dist harness verifies the final URLs). -->
<div class="k-clip">
  <video
    controls
    preload="metadata"
    poster="/media/korvun-v060-clip-poster.jpg"
    aria-label="Korvun v0.6.0 in 27 seconds: composing channels, brains, and models on the visual builder canvas"
  >
    <source src="/media/korvun-v060-clip-1920x1080.mp4" type="video/mp4" />
  </video>
  <p class="k-clip-caption">
    Korvun v0.6.0 in 27 seconds —
    <a :href="withBase('/media/korvun-v060-demo-full-1080p.mp4')" target="_blank">watch the full demo (43&nbsp;s)</a>.
  </p>
</div>

<script setup>
// A plain href in raw HTML does NOT get the '/korvun/' base (only the
// asset pipeline rewrites src/poster) — the dist harness caught the 404.
// withBase is the documented helper for exactly this.
import { withBase } from 'vitepress'
</script>
