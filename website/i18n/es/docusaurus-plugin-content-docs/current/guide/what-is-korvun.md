# ¿Qué es Korvun?

Korvun es **un único binario Go autoalojado** que es tres cosas a la vez:

- una **pasarela de mensajería** — Telegram, Discord y webhooks genéricos,
  de entrada y de salida;
- un **router multimodelo** — Ollama local y Groq en la nube tras un mismo
  contrato;
- un **orquestador multicerebro** — cada conversación enrutada a un cerebro
  con sus propios modelos, política y comportamiento.

Un mensaje real entra por un canal, se enruta a un cerebro, uno o varios
modelos responden, una política decide la respuesta, y esta vuelve — todo en
un solo proceso. El mismo binario estático corre en una Raspberry Pi y
escala en la nube; solo cambian las piezas de entrada/salida, por
configuración.

## El diferencial: el motor de políticas de despacho

La mayoría de las pasarelas reenvían mensajes. Korvun **decide** — el
enrutado es consciente de la privacidad y del coste, con consenso opcional,
todo como políticas de un mismo motor:

- **Privacidad** — un cerebro marcado `private` nunca envía su contenido a
  un modelo cloud. El selector de privacidad descarta los proveedores cloud
  *antes* de llamarlos, así que lo sensible no sale de tu máquina. El
  builder visual incluso dibuja la exclusión: un cable gris discontinuo
  hacia cada modelo cloud excluido.
- **Coste** — con despacho `sequential`, los modelos se prueban en orden y
  un proveedor de pago solo se contacta cuando el local ha fallado.
- **Consenso (opcional)** — los cerebros críticos pueden preguntar a varios
  modelos a la vez y quedarse con la respuesta en la que coincide una
  mayoría estricta.

## Lo que hay hoy

Todo lo de esta página está en la release actual — nada es hoja de ruta:

- **Canales**: Telegram (polling), Discord (Gateway de entrada, REST de
  salida) y un [endpoint webhook genérico](/es/channels/webhook) para
  cualquier cosa que sepa hacer POST de JSON.
- **Modelos**: [Ollama](https://ollama.com) local y Groq en la nube,
  mezclables en un mismo cerebro.
- **Cerebros**: conviven varios, cada uno con fan-out en paralelo o
  fail-over secuencial que ahorra coste.
- **El builder sin código**: configura canales, cerebros y modelos
  [visualmente en el navegador](/es/guide/builder) — lienzo de arrastrar y
  soltar incluido — con recarga en vivo segura, sin reiniciar.
- **Korvun Desktop**: el mismo núcleo tras una ventana nativa para macOS,
  Windows y Linux — con onboarding y los secretos en el llavero del
  sistema.
- **Memoria durable**: historial por conversación que sobrevive a
  reinicios (SQLite), opcional.
- **Observabilidad**: logs estructurados, `/metrics` de Prometheus,
  `/healthz` — en un servidor de administración solo-loopback por defecto.
- **Releases firmadas**: cada artefacto cubierto por un manifiesto de
  checksums firmado con cosign más un SBOM, para seis plataformas.

## Por dónde seguir

- Un bot respondiendo con un modelo local en minutos → [Inicio rápido](/es/guide/quickstart)
- Instalar el binario o la app de escritorio → [Instalación](/es/guide/install)
- Cada campo de configuración, explicado → [Referencia de configuración](/es/reference/configuration)
