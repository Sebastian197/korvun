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

<!-- The privacy scene (SP2b): the differential gets its own moment. All
     claims verified against v0.6.0 — the gray dashed cable to cloud
     models on private brains is shipped behavior. -->
<section class="k-privacy" aria-labelledby="k-privacy-title">
  <h2 id="k-privacy-title">Privacy you can see</h2>
  <p>
    Mark a brain private and the builder shows the exclusion instead of
    burying it: a gray dashed cable to every cloud model. Sensitive
    conversations stay with local models — a routing policy drawn on the
    canvas, not a promise in a README.
  </p>
  <div
    class="k-privacy-diagram"
    role="img"
    aria-label="A private brain linked to a local model by a solid violet cable, and excluded from a cloud model by a gray dashed cable"
  >
    <div class="k-wire-row">
      <span class="k-node">private brain</span>
      <span class="k-cable k-cable-live" aria-hidden="true"></span>
      <span class="k-node">local model</span>
    </div>
    <div class="k-wire-row">
      <span class="k-node">private brain</span>
      <span class="k-cable k-cable-excluded" aria-hidden="true"></span>
      <span class="k-node k-node-dim">cloud model — excluded</span>
    </div>
  </div>
</section>

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
