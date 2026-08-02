# Inicio rápido

De cero a un bot de Telegram respondiendo con un modelo **local** — sin nube
por medio. Este flujo está validado de punta a punta en hardware real, y es
el mismo en Linux, macOS y Windows.

## Antes de empezar

- **El binario `korvun` instalado** — la
  [guía de instalación (EN)](/guide/install) cubre descarga y verificación.
  Confírmalo con `korvun --version`.
- **[Ollama](https://ollama.com)** para el modelo local (paso 1).
- **Un token de bot de Telegram**, de [@BotFather](https://t.me/BotFather)
  (`/newbot`).

## Paso 1 — Arranca Ollama y descarga un modelo

Korvun habla con un Ollama local en `http://127.0.0.1:11434` (el valor por
defecto). Deja Ollama corriendo en otra terminal:

```sh
ollama serve
ollama pull llama3.2:1b
```

## Paso 2 — Crea `korvun.local.json`

El archivo de la release incluye `korvun.example.json` — esta misma
configuración mínima — para copiar y adaptar en vez de teclear. Un canal de
Telegram, un cerebro, un modelo local:

```json
{
  "channels": [
    { "type": "telegram", "mode": "polling", "token_env": "TELEGRAM_TOKEN" }
  ],
  "brains": [
    {
      "name": "assistant",
      "sensitivity": "public",
      "policy": { "kind": "priority" },
      "models": [
        { "provider": "ollama", "model_id": "llama3.2:1b", "locality": "local" }
      ]
    }
  ],
  "routes": [
    { "channel": "telegram", "brain": "assistant" }
  ]
}
```

Los campos que más importan:

- **`token_env`** — el **nombre** de la variable de entorno que guarda el
  token del bot. El token nunca va en este fichero (paso 3).
- **`sensitivity`** — `public` (sin filtro) o `private` (los modelos cloud
  se descartan antes del despacho: nada sale de tu máquina). Con un único
  modelo local ambos se comportan igual; usa `private` para hacer la
  garantía explícita.
- **`policy`** — es un **objeto**, no una cadena: `{ "kind": "priority" }`
  elige la respuesta del proveedor de mayor prioridad que contestó.
- **`routes[].channel`** — el **nombre de tipo** del canal (`telegram`), no
  una etiqueta inventada.

Cada campo está documentado en la
[referencia de configuración (EN)](/reference/configuration).

## Paso 3 — Exporta el token

La configuración nombra la variable; el **valor** vive solo en el entorno:

```sh
export TELEGRAM_TOKEN=<your-bot-token>
```

En Windows (PowerShell): `$env:TELEGRAM_TOKEN = "<your-bot-token>"`.

> **El token del bot es un secreto.** Quien lo tenga controla tu bot. No lo
> pegues nunca en un fichero, un chat, una captura o un log. Si se expone,
> revócalo en @BotFather (`/mybots` → tu bot → **API Token** →
> **Revoke current token**) y exporta el nuevo.

## Paso 4 — Comprueba, arranca, escribe

Valida primero la configuración sin conexión — estructura y valores, sin
red y sin leer secretos:

```sh
korvun config check korvun.local.json      # OK -> exit 0
```

Añade `--preflight` para confirmar además que la variable resuelve y que
los proveedores responden. Después, arranca:

```sh
korvun serve --config korvun.local.json
```

Abre tu bot en Telegram y mándale un mensaje. La respuesta del modelo local
vuelve al chat — cero nube. Korvun sirve hasta `Ctrl-C` y apaga limpio.

## Si algo no cuadra

- **Mira el cableado en vivo** de un Korvun corriendo — cerebros, los
  modelos que sobrevivieron al selector de privacidad, canales:

  ```sh
  korvun status
  ```

  Dirección por defecto `127.0.0.1:2112`; apunta a otra con
  `--addr host:port`.

- **Un WARN de `DeleteWebhook` al arrancar** es esperado e inofensivo —
  Korvun limpia de forma proactiva cualquier webhook residual de Telegram
  antes de empezar el polling.

- **¿El primer mensaje tarda en hardware modesto?** Korvun calienta los
  modelos locales al arrancar y reintenta los fallos transitorios con
  tiempos generosos, así que la carga en frío está gestionada. Si aun así
  la primera respuesta tarda unos segundos, es el modelo cargándose — las
  siguientes son inmediatas.

## Siguientes pasos

- Configúralo **visualmente, sin JSON** → [el builder (EN)](/guide/builder)
- Añade [Discord (EN)](/channels/discord) o un
  [webhook (EN)](/channels/webhook)
- La versión completa de esta guía y del resto de la documentación está en
  inglés → [Quickstart](/guide/quickstart)