# OpenSSF Best Practices (bestpractices.dev) — respuestas verificadas

> Para Chano: el proyecto ya está registrado (badge **InProgress**, que
> Scorecard ya puntúa con 2). Cada «Sí» de abajo está VERIFICADO contra el
> repo con su evidencia — revisa y marca en el cuestionario; lo que esté
> como «revisar» es tu palabra, no la mía. Criterios del nivel *passing*.

## Básicos

| Criterio | Respuesta | Evidencia |
|---|---|---|
| Descripción del proyecto | Sí | `README.md` (cabecera) + https://korvun.dev |
| Web del proyecto en HTTPS | Sí | https://korvun.dev/ |
| Cómo contribuir documentado | Sí | `CONTRIBUTING.md` (método completo: TDD, gates, releases) |
| Requisitos de contribución | Sí | `CONTRIBUTING.md` §método + `CLAUDE.md` público no aplica (interno) |
| Licencia FLOSS aprobada | Sí | Apache-2.0 — `LICENSE` en la raíz |
| Licencia en ubicación estándar | Sí | `/LICENSE` |
| Canal de discusión | Sí | GitHub Issues del repo |
| Documentación en inglés | Sí | README, docs/, godoc — todo en inglés |
| Proyecto mantenido | Sí | Releases y commits continuos (v0.10.0 el 2026-08-29) |

## Control de cambios

| Criterio | Respuesta | Evidencia |
|---|---|---|
| Repositorio público con historial | Sí | github.com/Sebastian197/korvun (git) |
| Versionado único por release | Sí | Tags SemVer `vX.Y.Z` (v0.6.0…v0.10.0) |
| SemVer | Sí | `CLAUDE.md` §Commit/SemVer + los tags |
| Release notes por versión | Sí | `docs/releases/*.md` + body de cada GitHub Release (verbatim) |
| Notas identifican vulnerabilidades corregidas | Sí | p. ej. las notas de calidad v0.9.x y el commit `38b9d76` (bumps CVE con IDs) |

## Reporte de problemas

| Criterio | Respuesta | Evidencia |
|---|---|---|
| Proceso para reportar bugs | Sí | GitHub Issues + `SECURITY.md` |
| Tracker de issues | Sí | GitHub Issues |
| Proceso de reporte de vulnerabilidades | Sí | `SECURITY.md` (canal y compromiso) |
| Canal privado para vulnerabilidades | REVISAR | `SECURITY.md` — confirma que el canal privado (GitHub private reporting o email) está habilitado en Settings |
| Respuesta a reportes (≤14 días) | REVISAR | Compromiso tuyo de operación — márcalo si lo asumes |

## Calidad

| Criterio | Respuesta | Evidencia |
|---|---|---|
| Build automatizado | Sí | `Makefile` (`make build`), herramientas FLOSS comunes (go, make) |
| Suite de tests automatizada | Sí | `make test` (-race, tabla-driven) |
| Tests en CI | Sí | `.github/workflows/quality.yml` (Quality Gate en cada push) |
| Política de tests para funcionalidad nueva | Sí | TDD por ley — `CONTRIBUTING.md` (el RED aprobado es contrato) |
| Cobertura sustancial | Sí | Umbral 85 % global; ≥90 % en policy/router/envelope/brain/action (`Makefile` cover) |
| Warnings habilitados y corregidos | Sí | `golangci-lint` (govet, staticcheck, errcheck, gosec) en verde obligatorio |

## Seguridad

| Criterio | Respuesta | Evidencia |
|---|---|---|
| Conocimiento de diseño seguro | Sí | Invariantes en `CLAUDE.md`/`SECURITY.md`; modelo de amenazas del blueprint §23 |
| Criptografía: solo estándar/publicada | Sí | Sin cripto propia: cosign/Sigstore (releases), llavero del SO (secretos), stdlib crypto (digests) |
| Entrega protegida contra MITM | Sí | HTTPS + manifiestos sha256 firmados con cosign + SBOM (`docs/packaging/INSTALL.md`) |
| Sin credenciales filtradas en el repo | Sí | Ley de secretos-por-nombre (`_env`), secret scanning de GitHub activo |
| Vulnerabilidades corregidas ≤60 días | Sí | govulncheck antes de CADA ensayo (ley `CLAUDE.md`); bumps del 2026-08-30 con IDs |

## Análisis

| Criterio | Respuesta | Evidencia |
|---|---|---|
| Análisis estático | Sí | golangci-lint+gosec en cada commit (hook y CI) + CodeQL semanal y por push |
| Estático busca vulnerabilidades comunes | Sí | gosec + CodeQL security queries |
| Hallazgos estáticos corregidos | Sí | gate bloqueante — un hallazgo rompe el commit |
| Análisis dinámico | Sí | Suite entera con `-race`; fuzzing nativo del kernel (`internal/action/fuzz_test.go`, smoke en `make quality`) |
| Hallazgos dinámicos corregidos | Sí | mismo gate bloqueante |

## Lo que NO se marca (honestidad)

- Todo criterio sobre **revisión por segunda persona** (two-person review):
  mantenedor único — no se marca lo que no es verdad.
- Los dos «REVISAR» de arriba son compromisos operativos tuyos.
