# Releases

Cada release de Korvun trae **binarios firmados para seis plataformas**
(Linux, macOS, Windows × x86-64, ARM64) más las apps de escritorio: cada
artefacto está cubierto por un manifiesto de checksums firmado con cosign y
un SBOM. Cómo descargar y verificar está en la
[guía de instalación](/guide/install).

**Todas las releases viven en GitHub:**
[github.com/Sebastian197/korvun/releases](https://github.com/Sebastian197/korvun/releases)

## La historia hasta hoy

| Release | Qué trajo |
|---|---|
| [v0.8.0](https://github.com/Sebastian197/korvun/releases/tag/v0.8.0) | **Release actual — la Beta** — la memoria gobernada cierra la última pieza de la beta: notas acotadas mediante la herramienta gobernada `memory_note`, `/recall` deliberado de la cola de una sesión, `/notes` para el operador. Los cambios completos están en las notas de GitHub. |
| [v0.7.0](https://github.com/Sebastian197/korvun/releases/tag/v0.7.0) | **La consola del operador + herramientas y skills gobernadas** — la pestaña Chat (takeover, sesiones, chat directo), permisos tri-estado con ensayo en shadow, el escudo de red, skills en markdown, tool calling nativo con fallback honesto. |
| [v0.6.0](https://github.com/Sebastian197/korvun/releases/tag/v0.6.0) | **El lienzo del builder visual** — arrastra canales, cerebros y modelos desde una paleta, cables validados, la exclusión de privacidad dibujada como cable gris discontinuo, personalidad por cerebro, aplicación en caliente. |
| [v0.5.0](https://github.com/Sebastian197/korvun/releases/tag/v0.5.0) | **El canal webhook genérico** — POST de JSON de entrada, respuestas a tu URL; auth Bearer que falla cerrado, identidad de conversación, 503 honesto en saturación. |
| [v0.4.0](https://github.com/Sebastian197/korvun/releases/tag/v0.4.0) | **Korvun Desktop** — la pasarela tras una ventana nativa: onboarding en el primer arranque, secretos en el llavero del sistema, el builder embebido. |
| [v0.3.0](https://github.com/Sebastian197/korvun/releases/tag/v0.3.0) | **El canal de Discord** — Gateway de entrada con resume/reconexión, REST de salida, menciones bloqueadas por defecto, una familia anti-bucle completa. |
| [v0.2.0](https://github.com/Sebastian197/korvun/releases/tag/v0.2.0) | **Resiliencia + la CLI** — calentamiento de modelos locales al arrancar, tiempos por intento generosos, reintento con fallback diferenciado; `serve`, `config check`, `status`. |

Las notas de cada release se publican en GitHub — esta página es un mapa,
no un espejo.
