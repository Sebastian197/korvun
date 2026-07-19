# Pieza 5 — App de escritorio (encuadre)

> **ACTUALIZACIÓN DE ESTADO (2026-07-19): los 8 `[NC-*]` están RESUELTOS**
> (NC-1/2/4/8 por Chano; NC-3/5/6/7 por el copiloto) y codificados como
> decisiones formales en **ADR-0035** (arquitectura de la app de escritorio) y
> **ADR-0036** (dependencia `wailsapp/wails/v2`). Este documento se conserva
> como registro del encuadre; las decisiones vigentes viven en los ADRs.

> **Estado: ENCUADRE, pendiente de la revisión del copiloto.** Este documento NO es
> el ADR y NO autoriza una sola línea de código. Encuadra la app de escritorio para
> que el copiloto vete las decisiones gordas (framework, forma de la app, alcance de
> firma) y resuelva los `[NEEDS CLARIFICATION]` antes de escribir ADR + TDD.
>
> Método: office-hours (challenge de premisas + alternativas honestas + doc-only, sin
> código). El estado de Wails y Tauri se verificó con **Context7 + fuentes primarias
> (GitHub releases, docs oficiales)** a 2026-07-19, ANTES de fijar nada (regla
> CLAUDE.md). Nada de lo que sigue viene de memoria.

## 0. Punto de partida — qué fijó el maestro y por qué se re-verifica

El doc maestro (§4.2, Etapa 15 del plan original) y `ROAD-TO-BETA.md` (Pieza 5)
fijaron **"app Wails"** por afinidad con Go: mismo lenguaje, el frontend del builder
ya existe, un solo toolchain. Esa decisión se escribió hace tiempo y **sin fijar
versión**. Este encuadre re-verifica la premisa con datos frescos en vez de heredarla.

Lo que la Pieza 5 debe entregar (del ROAD-TO-BETA, sin cambiar):

- Shell de escritorio: ventana nativa que sirve el builder + arranca/supervisa un
  Korvun embebido, **sin terminal**.
- Empaquetado por SO + firma "si se distribuye".
- `make quality` verde y **el binario headless intacto** ("la app es una carcasa,
  no un fork de la lógica").
- NO es criterio V1: objetivo de *ejecución* (presentación profesional), no de
  *criterio*.

## 1. Realidad verificada del ecosistema (a 2026-07-19)

### Wails v2 — ESTABLE. Wails v3 — SIGUE EN ALPHA

**El dato que cambia el encuadre: Wails v3 NO es estable a julio de 2026.** La
release más reciente es `v3.0.0-alpha2.117` (julio 2026, cadencia diaria), con el
aviso "pre-release software" en cada release; el propio roadmap declara el objetivo
de llegar a Beta, sin fecha anunciada. El estable real es **`v2.13.0`** (julio
2026, mantenimiento activo). Fuentes: `github.com/wailsapp/wails/releases`,
`v3.wails.io/status`, `v3.wails.io/blog`.

**Wails v2 (verificado en `wails.io/docs`):**

- **Plataformas:** Windows 10/11 (AMD64/ARM64), macOS 10.15+ (AMD64) y 11.0+
  (ARM64, con `darwin/universal` disponible), Linux (AMD64/ARM64). **Los 3 SO +
  ARM64 cubiertos.**
- **WebView del sistema** (no embebe Chromium): WebView2 en Windows (runtime
  descargable/embebible con estrategias configurables), WKWebView en macOS,
  **WebKitGTK en Linux** — esto último implica **cgo + `libgtk-3` + `libwebkit2gtk`
  como dependencias de build Y de runtime** del binario de escritorio.
- **Frontend agnóstico:** cualquier bundle web (React/Vite incluido) embebido vía
  `go:embed` — exactamente el modelo que `web/builder` ya usa.
- **Empaquetado:** `.app` en macOS, instalador **NSIS** en Windows (`wails build
  -nsis`). En Linux v2 produce el binario; **no genera `.deb`/AppImage nativamente**.
- **Firma:** no la automatiza; la doc oficial la delega a `codesign`/notarización en
  macOS y `signtool` en Windows dentro de CI (su guía usa `gon`, hoy superada por
  `notarytool` de Apple — detalle a fijar en fase ADR).

**Wails v3 (por qué NO hoy):** mejor packaging (`.deb`/AppImage/cross-compile con
Docker) y API nueva, pero **alpha explícita**. Adoptar una alpha como dependencia de
producción contradice la disciplina de dependencias del proyecto (ADR + test de 4
ejes; el eje de madurez falla de salida). La migración v2→v3 está documentada por el
propio proyecto (`v3.wails.io/migration/v2-to-v3`) — el coste de migrar DESPUÉS,
cuando v3 sea estable, es conocido y acotado.

### Tauri v2 — la alternativa evaluada honestamente

- **Estable** y maduro, con la mejor doc de firma/distribución del segmento
  (macOS sign+notarize, Windows signtool/Azure, AppImage/deb/rpm nativos).
- Modelo aplicable a Korvun: **sidecar** (`bundle.externalBin`) — empaqueta el
  binario `korvun` existente con sufijo target-triple
  (`korvun-x86_64-unknown-linux-gnu`, …) y lo supervisa desde Rust/JS
  (`plugin-shell`: spawn, stdin/stdout, kill).
- **El coste que lo mata aquí:** el shell es **Rust** — un segundo lenguaje, un
  segundo toolchain (cargo + crates), un segundo universo de deps a auditar, para un
  proyecto cuya identidad es "un solo binario Go" y cuyo mantenedor es uno.
  Además el modelo sidecar duplica artefactos (shell + binario korvun dentro del
  bundle: dos cosas que firmar, dos versiones que pueden divergir).

### Coste real de la firma de código para un desarrollador individual (verificado)

| SO | Qué exige | Coste real | Sin firma |
|---|---|---|---|
| macOS | Cert Developer ID + **notarización** (fuera del App Store) | **Apple Developer Program $99/año** (la notarización va incluida) | Gatekeeper bloquea el `.app` descargado; workaround documentable (clic derecho → Abrir / `xattr -dc`) pero hostil para no-técnicos |
| Windows | Cert de firma de código (OV) | **~$216–520/año** (Sectigo/DigiCert; desde 2026-02-15 máx. 1 año de vigencia). **Azure Trusted Signing ($9.99/mes) NO está disponible para individuos fuera de USA/Canadá** — en España solo aplica como organización | SmartScreen avisa "editor desconocido"; el usuario puede continuar con "Más información → Ejecutar de todas formas" |
| Linux | Nada obligatorio | $0 — la cadena cosign + checksums de Stage 16 ya cubre la integridad | n/a |

**Lectura honesta:** para un desarrollador individual en España, la firma completa
cuesta ~$99/año (macOS) + ~$216+/año (Windows) recurrentes. Linux es gratis. Qué se
paga en v1 y qué se difiere es decisión de Chano → `[NC-1]`, `[NC-2]`.

## 2. Challenge de la premisa (office-hours)

**Premisa heredada: "la app de escritorio se hace con Wails".**

- ¿Sigue en pie con datos de 2026? **Sí, pero con la versión pinneada: Wails v2
  (v2.13.x), NO v3.** El maestro dijo "Wails" sin versión; hoy eso es ambiguo y la
  mitad de la doc que devuelve una búsqueda es de la v3 alpha. La afinidad con Go
  sigue siendo el argumento decisivo: reusa el lenguaje, el módulo, el `make
  quality`, el conocimiento del proyecto, y permite (opción A abajo) que el core
  corra **in-process** — cosa que Tauri no puede ofrecer jamás (siempre sidecar).
- ¿Tauri lo desbanca? No para este proyecto: su ventaja (tooling de firma/packaging
  maduro) no compensa el coste estructural de introducir Rust. Queda registrado como
  alternativa evaluada con razón, no como omisión.
- ¿Y "esperar a v3"? No: sin fecha de estable anunciada, condicionar la Pieza 5 a
  un tercero sin fecha es ceder el control del roadmap. v2 hoy + migración
  documentada cuando v3 sea estable.

## 3. FORMA de la app — la decisión estructural

### La restricción innegociable primero: el headless NO puede romperse

La Pi no tiene escritorio. El binario headless actual se cross-compila **×6 con
`CGO_ENABLED=0`**; un binario Wails en Linux linkea **WebKitGTK/GTK vía cgo** y no
arranca sin esas librerías. Conclusión forzada, no opinable:

> **El mismo binario es IMPOSIBLE.** La app de escritorio es un **binario hermano**
> (`cmd/korvun-desktop`) en el mismo módulo, compartiendo los MISMOS paquetes
> `internal/` (config, app, router, brains, canales). El `cmd/korvun` headless y su
> pipeline GoReleaser ×6 quedan **byte a byte intactos**. Go solo linkea lo que cada
> binario importa: la dependencia Wails vive únicamente en el grafo del binario
> desktop.

### Alternativa A — core in-process (la app ES korvun con carcasa) — RECOMENDADA

`cmd/korvun-desktop` importa `internal/app` y arranca el core **dentro del mismo
proceso** que la ventana. Elegir config → `config.Load` + `app.Build`; parar →
`App.Shutdown` (el teardown limpio ya existe y está testeado); la UI habla con la
admin API loopback de siempre.

- ✅ **Un solo artefacto por SO** — una cosa que empaquetar, una que firmar.
- ✅ Cero descubrimiento de binarios, cero skew de versiones core/shell.
- ✅ Reusa `App.Run/Shutdown` tal cual; el supervisor de reload (Stage 14 P2a) ya
  existe para el ciclo config→rebuild.
- ❌ Un pánico del core tumbaría la ventana (mitigado: la disciplina no-panic del
  proyecto + el funnel de errores del router; riesgo R3 abajo).
- ❌ Restart del core = rebuild in-process; hay que probar que no fuga (goroutines/
  puertos) tras N ciclos start/stop — test explícito en el TDD de la pieza.

### Alternativa B — subprocess (la app lanza el binario korvun existente)

El shell empaqueta y lanza `korvun serve` como proceso hijo y lo supervisa
(el equivalente Wails del sidecar de Tauri).

- ✅ Aislamiento de fallos total; el core es EXACTAMENTE el artefacto ya validado.
- ✅ Restart trivial (matar/relanzar proceso).
- ❌ Dos binarios dentro del bundle: dos cosas que firmar/notarizar, skew posible.
- ❌ Gestión de proceso por SO (señales en Windows ≠ Unix), descubrimiento de ruta,
  y el `.app`/instalador crece.

**Recomendación: A**, con B como fallback documentado si el rebuild in-process
resulta leaky en la práctica. La razón de fondo: la promesa de Korvun es "un solo
binario"; A la conserva también en el escritorio.

## 4. Reutilización del builder web existente

`web/builder` es React 19 + Vite + Tailwind 4, compila a `web/builder/dist` y ya se
embebe en el binario vía `go:embed` sirviéndose en `/ui/`. En la app de escritorio
**no se reescribe nada**: el WebView carga ese mismo bundle y habla con la misma
admin API loopback.

**Matiz de origen (corregido en revisión):** en el navegador el builder es
same-origin porque el MISMO servidor del core sirve `/ui/` y `/api/*`. En una
ventana Wails el frontend se sirve desde el asset server propio de Wails (origen
`wails://` / `wails.localhost`), así que un fetch/EventSource a `127.0.0.1:2112`
sería **cross-origin**, y el admin API hoy no tiene CORS (verificado: cero
`Access-Control-Allow` en el código). El ADR debe elegir: **proxyar `/api/*` a
través del AssetServer handler de Wails** (restaura same-origin sin tocar el core,
opción preferida a priori) o añadir CORS restringido al admin API → `[NC-6]`.

- **Qué cambia:** el "navegador" pasa a ser la ventana nativa; opcionalmente un
  `index` de escritorio con controles de ciclo de vida (sección 5) alrededor del
  builder. Nada del contrato HTTP.
- **Qué NO cambia:** el builder, su build, sus tests (Vitest/Playwright), el embed.
- **Quién sirve los assets y qué se ve con el core parado:** si el WebView cargara
  `/ui/` del servidor del core, pulsar "stop" mataría la página que contiene los
  controles. El **chrome del shell (start/stop/selector de config/semáforo) debe
  servirlo el shell y sobrevivir al apagado del core**; el builder puede vivir
  dentro (proxy/iframe) o recargarse al arrancar el core. Decisión de seam →
  `[NC-6]`. El riesgo de duplicar el bundle (drift con `/ui/` embebido, agrava R5)
  se resuelve embebiendo `web/builder/dist` UNA vez y sirviéndolo por las dos vías
  desde el mismo `embed.FS`.
- **Qué gana el usuario de escritorio:** doble clic y ver el builder sin saber qué
  es un puerto; el ciclo instalar→configurar→hablar con un modelo sin abrir una
  terminal — el objetivo declarado de la pieza.

## 5. Ciclo de vida desde la UI — la admin API existente ES el puente

Superficie ya existente (verificada en código, no de memoria):

| Ruta | Uso desde la app |
|---|---|
| `GET /healthz` | semáforo running/stopped |
| `GET /metrics` | contadores (reconexiones, drops) si se quieren mostrar |
| `GET /api/brains`, `GET /api/channels` | estado resuelto del boot |
| `GET /api/events` (SSE) | live-view del pipeline de mensajes en la ventana |
| `GET/POST /api/config` (bearer) | leer/mutar config desde el builder |
| `GET /api/reload/{handle}` | estado del reload supervisado |

Lo ÚNICO nuevo que la app añade es lo que la API no puede dar por definición:
**arrancar/parar el proceso-core y elegir el fichero de config** (en A, llamadas
in-process a `app.Build/Run/Shutdown`; expuestas a la UI vía bindings Wails o un
mini-endpoint local del shell — decisión de ADR). Todo lo demás, por HTTP como hoy.
El modo "conectar a un Korvun remoto (la Pi)" usaría la MISMA API, pero es alcance
v2 → exclusiones.

**Tres huecos que la revisión adversarial destapó (los tres van a NC, no se
resuelven aquí):**

- **Secrets/env desde una app GUI (el hueco gordo).** Los tokens de canal, las API
  keys cloud y el bearer del admin API son **env-only en piedra** (ADR-0010 §3 /
  ADR-0028) — y una app lanzada con doble clic NO hereda el env de una shell. Tal
  cual, el flujo "configurar y hablar con un modelo **sin terminal**" no puede
  cumplirse. El ADR debe resolver cómo el shell aprovisiona env al core **sin
  violar la regla env-only** (p. ej. keychain del SO → inyección en el entorno del
  proceso ANTES de `app.Build`; bearer del admin generado in-process). → `[NC-7]`
- **Primer arranque (huevo-y-gallina).** "Elegir config → `config.Load` +
  `app.Build`" presume que existe un fichero de config; pero el builder
  (`POST /api/config`) solo existe con el core ya arrancado. El primer uso
  no-técnico necesita un camino: plantilla mínima embebida que el shell escribe en
  el primer arranque, o flujo builder-antes-de-boot. → `[NC-8]`
- **Colisión de puerto headless↔desktop.** Un `korvun` headless corriendo y el
  core del desktop querrían ambos `127.0.0.1:2112` por defecto. El core desktop
  debería usar puerto efímero (el accessor `Server.Addr()` ya permite descubrirlo)
  o detectar el conflicto — a fijar en el ADR.

## 6. Empaquetado por SO y CI — qué puede construir GitHub Actions

- **Pipeline headless: INTACTO.** El release GoReleaser ×6 `CGO_ENABLED=0` no se
  toca. La app desktop tiene su **workflow propio** con matrix nativa (cgo obliga):
  `macos-latest` (`.app` universal + `.dmg`), `windows-latest` (NSIS `.exe`,
  AMD64), `ubuntu` (binario AMD64 + `.desktop` en `tar.gz`).
- **Linux `.deb`/AppImage:** Wails v2 no los genera; producirlos (nfpm/appimagetool)
  es trabajo extra → propuesta: diferir a v1.x y entregar `tar.gz` en v1 → `[NC-4]`.
- **ARM64 desktop:** macOS queda cubierto por el binario universal. Windows ARM64 y
  Linux ARM64 requieren runners/cross-compile específicos — GitHub ofrece runners
  Linux ARM64 para repos públicos (a verificar en fase ADR, regla "verificar tags en
  fuente") → propuesta: diferir ambos → `[NC-4]`.
- **Firma en CI:** cosign ya firma checksums (cadena Stage 16) y aplica igual a los
  artefactos desktop. La firma de plataforma (notarización/signtool) depende de
  `[NC-1]`/`[NC-2]`; si se paga, va como secrets + pasos de workflow (patrones
  documentados y verificados en las docs de Wails y de las Actions de Apple import).

## 7. La decisión de dependencia que viene

`github.com/wailsapp/wails/v2` sería la **5ª dependencia directa** del módulo (hoy:
telegram-bot, sqlite, prometheus, coder/websocket). Va a su ADR con el **test de 4
ejes** del proyecto, con estas notas ya recogidas: solo la importa el binario
desktop (el grafo del headless no cambia); versión pinneada v2.13.x; plan de
migración a v3 documentado por upstream; cgo solo en el binario desktop. **Context7
sobre la doc v2 es prerrequisito duro antes de la primera línea** (regla CLAUDE.md,
ya ejercitada en este encuadre).

## 8. Alcance v1 honesto de la pieza

**Dentro:**

1. `cmd/korvun-desktop` (Wails v2, opción A in-process) + ADR(s).
2. Ventana que sirve el builder existente + controles start/stop/config + semáforo
   `/healthz` + live-view SSE.
3. Artefactos: `.dmg` (universal), instalador NSIS (AMD64), `tar.gz` Linux (AMD64).
4. Workflow CI desktop propio; headless intacto; `make quality` verde.
5. Validación en hardware real (el iMac de Chano) como en las Piezas 1–4.

**Fuera (explícito, con razón):**

- Conexión a un Korvun remoto (la Pi) desde la app — misma API, alcance v2.
- `.deb`/AppImage, Windows ARM64, Linux ARM64 desktop — `[NC-4]`.
- Auto-update de la app — otra pieza (Sparkle/winget/etc., cada una con su coste).
- Tray icon / arranque al login / minimizar a bandeja — azúcar post-v1.
- App Store / Microsoft Store — canales con coste y review propios, fuera de beta.

**Riesgos y mitigación:**

- **R1 — v3 sale estable a mitad de pieza:** seguir en v2 igualmente; la migración
  es un chore posterior acotado y documentado por upstream. No re-decidir en caliente.
- **R2 — firma no pagada degrada la primera impresión** (Gatekeeper/SmartScreen):
  decidirlo ANTES de publicar (NC-1/NC-2) y documentar el workaround con capturas en
  la guía de instalación, como se hizo con el paso manual del intent de Discord.
- **R3 — un fallo del core tumba la ventana (opción A):** el core ya es no-panic
  por disciplina; añadir al TDD un test de N ciclos start/stop sin fugas y el
  fallback B documentado si aparece un leak estructural.
- **R4 — WebView2 ausente en Windows viejos:** Wails trae estrategias de instalación
  del runtime (download/embedded) — elegirla en el ADR, no improvisarla.
- **R5 — deriva del bundle del builder** (el quirk conocido de `make build`
  regenerando `dist`): la pieza hereda el chore aparcado; no se resuelve aquí, se
  menciona para que no sorprenda en los builds desktop.

## 9. [NEEDS CLARIFICATION] — para la revisión del copiloto

- **[NC-1] Firma macOS:** ¿se paga el Apple Developer Program ($99/año) para
  notarizar el `.dmg` en la v1 de la pieza, o v1 sale sin firmar con el workaround
  de Gatekeeper documentado? (Decisión de coste recurrente — es de Chano.)
- **[NC-2] Firma Windows:** ¿se asume SmartScreen sin firma en v1 (coste $0), se
  compra cert OV (~$216+/año), o se constituye vía como organización para Azure
  Trusted Signing ($9.99/mes)? Propuesta honesta: sin firma en v1 + documentación.
- **[NC-3] Forma de la app:** ¿se aprueba la opción A (in-process, binario hermano
  `korvun-desktop`) con B como fallback, tal como recomienda este encuadre?
- **[NC-4] Matriz Linux/ARM64 v1:** ¿basta `tar.gz` AMD64 en v1, difiriendo
  `.deb`/AppImage y los ARM64 de Windows/Linux desktop? (macOS ARM64 sí entra vía
  binario universal.)
- **[NC-5] Nombre y convivencia de artefactos:** ¿`korvun-desktop` como nombre de
  binario/artefacto, releases en el MISMO tag SemVer que el headless (un release,
  9+ artefactos) o cadencia propia?
- **[NC-6] Exposición del ciclo de vida y seam de assets:** (a) ¿bindings nativos
  Wails para start/stop/config (JS↔Go directo) o mini-endpoints locales del shell
  sobre HTTP? (b) ¿proxy de `/api/*` vía AssetServer de Wails (same-origin, core
  intacto) o CORS restringido en el admin API? (c) el chrome del shell debe
  sobrevivir al apagado del core (§4) — confirmar ese requisito. Afecta a la
  testabilidad del shell — a fijar en el ADR.
- **[NC-7] Secrets env-only desde GUI:** ¿cómo aprovisiona el shell los tokens/keys
  al core sin terminal y sin violar ADR-0010 §3 (env-only)? Propuesta a evaluar en
  el ADR: keychain del SO → inyección en el env del propio proceso antes de
  `app.Build` + bearer del admin generado in-process. Este NC condiciona la promesa
  central de la pieza ("sin terminal") — es el primero a resolver.
- **[NC-8] Primer arranque sin config:** ¿plantilla mínima embebida que el shell
  escribe en el primer uso (propuesta: derivar de `configs/edge.json`), o flujo
  builder-antes-de-boot? Sin esto el usuario no-técnico no tiene por dónde empezar.

## 10. Fuentes de la verificación (2026-07-19)

- Wails releases: `github.com/wailsapp/wails/releases` (v2.13.0 estable;
  v3.0.0-alpha2.117 pre-release).
- Wails v2 docs (Context7 `/websites/wails_io`): plataformas, deps por SO, NSIS,
  guía de firma, `darwin/universal`.
- Wails v3 estado (Context7 `/websites/v3_wails_io` + `v3.wails.io/status`): alpha,
  objetivo beta, migración v2→v3, cross-compile Docker.
- Tauri v2 docs (Context7 `/tauri-apps/tauri-docs`): sidecar `externalBin` +
  target-triples, `plugin-shell`, firma macOS/Windows/Linux.
- Firma Windows: Microsoft Learn (code signing options), Azure Artifact/Trusted
  Signing pricing ($9.99/mes; individuos solo USA/Canadá), CAs OV (~$216+/año,
  vigencia máx. 1 año desde 2026-02-15).

**Pendiente de verificar en fase ADR (no bloquea el encuadre, sí la primera línea
de código):** compatibilidad de Wails v2.13.x con el Go del repo (`go 1.26.x`) —
comprobar en la matriz de soporte de Wails antes del `go get`; que **el AssetServer
de Wails soporta respuestas streamed/flushed (necesario para el SSE del live-view
del admin API)** — a comprobar en la primera sub-fase técnica de la pieza, porque
un proxy que bufferea rompería el live-view en silencio y forzaría la vía CORS de
`[NC-6b]`; y la disponibilidad real de runners Linux ARM64 en GitHub Actions si
`[NC-4]` decide incluir ese target.
