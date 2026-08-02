---
layout: home

hero:
  name: Korvun
  text: Un binario. Tus modelos. Tus reglas.
  tagline:
    Pasarela de mensajería con IA autoalojada, router multimodelo y
    orquestador multicerebro — un único binario Go, de una Raspberry Pi a
    la nube.
  image:
    src: /brand/korvun-logo-hero.svg
    alt: Korvun — la marca K terminal
  actions:
    - theme: brand
      text: Empezar
      link: /es/guide/quickstart
    - theme: alt
      text: Ver en GitHub
      link: https://github.com/Sebastian197/korvun

features:
  - title: Pasarela de mensajería
    details:
      Canales de Telegram, Discord y webhook genérico en un solo proceso —
      un mensaje real entra, se enruta y la respuesta vuelve.
  - title: Router multimodelo
    details:
      Ollama local y Groq en la nube tras un mismo contrato de modelo, con
      coordinadores de fan-out, secuencial y reintentos.
  - title: Orquestador multicerebro
    details:
      Enruta cada conversación a un cerebro con sus propios modelos,
      política y personalidad.
  - title: Motor de políticas de despacho
    details:
      Políticas conscientes de privacidad, coste y consenso deciden qué
      modelos ven un mensaje y qué respuesta gana.
  - title: Builder visual sin código
    details:
      Compón canales, cerebros y modelos en un lienzo — la exclusión de
      privacidad se ve, y los cambios se aplican en caliente.
  - title: Korvun Desktop
    details:
      El mismo núcleo en una ventana nativa, empaquetado para macOS,
      Windows y Linux.
  - title: Autoalojado, binario único
    details:
      Un binario Go estático por plataforma — releases firmadas con SBOM
      para 6 plataformas, verificadas con cosign.
---

<!-- El clip del estreno (FR-LAND-2): mismo fichero committeado. Los assets
     de public/ se referencian con ruta absoluta raíz — VitePress los
     reescribe bajo el base '/korvun/' al construir (una ruta relativa aquí
     se trata como import de módulo y rompe el build; el harness del dist
     verifica las URLs finales). -->
<div class="k-clip">
  <video
    controls
    preload="metadata"
    poster="/media/korvun-v060-clip-poster.jpg"
    aria-label="Korvun v0.6.0 en 27 segundos: componiendo canales, cerebros y modelos en el lienzo del builder visual"
  >
    <source src="/media/korvun-v060-clip-1920x1080.mp4" type="video/mp4" />
  </video>
  <p class="k-clip-caption">
    Korvun v0.6.0 en 27 segundos —
    <a :href="withBase('/media/korvun-v060-demo-full-1080p.mp4')" target="_blank">mira la demo completa (43&nbsp;s)</a>.
  </p>
</div>

<script setup>
// Un href plano en HTML crudo NO recibe el base '/korvun/' (solo el
// pipeline de assets reescribe src/poster) — el harness del dist cazó el
// 404. withBase es el helper documentado para exactamente esto.
import { withBase } from 'vitepress'
</script>