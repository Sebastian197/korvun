# Pre-test adversarial review — el adaptador de aprobaciones contra el almacén real

**Tren:** la pantalla de aprobaciones (v0.15.0). **Pieza:** la costura
`controlapi.Approvals` implementada contra `internal/action/sqlite` y
`internal/app`, los moldes de FR-TEST-4/FR-TEST-5 y las curas 16-21 y 25.

**Estado:** la SUPERFICIE está escrita y en verde contra un almacén falso
(`internal/controlapi/approvals.go`, commit `91fd378`: las cuatro rutas, los 22
nombres, los 22 centinelas, el mapa de literales, los DTO y la costura
`Approvals`). **El ADAPTADOR no existe**: `ListPending` solo tiene la interfaz,
el handler y el doble del test. Lo que este papel planea es el adaptador y sus
moldes. Es el paso 10 de la ley de revisión adversarial previa y no abre rojo
por sí solo. La v4 decía «nada de esto está escrito» y era más ancho que su
cable — y era el error que hacía que G1 y R13 se escribieran contra un fichero
que ya los contradecía.

**Versión 6, CONGELADA por el director el 2026-09-12.** Cinco pasadas, cinco
VETO MANTENIDO, 57 hallazgos curados. El papel no admite más pasadas: lo que
quede se cura en el rojo. Las adjudicaciones están en el §0-sexies y mandan
sobre cualquier frase anterior de este documento. La v1 recibió VETO MANTENIDO con diecisiete
hallazgos; la v2, con nueve más, tres de ellos P1 de comportamiento — y uno de
esos tres era una cura de la v2 que **empeoraba** la garantía. El §0 y el §0-bis
los listan con su cura; el §0-ter lista los de la tercera. Nada se dobló en
silencio.

**Lo que las tres pasadas han enseñado, y el director tiene que saber:** una
parte creciente de los hallazgos ya no son defectos de este papel sino
**contradicciones de la spec madre consigo misma**. Van cuatro localizadas, y
el §0-quater las separa de lo mío porque no las puedo adjudicar yo.

**Qué se leyó:** `internal/action/sqlite/approvals.go`,
`internal/action/sqlite/store.go`, `internal/app/approvals.go`,
`internal/action/preview.go`, `internal/action/approval.go`,
`internal/action/action.go`, `internal/action/bound.go`,
`internal/controlapi/approvals.go`, y §11, §13, §13-bis y §17 F de
`2026-09-08-approvals-screen-ux.md`. **Qué se ejecutó:** nada de la pieza. Dos
comprobaciones puntuales sobre el árbol, marcadas donde aparecen.

---

## 0. Lo que la pasada 9-bis tumbó de la v1

| # | Clase | Qué estaba mal | Dónde se curó |
|---|---|---|---|
| 1 | P1 garantía | G4 decía «tras **cualquier** fallo del claim el nombre lo decide la re-lectura». Escrito así, G4 **es** la mutación que §13-bis manda enrojecer: un rehúse de cinturón dentro del claim caería en `held` en vez de `decided_evidence_corrupt`. Y contradecía a G5 dentro del mismo §1 | §1 G4, reescrita con el eje 1 primero |
| 2 | P1 comportamiento | V7 afirmaba que `ExecuteApprovedAction` rehúsa «sin centinela **en todos** sus caminos». Es **falso**: cinco de sus nueve salidas propagan con `%w` | §2 V7, con los nueve sitios repartidos |
| 3 | P1 comportamiento | el molde de «fila nacida vacía» llevaba una mutación que ataca la detección del vacío, no la re-derivación, y pasaría verde sin la rama vigilada | §6, mutación tomada de §13-bis |
| 4 | P1 comportamiento | el molde de la instantánea (G2) no tenía con qué forzar su rama: una llamada atómica no admite un commit ajeno en medio, así que su mutación no lo enrojecería | §6 nota de forzado, y §7 R2 |
| 5 | P1 | los moldes «competidor que gana el claim» y «params vaciados por un competidor» **no** son el mismo: vigilan ramas distintas y §13-bis les da mutaciones distintas. Fusionarlos borraba una mutación del plan | §6, separados |
| 6 | P1 comportamiento | el hallazgo decía que `origin` no tenía fuente. **La cura que le puse era falsa y la segunda pasada la tumbó** (hallazgo 19): la previa sí sella el canal en `Resources`. Lo que queda en pie del hallazgo original es que la previa no lleva un campo LLAMADO canal — lo lleva dentro de `Resources`, y eso no es lo mismo que no llevarlo | §2 V6, reescrita en la v3 |
| 7 | P1 garantía | G1 se enunciaba sin cable: los cinturones del almacén rehúsan con `fmt.Errorf` plano, así que hoy `invalidated` y `evidence_corrupt` solo se nombrarían por texto — lo que FR-API-15 llama «un hallazgo, no una implementación» | §2 V9 |
| 8 | P2 | la cura 18 se atribuía a `internal/app`; es íntegramente del almacén | §2 V6 y V7 |
| 9 | P2 | el molde del sweep mutaba el decorado (el sweep) en vez de la precedencia del endpoint | §6 |
| 10 | P2 | al molde «claim ajeno sin commitear» se le fabricaba una mutación; §13-bis lo declara **sin mutación por construcción** | §6 |
| 11 | P2 comportamiento | el cinturón de `ExecuteApprovedAction` juzga una terna leída **dos transacciones antes**, con la aprobación ya consumida. TOCTOU real en el único camino que produce efecto | §2 V10, §3.2 B9 |
| 12 | P2 | el pool de una conexión no solo impide un ataque: **cuelga** cualquier lectura suelta anidada en la transacción del detalle | §2 V11 |
| 13 | P2 | §4 decía «un solo transitorio» y el registro ya trae dos; y clasificaba `empty`, que no es un nombre del registro | §4 |
| 14 | P2 | R1 apuntaba a la conexión equivocada y pasaba por alto la vía barata que la propia spec ofrece | §7 R1 |
| 15 | P2 | once filas de la matriz no tenían molde y el papel no lo declaraba | §3 y §6, con la columna «molde» |
| 16 | P2 | `isBusyClass` es no exportada: no es alcanzable desde `controlapi` ni `app` | §2 V11 |
| 17 | P3 | «siete hechos» sobre ocho; «22 nombres» sobre 18 clasificados; either/or en cuatro mutaciones | corregido en todo el documento |

---

## 0-bis. Lo que la SEGUNDA pasada tumbó de la v2

| # | Clase | Qué estaba mal | Dónde se curó |
|---|---|---|---|
| 18 | **P1 comportamiento** | «el plan de rojo es §13-bis **verbatim**». §13-bis está **rancia** contra §11: su fila «fila de `actions` ausente tras el sí ⇒ `evidence_corrupt`» es anterior a la ronda 30, y §11 (eje 1) y la tabla de FR-UI-18 exigen **`decided_evidence_corrupt`** para esa misma rama, con AS-113 prohibiendo expresamente que caiga en `held`. Un rojo verbatim habría fijado el nombre contrario | §6, el plan pasa a ser §13-bis **reconciliada** con §11, con el conflicto nombrado |
| 19 | **P1 comportamiento** | V6 negaba que la previa lleve el canal. **Es falso**: `bound.go` escribe `Resources: []string{strings.TrimSpace(env.Source.Channel)}`, `Resources` entra en `ActionPreview.Digest()` y en `previewWire`, luego el canal viaja **bajo el sello** en `canonical_preview`. Sobre esa negación la v2 (a) declaraba falsa una frase de la spec, (b) **reescribía la cura 19**, autorizada en bloque, y (c) inventaba un molde. Y la dirección empeoraba la garantía: mover `origin` a `actions.source_channel` —que ningún cinturón compara— habría convertido un campo sellado en uno que una mano externa cambia sin que nada rehúse | §2 V6 reescrita; R5 y el molde de `origin` **retirados** |
| 20 | **P1 comportamiento** | §3.2 B4 apuntaba a un molde que vigila otra rama, y **ninguna** de las 38 filas de §13-bis nombra `not_started_params_gone` (verificado por `grep`: cero). El peldaño 2 del eje 2 entraba al rojo sin mutación, con el literal más categórico de la superficie | §6.1, molde nuevo |
| 21 | P2 | el molde del **error** de `RowsAffected()` no tenía vía de forzado y no se declaraba NO VERIFICABLE. §15-quater ya dice que esa clase «solo es forzable con driver envuelto», y `open()` hace `sql.Open("sqlite", …)` a pelo | §6.2 y §7 R10 |
| 22 | P2 | D3 se anunciaba fichado en §7 y no estaba en ningún riesgo | §7 R11 |
| 23 | P2 | la numeración 1-39 era **posicional**: solo existe en este papel, y §13-bis no tiene columna de número. Insertar una fila allí pudre en silencio todas las referencias de §3. Es la ley «los comentarios no llevan posiciones relativas» aplicada a un plan de rojo | §3 y §6, cada molde se nombra por su **frase**, no por su número |
| 24 | P2 | el oráculo de «una sola resolución» se prescribía sin mecanismo, en un papel que sí paró el tren para pedir el seam del molde de la instantánea | §6.2 y §7 R12 |
| 25 | P2 | G1 exigía centinela tipado para **los 22** nombres; dos de ellos son **residuales por construcción** —se emiten cuando ningún centinela nombró—, que es el inverso literal | §1 G1, acotada |
| 26 | P3 | «CINCO propagan con `%w`»: son **cuatro** con `%w` y una identidad desnuda. La tabla de nueve salidas era correcta; el encabezado mentía | §2 V7 |
| 27 | P3 | V8 contaba **tres** señales en banda; son **cuatro**, y la que faltaba es la más importante: la pérdida del one-shot | §2 V8 |
| 28 | P3 | §4 atribuía «no se sabe» a los seis de ignorancia declarada; la tabla P4 dice «**no**» para tres de ellos | §4 |
| 29 | P3 | §5 decía «tres transacciones sueltas» en el camino de ejecución; son tres **después** de las cuatro lecturas de `GetApproval` | §5 |

---

## 0-ter. Lo que la TERCERA pasada tumbó de la v3

| # | Clase | Qué estaba mal | Dónde se curó |
|---|---|---|---|
| 30 | **P1 comportamiento** | V6 concluía desde el ESCRITOR lo que solo el LECTOR garantiza. El canal sí viaja sellado, pero la lista lo lee **sin cinturón por diseño** (FR-API-1), y `ParseCanonicalPreview` asigna `Resources: w.Resources` sin cota: un `"resources": []` mutado da `len == 0`, `Resources[0]` **entra en pánico**, y no hay `recover()` en `internal/controlapi` — una fila corrupta se llevaría la LISTA ENTERA y la ventana pintaría E2 «núcleo no responde», transitorio, sobre corrupción permanente. Y `sortedSet` **ordena**: el índice 0 no es «el canal», es «el recurso alfabéticamente menor» | §2 V6, §6.1 HUECO 3 ampliado |
| 31 | **P1 garantía** | G7 ataba `params_unreadable` a la re-lectura y con eso mandaba a permanente un fallo de driver dentro del claim, que §11 y P4 declaran transitorio | §1 G7 |
| 32 | **P1 garantía** | G1 prometía centinela tipado para veinte nombres; §17 F autoriza **siete**. Y `brain_gone` no tiene centinela ni lo autoriza ninguna cura: hoy solo se nombra por texto | §1 G1, §7 R13 |
| 33 | **P1 comportamiento** | la mutación del HUECO 2 no enrojecería: atacaba la comparación de `verifyApprovalStory`, que ya ignora `op_version` — por eso el hueco existe. Es el hallazgo 3 de la primera pasada, reintroducido en un molde nuevo | §6.1 HUECO 2 |
| 34 | **P1 garantía** | el literal de `decided_evidence_corrupt` afirma «la decisión quedó registrada y sellada, con su recibo». Emitido por el GET sobre una fila **PENDING** con `requested_at` corrupto, publica un recibo que no existe. C5 y C14 necesitan el mismo desdoble por puerta que el CONFLICTO 1 | §6.1 CONFLICTO 3 |
| 35 | P2 | el molde del **error** de `RowsAffected()` no es NO VERIFICABLE: AS-104-bis lo tiene, con su costura de test declarada. La v3 citó §15-quater, que es sobre **otros tres** `RowsAffected()` que el tren no cura | §6.2, §7 R1 y R12 |
| 36 | P2 | §6.3 decía «[2conn] **solo** tres» y con eso degradaba en bloque una docena de AS que §12 etiqueta «dos conexiones reales» — y para AS-113 el segundo handle es load-bearing (`PRAGMA foreign_keys=OFF` en la conexión atacante) | §6.3 |
| 37 | P2 | **G5 no tenía ni una fila de ataque.** Es la garantía que nació de curar el hallazgo 1 de la primera pasada, y con ella quedaban siete filas de §13-bis fuera de §3, §6 y §7 | §3.5 |
| 38 | P3 | §4 citaba una celda de P4 que pertenece a `not_decided`, y colocaba `already_closed` entre las «ciertas y leídas» cuando P4 le da «no se sabe» | §4 |
| 39 | P3 | R7 publicaba 42 como «la cuenta real» incluyendo moldes declarados NO VERIFICABLES | §7 R7 |
| 40 | P3 | §5 llamaba «implícitas» a siete transacciones; el claim abre `BeginTx` y es **explícita** — y esa es justo la propiedad sobre la que descansa B1 | §5 |

---

## 0-quater. Las contradicciones de la SPEC MADRE — no las adjudico yo

Cuatro, localizadas verificando fila a fila. Ninguna es defecto de este papel y
ninguna la puedo resolver sin ti, porque las dos ramas están **escritas y
aprobadas**.

| # | Dónde | Qué dice una | Qué dice la otra | Lo que propongo |
|---|---|---|---|---|
| K1 | §13-bis «fila de `actions` ausente tras el sí» **vs** §11 eje 1 y la tabla de FR-UI-18 | `evidence_corrupt` | **`decided_evidence_corrupt`**, y AS-113 prohíbe expresamente que caiga en `held` | manda §11: la fila de §13-bis es anterior a la ronda 30. Desdoble en dos moldes, uno por puerta |
| K2 | §11 FR-API-21, la «tercera vía» del trigger, **vs** el bullet de esa misma sección que empieza «`RowsAffected() == 0` ⇒ centinela INTERNO del claim», AS-104 y §13-bis | el trigger «no cambia el desenlace exigido —**`params_unaccounted`**—» | con la fila intacta es **`not_started_params_held`**, y AS-104 dice que un literal que afirmara «ya no estaban donde estaban» sería «**literalmente falso** sobre una fila donde están» | manda AS-104. La frase del trigger está mal y hay que acotarla |
| K3 | la tabla de FR-UI-18, fila `not_started_params_held` **vs** la fila «(el claim rehúsa por sí mismo)» | «El fallo de driver tiene **un solo nombre** —`params_unreadable`— y salió de aquí en la ronda 33» | un fallo de driver dentro del claim «lo decide una RE-LECTURA … `held` si la fila conserva sus params» | manda la ronda 33. La segunda fila quedó rancia |
| K5 | **RETIRADO.** Lo fabriqué truncando la cita: AS-104-bis dice «el único molde que necesita una costura de test **en producción**», y las de AS-100 y AS-102 viven en un `_test.go` del paquete del almacén, que no toca producción. La frase de AS-104 es verdadera. Y la inversión importa: la que **sí** parece necesitar costura de producción es AS-100 (§7 R2), que es justo la que declaré resuelta | | | |
| K4 | AS-113, dentro de una línea | «el desenlace es **`evidence_corrupt`**, permanente» | «El desenlace es **`decided_evidence_corrupt`** con su literal PROPIO» | manda la segunda, por K1 |

**Ninguna de las cuatro deja viva una frase falsa PUBLICADA mientras no se
adjudiquen**, porque el rojo no está escrito. En cuanto se abra, sí: el molde
la fijaría. Por eso van antes del rojo y no después.

---

## 0-quinquies. Lo que la CUARTA pasada tumbó, y la causa raíz que destapó

| # | Clase | Qué estaba mal | Dónde se curó |
|---|---|---|---|
| 41 | **P1 comportamiento** | la mutación del HUECO 2, rehecha en la v4, **tampoco enrojecería**: re-derivar desde el `"ns/name"` de la previa hashea `strconv.Itoa(0)` como versión, así que el digest **nunca** coincide y el mutante emite `params_digest_mismatch` para toda fila, sana o atacada — el molde pasa igual. Tercera vez que este hueco rompe la regla 3 | §6.1 HUECO 2, mutación correcta |
| 42 | **P1 garantía** | G7 trazaba la frontera por PUERTA HTTP; un fallo de driver antes del commit del decide deja la fila PENDING y recibiría un literal que habla de una ejecución que nunca empezó | §1 G7, frontera en el COMMIT |
| 43 | **P1 comportamiento** | **una sola columna de tiempo corrupta se lleva la LISTA ENTERA, hoy.** `ListApprovals` hace `return nil, err` en la primera fila que `scanApproval` no puede parsear, así que un `requested_at` mutado en UNA fila borra también las sanas. Ninguna de las 38 filas de §13-bis lo cubre | §2 V12, §6.1 HUECO 4 |
| 44 | **P1 garantía** | G1 negaba `ErrApprovalDecidedEvidenceCorrupt`… buscando esa ortografía. El centinela existe, se llama `ErrApprovalDecidedEvidenceBad`, y está ligado al nombre en el mapa `outcomes`. Además G1 duplicaba `params_unreadable` en dos grupos excluyentes, llamaba «seis» a ocho, y metía los dos nombres de E6-ter entre los «desenlaces de la ejecución» cuando son de lectura | §1 G1, cuarta redacción |
| 45 | P2 | §5 llamaba implícita a `FinishWithResult`, que abre `BeginTx`. Son cinco implícitas y **dos** explícitas | §5 |
| 46 | P2 | §4 seguía publicando la frontera de transitorios que §1 declaraba refutada | §4 |
| 47 | P2 | §3.5 archivaba bajo G5 cuatro filas que se deciden **leyendo**; E8 es el contraejemplo de G5, no un ataque contra ella | §3.5, reducida a sus cuatro reales |
| 48 | P2 | HUECO 3 molde B servía una previa CORRUPTA con los mismos bytes que un canal legítimamente vacío, después de que C16 levantara «vacío ≠ ausente» | §6.1 HUECO 3 |
| 49 | P2 | «Estado: PLANNED. Nada de esto está escrito»: la superficie SÍ está escrita y commiteada | cabecera |
| 50 | P3 | R7 no descontaba las dos filas que el CONFLICTO 3 sustituye; R9 seguía diciendo «42 moldes sobre tres paquetes» cuando son cuatro | §7 |
| 51 | P3 | K2 y §6.2 localizaban una frase como «doce líneas antes»: posición relativa, que la ley prohíbe — y además la distancia era otra | §0-quater, §6.2 |

**La causa raíz, y es mía.** Leí §11, §13 y §13-bis y **no leí en prosa los AS
de §12**. Por eso el papel llevaba cuatro versiones «descubriendo» problemas
que §12 tenía resueltos y aprobados:

- **AS-100 ya declara la costura de la instantánea**, con su mecanismo
  entero: «el molde vive en el almacén, abre la transacción del detalle, hace
  su primera lectura, **espera en un canal** a que una segunda conexión
  confirme el `Commit` de su rechazo, y solo entonces sigue leyendo dentro de
  la MISMA transacción». Mi R2 pedía tu visto para algo ya adjudicado.
- **AS-102 ya declara el suyo** para el busy sobre otra aprobación, «en el
  almacén, en una puerta de test».
- **AS-104-bis ya declara el del driver envuelto.**
- **AS-96 ya fija `evidence_corrupt`** para la fila de `actions` borrada vista
  por el DETALLE, que es exactamente la mitad de mi CONFLICTO 1.

**Dos de mis siete bloqueos se disuelven con eso** (R2 y la mitad de R12), y
la pasada 2 me había hecho aceptar como cierto que el molde de la instantánea
no podía forzarse: era cierto **sin** la costura, y la costura llevaba escrita
desde §12.

---

## 0-sexies. Las adjudicaciones del director (2026-09-12) — MANDAN

Congelado el papel, el director decide. Lo que sigue es su decisión, no mi
recomendación, salvo donde dice que adopta la mía.

**Los tres defectos del árbol van LOS TRES DENTRO del tren**, ninguno fichado:
están en el camino que este tren enchufa a la pantalla y los tres rompen
garantías suyas.

| Defecto | Cómo se cura, literal |
|---|---|
| **V10** (TOCTOU) | la comparación del digest se hace contra la fila **leída DENTRO de la transacción del claim**, nunca contra una lectura anterior al commit. *Molde:* mutar `op_version` en la ventana ⇒ rehúse **nombrado**, sin ejecución |
| **V12** (la lista) | una fila con `requested_at` corrupto **no se lleva la lista**: se salta, se cuenta y se nombra — la lista vuelve con las sanas más un aviso con nombre de cuántas quedaron fuera y por qué. *Molde:* una corrupta entre tres ⇒ **dos filas + aviso**, jamás un error vacío |
| **V13** (`transitionTx`) | el rehúse por cero filas recibe **nombre del registro**. Nada se decidió y `exec.Run` no corrió, así que su nombre dice «la acción ya no estaba pendiente», **no** «no sabemos si el efecto ocurrió». Palabras del director: *mentir por exceso de cautela sigue siendo mentir* |

**Las adjudicaciones, numeradas como él las dio:**

1. **K1-K4: ADOPTADAS** las cuatro propuestas de este papel, **por
   recomendación del ejecutor**, y así quedan declaradas. El adversario sobre
   el diff las ataca como a todo lo demás. **K5 retirada**, como yo mismo
   propuse.
2. **R2 / AS-100: manda la lectura bajo la que el molde de G2 ENROJECE con su
   mutación.** Su razón, literal: *una lectura que hace al molde inmune a su
   propia mutación no es una garantía, es una tautología*. AS-100 se reescribe
   con esa lectura y sin la ambigüedad.
3. **La sonda de conteo** de la resolución única **se declara en §12 y se
   añade**: barata, y cierra la única costura sin declarar.
4. **El competidor que gana el claim y vacía la fila:** si no se puede forzar
   con dos conexiones reales y una barrera, se declara **NO VERIFICABLE**, la
   garantía se **acota a lo que sí se prueba**, y se ficha con su razón.
   **Jamás un error inyectado.**
5. **`brain_gone`: ENTRA UN CENTINELA**, uno fuera de las 25. Su razón: *un
   nombre que la pantalla pinta y el servidor nunca emite es interfaz muerta*.
6. **El fallo de re-lectura del recibo:** se mapea a un nombre **ya existente**
   de desenlace incierto **si su semántica es honesta** —el efecto pudo ocurrir
   y el registro no se cerró—; si no encaja, **nombre nuevo** en el registro con
   su texto de pantalla. Se decide con el código delante y se declara.
7. **La señal en banda de `DecideApprovalUnderLaw`: MOLDE**, salvo que exija
   rediseño; entonces fichaje **con reproducción**.
8. **Denominador: 46 moldes** (48 con los dos de adjudicación, uno no
   ejecutable), cifra del fichero. El «21» era la estimación de la spec y queda
   registrado como tal.

**Hallazgo de proceso, al canto:** leí §11, §13 y §13-bis de la spec madre y
**no leí los AS de §12 en prosa**, y por eso cuatro versiones de este papel
plantearon problemas que §12 tenía resueltos y aprobados. Va al canto del tren
como hallazgo de proceso, no como nota de color.

---

## 1. Las garantías, literales

**G1 — Ningún nombre se decide por el TEXTO de un error.** Cuarta redacción.
Las tres anteriores fallaron por el mismo sitio: prometer cable de más. Ésta se
escribe **contando sobre el fichero**, no sobre la spec, porque el fichero ya
existe.

Los 22 nombres de `ApprovalOutcomeNames` tienen los 22 su centinela tipado en
`internal/controlapi/approvals.go` — incluido
`ErrApprovalDecidedEvidenceBad`, que la v4 negaba («no hay ni habrá un
`ErrApprovalDecidedEvidenceCorrupt`») **buscando otra ortografía**: el
centinela existe y está ligado a `decided_evidence_corrupt` en el mapa
`outcomes`. Lo que G1 tiene que decir, entonces, no es «hay centinela» —lo
hay— sino **de dónde viene el valor que el adaptador mete en cada uno**:

| De dónde sale la distinción | Nombres |
|---|---|
| de un **centinela del almacén** (los siete que §17 F filas 18 y 21 autorizan) | `invalidated`, `evidence_corrupt`, `not_found`, y los cuatro de la escalera del eje 2 |
| de **leer la columna `status`** (precedencia FR-API-18) | `already_decided`, `expired`, `already_closed`, `not_decided` |
| de un **cinturón de LECTURA** en el GET | `params_digest_mismatch`, `params_not_canonical` — los dos nombres de E6-ter, que es la cara visible de FR-UI-62 |
| de la **comparación pre-decide** de FR-API-14 | `digest_mismatch` |
| del **desenlace de la ejecución** | `params_belt_failed`, `unknown_outcome`, `close_failed` |
| de la **puerta HTTP** | `forbidden`, `disabled` |
| de **si hubo decide commiteado**, sobre el mismo rehúse de evidencia | `decided_evidence_corrupt` |
| **residual declarado**: el error que ningún centinela nombra | `unavailable` |
| **SIN CABLE**: un `fmt.Errorf` plano en `internal/app`, y ninguna cura crea centinela | `brain_gone` (§7 R13) |

Suman 22 contados uno a uno, sin duplicar. La v4 sumaba 22 duplicando tres y
perdiendo dos; **la v5 dijo 22 y su tabla sumaba 20**, perdiendo precisamente
`unavailable` —el residual, que es donde caen V12 y V13— y
`decided_evidence_corrupt`, sobre el que gira G7 entero.

**Y lo que el residual significa, escrito, porque la v5 lo dejaba sin decir:**
cuando el adaptador recibe un error que ningún centinela nombra, el nombre es
`unavailable` **solo si no hubo decide commiteado**; si lo hubo, es
`params_unreadable`. Nunca es un 500 sin nombre: eso lo pinta la pantalla como
«respuesta que no reconoce», que es la degradación silenciosa que FR-UI-37
prohíbe.

**Y lo que separa `evidence_corrupt` de `decided_evidence_corrupt` NO es el
centinela ni la puerta: es si hubo un decide COMMITEADO.** Los dos tienen
centinela propio; el adaptador elige entre ellos por el estado del mundo, no
por el verbo HTTP. Ver G7 y el CONFLICTO 3.

**G2 — El detalle se sirve de UNA instantánea.** Estado, terna
`(op_namespace, op_name, op_version)`, params y cinturones se leen dentro de
una sola transacción. Promete CONSISTENCIA, no frescura.

**G3 — Una sola resolución de la ley por decisión.** El pin que juzga el
decide y la jaula que arma el ejecutor salen del MISMO objeto
(`ResolveApprovalLaw` → `BuildApprovalExecutorFromCage`, nunca
`BuildApprovalExecutor`).

**G4 — Dos ejes, y en este orden.**
*Eje 1:* si lo que rehusó fue un **cinturón o un parseo de evidencia** —corra
donde corra, en `GetApproval`, en `ClaimApprovalParams` o en `internal/app`—
el nombre lo da el **centinela**, y no se vuelve a leer la fila.
*Eje 2:* **solo cuando el eje 1 no ha nombrado** —la clase busy, el
`RowsAffected`, la fila ausente— el nombre lo decide **re-leer** la fila.
Inferirlo de «nuestra transacción rodó atrás» es falso en al menos dos ramas
verificadas.

**G5 — Lo que un centinela nombra, no se lee.** Un desenlace del eje 1 se
emite sin volver a mirar la fila, así que su literal **no puede afirmar
estado**: ni «conserva sus parámetros» ni «ninguna pasada la cerrará».

**G6 — `brains_can_park` es una COTA SUPERIOR declarada.** Se computa sobre
los canales configurados, sale de la MISMA resolución que cablea las
identidades, y el literal dice su alcance.

**G8 — Una fila ilegible ni se lleva la lista ni desaparece de ella.** Nueva
tras la quinta pasada: V12, V13, el HUECO 4, A10, C11 y C16 son todos de la
LISTA, y §1 no enunciaba ninguna garantía sobre ella — un molde sin garantía
nombrada rompe el punto 1 de la doctrina. La lista sirve todas las filas que
puede servir; una que no se pueda leer entera sale con lo que sí se leyó —su
identificador y su digest— marcada, y nunca se lleva a las sanas por delante.

**G7 — Hay DOS transitorios y su frontera está en el COMMIT del decide.**
Tercera redacción. La v3 la ató a la re-lectura y mandaba a permanente un fallo
de driver dentro del claim; la v4 la ató a la PUERTA HTTP y con eso publicaba
un literal post-decisión sobre un POST que no había decidido nada — un fallo de
driver en la comparación de FR-API-14, o en cualquiera de las cuatro lecturas
que `decideApprovalWithLaw` hace antes de escribir, deja la fila **PENDING e
intacta** por su `defer tx.Rollback()`, y el literal de `params_unreadable`
diría ahí «no puede afirmar que la petición no se haya ejecutado», que es un
desconocimiento inventado.

La frontera correcta, y es la misma que el CONFLICTO 3 usa:

- **antes del commit del decide** —lea quien lea, GET o POST— un fallo de
  driver es `unavailable`: no se decidió nada, reinténtalo;
- **después del commit del decide**, un fallo de driver en el camino de
  ejecución —la lectura inicial o la re-lectura del eje 2— es
  `params_unreadable`: la decisión existe y lo que no se sabe es el efecto;
- **el camino del RECHAZO** tiene su propia frontera, que la v5 dejó fuera:
  después del commit no hay ejecución ninguna, así que un fallo de la
  re-lectura del recibo que la cura 25 obliga —`decision_receipt_id`— **no**
  puede llevar el literal de `params_unreadable`, que habla de una ejecución
  que no arrancó. Necesita su propio nombre o su propia frase; **ninguno de los
  22 lo tiene** (§7 R14);
- todo lo demás, y en particular toda AUSENCIA de fila, falla cerrado, **con
  el residual del G1 como suelo: nunca un 500 sin nombre**.


---

## 2. Lo que el árbol hace HOY — verificado contra el fichero

Once hechos. Cada uno con su ancla; ninguno ejecutado salvo donde se dice.

**V1 — `GetApproval` lee CUATRO veces sueltas sobre `s.db`.** La fila de
`approvals`, el `SELECT canonical_preview`, y las dos consultas de
`verifyApprovalStory` (`actions` y `action_decisions`), cada una en su
transacción implícita. **Precisión que la v1 no hizo:** `GetApproval` **no
lee `canonical_params`**, así que el E6-ter sobre un rechazo legítimo aparece
cuando se añada el cinturón de FR-API-19 como quinta lectura — no lo produce
el tejido de cuatro que hay hoy. La cura 20 es igualmente necesaria, y lo es
**antes** de añadir esa quinta. Cura 20.

**V2 — Tres sitios colapsan `sql.ErrNoRows` con el error de driver.** Las dos
consultas de `verifyApprovalStory` y el `SELECT canonical_preview` de
`GetApproval`. Una fila de `actions` DESTRUIDA y un disco que no responde
salen por el mismo `return`. Es la mitad «residual» de la cura 21.

**V3 — `ClaimApprovalParams` devuelve `ErrNotFound` en TRES ramas.** Fila
ausente, columna vacía, y `RowsAffected()==0`. Cura 16.

**V4 — `ClaimApprovalParams` descarta el error de `RowsAffected()`.** El
literal es `if n, _ := res.RowsAffected(); n == 0`. Clase (c) de la RULE 2.
Cura 17.

**V5 — `ApprovalParams` tiene el mismo colapso con dos ramas.** La otra mitad
de la cura 16.

**V6 — `ListApprovals` devuelve `[]action.Approval`, y la previa SÍ basta para
completar la fila.** Reescrita tras el hallazgo 19: la v2 afirmaba lo
contrario y era **falso**. El canal viaja en la previa, bajo el sello:
`internal/action/bound.go` escribe
`Resources: []string{strings.TrimSpace(env.Source.Channel)}`; `Resources`
entra en `ActionPreview.Digest()` (la clave `"resources"`) y en `previewWire`
(`json:"resources"`), y `canonical_preview` guarda esa vista, que
`ValidatePreviewBinding` compara contra `a.PreviewDigest`. `bound.go` es el
único sitio de producción que rellena `Resources` (verificado por `grep` sobre
`internal/`, excluidos los tests). Luego la puerta de LISTA de la cura 19 —«la
previa **sin** cinturón», como el director la autorizó— entrega los tres
campos: operación y clase de `ActionPreview`, canal de `Resources[0]`. **No
hace falta JOIN en la lista, y meterlo sería un retroceso**: `source_channel`
no lo compara ningún cinturón —`verifyApprovalStory` compara `effect_class`,
`op_namespace+"/"+op_name` y `principal_id`, nada más—, así que servir
`origin` desde esa columna cambiaría un campo sellado por uno que una mano
externa mueve sin que nada rehúse.

**Pero la conclusión de la v3 iba demasiado lejos, y la tercera pasada la
tumbó (hallazgo 30).** Que `bound.go` sea el único escritor es una propiedad
del NACIMIENTO. El campo que la lista lee no nace: se **parsea**, y la lista
corre **sin cinturón por diseño** (FR-API-1: «La lista no corre el cinturón de
verificación»). `ParseCanonicalPreview` asigna `Resources: w.Resources` sin
comprobar longitud, y `DisallowUnknownFields()` rehúsa campos de MÁS, nunca de
menos: un `"resources": []`, un `null` o la clave ausente dan `len == 0`. Dos
consecuencias, las dos graves:

- **`Resources[0]` es un índice sin guarda sobre bytes que una mano externa
  escribe.** No hay `recover()` en `internal/controlapi` (verificado por
  `grep`), así que el de `net/http` cerraría la conexión sin respuesta y la
  ventana pintaría **E2 «núcleo no responde»** —transitorio— sobre corrupción
  permanente y determinista. Y una fila corrupta se llevaría **la lista
  entera**, que es el inverso exacto de «una fila cuyo preview no se pueda
  LEER no desaparece».
- **`sortedSet` ORDENA.** `Resources[0]` no es «el canal»: es «el recurso
  alfabéticamente menor». Coincide hoy solo porque `len == 1`, y nada lo fija
  —el godoc del campo dice «the coarse resources the operation touches», en
  plural—. Es la clase (g) de la RULE 2: guarda **por posición** donde tiene
  que ser por nombre o por tipo.

**Lo que el adaptador hace, entonces:** sirve `origin` desde la previa
**solo cuando `len(Resources) == 1`**, y en cualquier otro caso lo sirve
vacío. Nunca indexa a ciegas. Sigue sin hacer falta JOIN.

Dos cosas más que quedan abiertas, y son nuevas:

- **`origin` vacío no es `origin` ausente.** `sortedSet` solo ordena, no
  descarta, así que un envelope con `Source.Channel == ""` produce
  `Resources == [""]` y la fila sale con canal vacío. Es la clase (a) de la
  RULE 2 y **ningún molde de §13-bis la nombra**. Va a §6.1.
- **Las dos copias del canal no son byte-idénticas.** La previa guarda
  `strings.TrimSpace(env.Source.Channel)`; `createApprovalParts` escribe
  `env.Source.Channel` **sin recortar** en `actions.source_channel`. Para un
  canal con espacios difieren, y ningún cinturón las cruza. No lo cura este
  tren; se ficha en §7.

**V7 — `ExecuteApprovedAction` reparte sus nueve salidas: CINCO conservan
identidad —cuatro con `%w` y una desnuda—, CUATRO son cadena pura.** La v1
decía «todos como cadena» y era falso; la v2 escribió «cinco con `%w`» y
tampoco: la de `GetApproval` no envuelve, devuelve el error tal cual.

| Salida | Forma |
|---|---|
| `GetApproval` | `return "", err` — identidad **completa**, ni envoltura |
| status ≠ APPROVED | cadena |
| `ValidateApprovalBinding` | cadena |
| `store.Get` | `%w` |
| `rec.State` ≠ Approved | cadena |
| `ClaimApprovalParams` | `%w` |
| cinturón de los params | cadena |
| `FinishWithResult` | `%w` |
| `execErr` | `%w` |

**Y el hecho relevante que la v1 escondía:** el `ErrNotFound` que HOY viaja con
identidad por dos de esos `%w` es precisamente el **colapso de tres ramas** de
V3. Un adaptador que se apoye en `errors.Is(err, sqlite.ErrNotFound)` nombra
por un centinela que miente. La cura 16 es condición previa de la 18, no
paralela.

**V8 — `DecideApprovalUnderLaw` señala EN BANDA, por CUATRO salidas.**
Corregido tras el hallazgo 27; la v2 contaba tres. `return rule, nil` cuando
`ApprovalConsumableAt` dice «ya decidida»; `return RuleApprovalAlreadyDecided,
nil` cuando se pierde la carrera del cierre por caducidad; `return
RuleApprovalExpired, nil` cuando el toque la cierra; y `return
RuleApprovalAlreadyDecided, nil` cuando el `RowsAffected()` del one-shot da
cero — **esta última es la pérdida del one-shot, la más importante y la que
faltaba**. `err == nil` no significa «decidida». No está en las 25 curas; es
riesgo de diseño del adaptador y va a §7 R6.

**V9 — Los cinturones del almacén rehúsan con `fmt.Errorf` PLANO.** Nuevo tras
el hallazgo 7, y es el cable que le faltaba a G1. Sin centinela tipado hoy:
el rehúse por ley movida en el decide y en el claim (el nombre de la regla
entra **interpolado como texto**); los cinco mismatches de
`verifyApprovalStory` (`preview_effect_`, `preview_operation_`,
`preview_principal_`, `decision_outcome_`, `decision_policy_`); los de
`ValidatePreviewBinding` en `internal/action/approval.go`. **Consecuencia:**
`invalidated` y `evidence_corrupt` solo se pueden separar hoy por
`strings.Contains`. Es la mitad «centinelas tipados POR CINTURÓN» de la cura
21, y es la que manda.

**V10 — El camino de ejecución juzga una terna RANCIA con la aprobación ya
consumida.** Nuevo tras el hallazgo 11. `internal/app/approvals.go`:
`store.Get` lee el sobre en su transacción implícita; **después**
`ClaimApprovalParams` commitea (la aprobación se consume y la columna se
vacía); **después** se compara `action.Digest(rec.Envelope.Operation,
params)` contra `approval.ActionDigest`. Un `UPDATE actions SET op_version`
ajeno entre la primera y la segunda hace que la comparación use la terna
vieja, **pase**, y se ejecute un efecto irreversible bajo una operación que la
fila ya no declara. Clase (b) de la RULE 2: comparación sobre un valor
RECOMPUTADO habiendo uno almacenado. La cura 20 cubre **solo el detalle**; el
camino de ejecución se queda con sus tres transacciones sueltas.

**V11 — El pool es de UNA conexión, y eso corta en los dos sentidos.**
`db.SetMaxOpenConns(1)`, WAL, `busy_timeout(5000)` en el DSN de paquete.
Hacia fuera: «dos conexiones reales» exige dos `*sql.DB`. Hacia dentro:
**cualquier lectura suelta de `s.db` anidada en la transacción del detalle no
compite, se cuelga hasta el deadline** — y `verifyApprovalStory` se invoca hoy
con `s.db` desde `GetApproval` y con `tx` desde el decide, la misma función
con dos receptores. Un `context deadline exceeded` no es corrupción, así que
caería en el residual transitorio y el operador leería «reintenta» sobre un
interbloqueo determinista nuestro. **Y `isBusyClass` es NO EXPORTADA**
(`internal/action/sqlite/store.go`), así que el diagnóstico técnico del eje 2
no la puede llamar desde `controlapi` ni desde `app`: o se exporta —fuera de
las 25— o nace una **tercera** copia del guard por texto.

**V12 — Una sola columna de tiempo corrupta se lleva la LISTA ENTERA.** Nuevo
tras el hallazgo 43, y es el defecto más barato de explotar de todo el papel.
`ListApprovals` escanea fila a fila y hace `return nil, err` en cuanto
`scanApproval` falla; `scanApproval` hace `time.Parse` sobre `requested_at`, y
`parseNullTime` lo hace sobre `expires_at` y `decision_at` cuando no están
vacíos. Un `UPDATE approvals SET requested_at = 'ayer'` sobre **una** fila
borra de la respuesta **también las sanas**. El adaptador recibe un error sin
centinela: o lo manda a `unavailable` —«es transitorio y no dice nada sobre la
evidencia», sobre corrupción permanente y determinista— o cae al 500 sin
nombre, que E9 pinta como «respuesta que esta pantalla no reconoce». Las dos
salidas son la clase histórica de esta casa: corrupción disfrazada de ausencia
o de transitorio. **Ninguna de las 38 filas de §13-bis lo cubre**, y la única
que toca la lista —«previa ilegible ⇒ fila sin clase»— es otro campo.

**V13 — El decide tiene un rehúse alcanzable que NINGUNO de los 22 nombres
cubre.** Nuevo tras la quinta pasada, y es el tercer defecto real del árbol que
este papel destapa. `transitionTx` devuelve `fmt.Errorf("… row not in expected
state")` —plano, sin centinela— cuando su `UPDATE actions SET state = ? WHERE
action_id = ? AND state = ?` afecta cero filas, y el decide lo llama **después**
de que el one-shot sobre `approvals` haya ganado. `verifyApprovalStory` **no
lee `state`** (su SELECT trae `effect_class, op_namespace, op_name,
principal_id`), así que nada lo caza antes, y la columna no tiene `CHECK`.

*Reproducción:* aparcar; desde una segunda conexión `UPDATE actions SET state
= 'SUCCEEDED' WHERE action_id = ?`; `POST …/approve` con el digest correcto.
Todos los cinturones pasan, el acto de decisión se inserta, el one-shot gana, y
`transitionTx` rehúsa: la transacción rueda atrás **entera**, la aprobación
sigue `PENDING` y `exec.Run` no se llamó nunca. El adaptador recibe un error
sin centinela. Hoy saldría por el 500 sin nombre, que la pantalla pinta como
desenlace desconocido —«**No sabemos si el efecto llegó a ocurrir**»— sobre una
transacción que no hizo absolutamente nada; o por `unavailable`, cuyo literal
dice «es transitorio» sobre una condición determinista creada por una mano
externa. Las dos son la clase histórica de esta casa. **Ni §3, ni §13-bis, ni
§12 tienen fila para el `state` de `actions` mutado antes del commit del
decide.** Y `transitionTx` descarta además el error de `RowsAffected()`, que es
uno de los tres que §15-quater ficha sin molde.

> **Comprobación ejecutada (la única de este papel):** `Open` y
> `OpenOperator` pasan las dos por el mismo `open(path)`, luego comparten el
> DSN y su `busy_timeout(5000)`. No hay pomo corto por la puerta pública.

---

## 3. Matriz de ataque

Cada fila lleva su molde **nombrado por su frase**, no por un número. La v2
numeraba 1-39 por la POSICIÓN de la fila en la tabla de §13-bis, que no tiene
columna de número: insertar una fila allí pudría en silencio todas las
referencias de aquí. La frase sobrevive a su propia edición.

### 3.1 Contra G2 (la instantánea del detalle)

| # | Ataque | Desenlace exigido | Molde de §13-bis |
|---|---|---|---|
| A1 | rechazo de la CLI confirmado **entre la primera y la segunda lectura** | documento coherente; **nunca** E6-ter | «rechazo de la CLI confirmado entre la primera y la segunda lectura» |
| A2 | fila de `actions` ausente, vista por el **cinturón de la lectura** | `evidence_corrupt` | «historia mutada ⇒ `evidence_corrupt`», ampliado a la ausencia (ver §6.1) |
| A3 | `op_version` mutada (ningún cinturón la compara hoy) | `params_digest_mismatch` | **NUEVO** (ver §6.1): el molde de params muta otra columna |
| A4 | params mutados por otra conexión antes de pintar | `params_digest_mismatch` **antes** de clasificar | «params mutados y leídos en el detalle» |
| A5 | fila decidida entre la lista y el detalle | `already_decided`, por precedencia de estado | «decidida entre lista y detalle» |
| A6 | `status` fuera del juego (la columna no tiene `CHECK`) | `already_decided` (GET) / `already_closed` (POST) | «`status` desconocido» |

Las cuatro filas que la v5 metió aquí **no atacan G2**, que es del DETALLE, y
se mudan: `not_decided` y las dos de `recovery_marker` son del camino de
ejecución y van a §3.2 como B11, B12 y B13; la de la lista va a §3.6, que es de
G8. Es el mismo defecto de archivo que la v4 cometió con G5, con otra sección
de destino.

### 3.6 Contra G8 (la lista)

| # | Ataque | Desenlace exigido | Molde |
|---|---|---|---|
| F1 | una columna de tiempo corrupta en UNA fila | **las dos salen**; la corrupta con su id, su digest y su cartel (V12) | **NUEVO**, HUECO 4 |
| F2 | previa con `"resources": []` | no hay pánico, no cae la lista, la fila sale marcada | **NUEVO**, HUECO 3 B |
| F3 | previa ilegible | fila **sin clase**, nunca ausente | «previa ilegible» |
| F4 | canal legítimamente vacío | `origin` vacío, y vacío ≠ ausente ≠ corrupto | **NUEVO**, HUECO 3 A |

### 3.2 Contra G4 (los dos ejes)

| # | Ataque | Desenlace exigido | Molde de §13-bis |
|---|---|---|---|
| B1 | competidor que vacía la fila y gana el upgrade | `params_unaccounted`, **no** `gone` | «competidor que gana el claim y vacía la fila» |
| B2 | busy por commit ajeno con la fila **intacta** | `not_started_params_held` | «busy por un commit ajeno con la fila intacta» |
| B3 | claim ajeno abierto y **sin commitear** | params intactos ⇒ `held`; ningún literal promete exclusividad | «claim ajeno sin commitear» — **sin mutación por construcción** |
| B4 | fila presente, columna vacía, `action.Digest(op,"")` **sí** re-deriva | `not_started_params_gone` | **NUEVO** (ver §6.1): ninguna de las 38 filas nombra este desenlace |
| B5 | fila presente, columna vacía, `action.Digest(op,"")` **no** re-deriva | `params_unaccounted` | «params vaciados por un competidor» |
| B6 | la re-lectura misma falla por driver | `params_unreadable` | «re-lectura fallida» |
| B7 | params mutados entre el commit del decide y el claim | `params_belt_failed`, aprobación consumida y fila vaciada | «params mutados entre commit y claim» |
| B8 | `status` fuera de `APPROVED` al entrar | `already_closed`, cortando **antes** de la escalera | «cortes previos al claim con estados terminales» |
| B9 | `op_version` mutada entre `store.Get` y el claim (V10) | **SIN MOLDE — riesgo abierto, §7 R4** | — |
| B10 | claim saltado por trigger, params **intactos** | `not_started_params_held` — **no** `params_unaccounted`, ver K2 | «claim saltado por trigger con params intactos» |
| B11 | aprobación `PENDING` al llegar el POST | `not_decided` | «aprobación `PENDING`» |
| B12 | acción cerrada con `recovery_marker` | el literal lo dice | «acción cerrada con `recovery_marker`» |
| B13 | nacida vacía y cerrada por la recuperación | sin frase de efecto | «aprobación nacida vacía cerrada por la recuperación» |
| B14 | `actions.state` mutado antes del commit del decide (V13) | **no se decidió nada**; jamás «no sabemos si el efecto ocurrió» | **NUEVO**, HUECO 5 |

### 3.3 Contra G1 y G7 (la taxonomía)

| # | Ataque | Desenlace exigido | Molde de §13-bis |
|---|---|---|---|
| C1 | fila barrida a `EXPIRED` | `expired` | «fila barrida a `EXPIRED`» |
| C2 | pin de ley movido, al leer **y** al tocar | `invalidated`, con `current_law_digest` en campo propio | «pin de ley movido» + «`current_law_digest` en el cuerpo» |
| C3 | historia mutada | `evidence_corrupt` | «historia mutada» |
| C4 | `ValidateApprovalBinding` rehusando **desde el endpoint** | `decided_evidence_corrupt` (cura 24) | «`policy_digest` mutado tras el commit» |
| C5 | previa impaseable (`DisallowUnknownFields`) | `decided_evidence_corrupt`, con el parseo **crudo** | «previa impaseable» |
| C6 | `FinishWithResult` falla tras `exec.Run` OK | `close_failed`, **no** `unknown_outcome` | «`FinishWithResult` forzado a fallar» |
| C7 | digest rancio en el approve | `digest_mismatch` **sin consumir** | «digest rancio» |
| C8 | fila nacida sin argumentos | `parameters_state: empty` en un **200** | «fila nacida vacía» |
| C9 | bytes no canónicos que canonizan igual | el detalle rehúsa | «bytes guardados no canónicos» |
| C10 | crudo ≤ 64 KiB, canónico > | `too_large` | «valor JSON suelto cuyo canónico pasa de 64 KiB» |
| C11 | preview ilegible | fila en la lista **sin clase** | «previa ilegible» |
| C12 | cerebro ausente del perfil | `brain_gone` **en el 200** | «cerebro ausente» |
| C13 | fallo del sellador al cerrar un rechazo | rollback, sigue `PENDING` | «fallo del sellador al cerrar un rechazo» |
| C14 | `requested_at` corrupto | `decided_evidence_corrupt` | «`requested_at` corrupto» |
| C15 | fila de `actions` borrada entre `GetApproval` y `store.Get` | **`decided_evidence_corrupt`** — AS-113 prohíbe que caiga en `held` | **EN CONFLICTO**, ver §6.1 |
| C16 | `origin` vacío en la previa | la fila sale con canal vacío, y vacío ≠ ausente | **NUEVO**, ver §6.1 |

### 3.5 Contra G5 (lo que un centinela nombra, no se lee)

Reducida tras el hallazgo 47. La v4 barrió aquí las siete filas huérfanas de
§13-bis sin mirar qué garantía ataca cada una, y cuatro de ellas se deciden
**leyendo**: `not_decided` sale del corte 1 leyendo `status`, las dos de
`recovery_marker` salen de lo que `store.Get` trajo, y «claim saltado por
trigger ⇒ `held`» lo decide **la re-lectura del eje 2** y su literal SÍ afirma
estado. Esa última es el contraejemplo de G5, no un ataque contra ella: va a
§3.2 como B10. Los ataques reales contra G5 son cuatro.

| # | Ataque | Desenlace exigido | Molde de §13-bis |
|---|---|---|---|
| E1 | un literal nombrado por centinela afirma estado de la fila | prohibido: el literal solo dice lo que su cinturón probó | «el literal nombrado por centinela no afirma estado» |
| E2 | fila borrada tras la decisión, params normales | `params_unaccounted` | «fila borrada tras la decisión, params normales» |
| E3 | fila borrada sobre una aprobación nacida vacía | `params_unaccounted` | «fila borrada sobre una aprobación NACIDA VACÍA» |
| E4 | fila de decisión borrada o previa mutada tras el sí | `decided_evidence_corrupt` | «fila de decisión borrada o previa mutada tras el sí» |

Las otras cuatro se reubican: `not_decided` y las dos de `recovery_marker` van
a §3.1 como lecturas de estado (A7, A8, A9); el trigger va a §3.2 como B10, con
`not_started_params_held` y el CONFLICTO K2 anotado.

### 3.4 Contra G3 y G6

| # | Ataque | Desenlace exigido | Molde de §13-bis |
|---|---|---|---|
| D1 | resolver la ley dos veces / usar `BuildApprovalExecutor` | el molde enrojece | «una sola resolución de la ley» — su oráculo, en §6.2 |
| D2 | cuatro condiciones cumplidas y falla la quinta | `brains_can_park` = 0 | «`brains_can_park` = 0 por fallar la quinta» |
| D3 | techo `write_irreversible` con única herramienta `critical` | no cuenta: `effect_ceiling` | **SIN MOLDE — §7 R11** |

---

## 4. Taxonomía de fallo esperada

Los **22** nombres de `ApprovalOutcomeNames`, verificados uno a uno contra el
fichero. `empty` **no está** entre ellos: es un valor de `parameters_state` en
un 200 (§17 F fila 7).

- **Permanentes por corrupción de evidencia (5):** `evidence_corrupt`,
  `decided_evidence_corrupt`, `invalidated`, `params_digest_mismatch`,
  `params_not_canonical`.
- **De estado, ciertas y leídas (7):** `already_decided`, `expired`,
  `not_decided`, `not_found`, `digest_mismatch`, `brain_gone`, y
  `already_closed` **con una salvedad**: su corte leyó el estado, pero P4 le da
  «no se sabe» en la columna del efecto, porque lo que leyó no dice qué hizo
  otro ejecutor. Ciertas sobre lo que leyeron, no sobre el mundo.
- **De ignorancia declarada (6):** `not_started_params_held`,
  `not_started_params_gone`, `params_unaccounted`, `unknown_outcome`,
  `close_failed`, `params_belt_failed`. **Su columna «¿ocurrió el efecto?» NO
  es la misma para los seis** —corregido tras el hallazgo 28; la v2 les
  atribuía «no se sabe» a todos—: la tabla P4 de la spec dice **«no, y esta
  ejecución no ha hecho nada»** para `not_started_params_held`,
  `not_started_params_gone` y `params_belt_failed`, y **«no se sabe»** solo
  para `params_unaccounted`, `unknown_outcome` y `close_failed`. La
  distinción es el contrato: los tres primeros leyeron algo; los tres
  segundos no. **La cita exacta de P4**, corregida tras el hallazgo 38: las
  celdas son «esta ejecución, **no**» para `not_started_params_held` y
  `not_started_params_gone`, y «**no**» para `params_belt_failed`; la frase
  «no, y esta ejecución no ha hecho nada» es la de `not_decided`, que va en
  otro grupo.
- **Transitorios (2), y su frontera está en el COMMIT del decide**, como dice
  G1/G7 y no como decía esta sección hasta la v4: `unavailable` antes del
  commit —lea quien lea—, `params_unreadable` después, en el camino de
  ejecución. Ningún otro nombre dice «reintenta».
- **De puerta (2):** `forbidden`, `disabled`.

---

## 5. Fronteras de transacción y de persistencia

- **Pool de UNA conexión**, WAL, `busy_timeout(5000)` en el DSN de paquete.
  Dos conexiones reales = dos `*sql.DB`. Y ninguna lectura suelta puede
  anidarse en la transacción del detalle: se cuelga (V11).
- **`BeginTx(ctx, nil)` es DEFERRED.** El claim lee y luego escribe: ese
  upgrade es el que puede perderse. Es la rama de B1.
- **El decide vacía `canonical_params` en su misma transacción** para
  `REJECTED`, `CANCELLED` y `EXPIRED`. Por eso B8 corta antes de la escalera.
- **La recuperación cierra al arrancar** las aprobadas cuya columna quedó
  vacía. Por eso ningún literal puede prometer «ninguna pasada la cerrará».
- **El camino de ejecución son SIETE transacciones**: **cinco implícitas** —las
  cuatro lecturas sueltas de `GetApproval` (V1) y `store.Get`— y **DOS
  explícitas**, el claim y `FinishWithResult`, que abren las dos
  `BeginTx(ctx, nil)`. Que el claim sea explícita y DEFERRED es la propiedad
  sobre la que descansa B1; que `FinishWithResult` lo sea es la que sostiene
  su rollback. El cinturón de los params cae entre el claim y el cierre, y
  juzga la terna que leyó `store.Get`. Es V10. La v4 contaba seis y una.

---

## 6. Las reproducciones planeadas

**El plan de rojo NO es §13-bis verbatim.** La v2 lo declaraba así y era el
peor de sus defectos: **§13-bis está parcialmente rancia contra §11**. El plan
es §13-bis **reconciliada** con §11, y los conflictos se nombran aquí en vez
de resolverse en silencio.

**La cuenta.** §13-bis tiene **38 filas de mutación**, contadas por ejecución
(`grep` sobre el bloque §13-bis → §14, menos cabeceras y separadores: 38), en
tres bloques de 23, 12 y 3. FR-TEST-4 dice «veintiún moldes» y enumera en
prosa: no son la misma lista. A las 38 hay que **restar** una en conflicto y
**sumar** las que §6.1 añade.

**Las mutaciones se toman de §13-bis salvo donde este papel declara conflicto.**
La v1 reescribió cuatro y tres no habrían enrojecido; la v2 juró verbatim y
eso arrastraba la fila rancia. Ninguna de las dos.

### 6.1 Los conflictos y los huecos — lo que hay que decidir ANTES del rojo

**CONFLICTO 1 — la fila de `actions` ausente tras el sí.**
§13-bis dice: «⇒ `evidence_corrupt`, mutación: dejarla caer en `held`».
§11 (eje 1) dice: «fila de `actions` ausente — por el cinturón **o** por el
`store.Get` posterior ⇒ **`decided_evidence_corrupt`**». Y la tabla de
FR-UI-18 lo repite con su porqué: «sin ella caía en `not_started_params_held`
y ofrecía `execute`, que es justo lo que **AS-113 declara prohibido**».
**Adjudico que manda §11**: `decided_evidence_corrupt` nace en la ronda 30 y
la fila de §13-bis es anterior. La consecuencia es que hacen falta **DOS**
moldes donde §13-bis pone uno:
- en la **puerta de lectura** (el detalle), la fila ausente ⇒ `evidence_corrupt`;
- en el **camino de ejecución**, borrada entre `GetApproval` y `store.Get` ⇒
  `decided_evidence_corrupt`, y su mutación es la de §13-bis: dejarla caer en
  `held`. **Ésta es la que la v2 perdió entera.**

**HUECO 1 — `not_started_params_gone` no tiene molde DE SERVIDOR.** Verificado:
**cero** de las 38 filas de §13-bis nombra ese desenlace. Sí tiene el de
renderizador, AS-95, con su mutación; la v5 decía «no tiene molde» a secas y
era más ancho que su cable. Es el peldaño 2 del eje 2, y su literal
es de los más categóricos de la superficie —«the row was born without
parameters, so no execution could have started»—: una afirmación de que un
efecto irreversible **no ocurrió**. *Molde nuevo:* nacer con `rawParams=""`,
aprobar, el claim ve la columna vacía; la re-lectura ve fila presente, columna
vacía y `Digest(op,"")` re-derivando ⇒ `not_started_params_gone`. *Mutación:*
devolver `params_unaccounted` para toda columna vacía sin comprobar la
re-derivación.

**HUECO 2 — `op_version` mutada no tiene molde.** El molde de §13-bis muta los
**params**, no la terna. *Molde nuevo:* `UPDATE actions SET op_version` sobre
una fila viva ⇒ `params_digest_mismatch` en el detalle.

*Mutación, tercera redacción.* Las dos anteriores no habrían enrojecido y la
razón es la misma las dos veces: atacaban algo que no cambia el resultado del
molde. La v3 neutralizaba la comparación de `verifyApprovalStory`, que **ya
ignora** `op_version` — por eso el hueco existe. La v4 re-derivaba desde el
`"ns/name"` de la previa, que no guarda versión, así que el mutante hashea
`strconv.Itoa(0)`, **nunca** coincide con ningún `action_digest`, y emite
`params_digest_mismatch` para toda fila: el molde, que afirma ese nombre,
pasaría igual.

La mutación que sí enrojece: **leer `ns` y `name` de la fila `actions` y FIJAR
la versión a la constante de producción**. Sobre la fila atacada,
`Digest({ns, name, 1}, params)` vuelve a igualar el `action_digest` sellado, no
hay mismatch, y el molde —que lo exige— se pone rojo. Sobre una fila sana no
cambia nada, que es como tiene que ser.

**HUECO 3 — `origin`: el vacío del STRING y el vacío del SLICE.** La v3 vio el
primero y no el segundo, que es el que revienta (hallazgo 30).
*Molde nuevo A:* un envelope sin canal produce `Resources == [""]`; la fila
sale con canal vacío, y vacío **nunca** se sirve como ausente ni como un valor
inventado. *Mutación:* sustituirlo por `RequestedFrom` cuando esté vacío.
*Molde nuevo B:* una previa mutada con `"resources": []` —o sin la clave— **no
entra en pánico, no tumba la lista y no esconde su fila**. Y **no sale con los
mismos bytes que el molde A**: `len != 1` es detectable sin cinturón, así que
la fila sale marcada como previa no íntegra, no como canal vacío. La v4 las
colapsaba a las dos en «origin vacío» veinte líneas después de levantar «vacío
≠ ausente» en C16. *Mutación:* indexar `Resources[0]` sin comprobar la
longitud.

**HUECO 4 — la lista entera no tiene molde contra V12.** *Molde nuevo:* dos
filas `PENDING` sanas, una de ellas con `requested_at` corrupto ⇒ **las dos
salen**, y la corrupta sale con su identificador, su digest y su cartel de no
íntegra. **Un solo desenlace, sin either/or**: la v5 escribió «sale marcada o
no sale» y eso era la clase (i) de la RULE 2 en un molde que este mismo papel
había corregido ya en otros cuatro. Además la rama permisiva era la
equivocada: `approval_id` y `action_digest` se escanean **antes** del
`time.Parse` que falla, así que la fila **es servible**, y FR-API-1 ya dice que
una fila que no se puede leer «no desaparece». *Mutación:* `return nil, err`
en el escaneo, que es lo que el árbol hace hoy.

**HUECO 5 — el rehúse de `transitionTx` (V13) no tiene nombre ni molde.**
*Molde nuevo:* `actions.state` mutado desde fuera antes del commit del decide ⇒
la transacción rueda atrás entera y el desenlace **dice que no se decidió
nada**, nunca «no sabemos si el efecto ocurrió». *Mutación:* dejarlo caer al
500 sin nombre, que es lo que pasa hoy.



**HUECO 6 — `decided_evidence_corrupt` emitido por el GET mentiría.** Lo llamé
CONFLICTO y no lo es: §12 ya acota las dos mutaciones al post-commit —AS-116
empieza «Tras el decide commiteado» y AS-117 dice que si llega antes «el
desenlace es otro, **sin nombre en esta taxonomía**»—, y las filas de §13-bis
son el resumen de esas AS, no una regla aparte. Es la misma causa raíz del
§0-quinquies, aplicada a R2 y a R12 y no aplicada aquí. Lo que queda es un
hueco real, y AS-117 lo nombra ella misma: **no hay nombre** para el GET sobre
una fila PENDING con evidencia corrupta. Su
literal dice «La decisión quedó registrada y sellada, con su recibo»
(`approvalsText.ts`, y P4 lo repite). Pero `ParseCanonicalPreview` y el
`time.Parse` de `scanApproval` corren **también en `GetApproval`**, así que
§13-bis mandaría ese nombre —y ese literal— sobre una fila **PENDING** jamás
decidida. **Adjudico el mismo desdoble del CONFLICTO 1**, y por la misma razón:
- por el **GET** ⇒ `evidence_corrupt` (E8 de la pantalla, que es de lectura);
- por el **POST**, después de un decide commiteado ⇒ `decided_evidence_corrupt`.

Aplica a «previa impaseable» y a «`requested_at` corrupto». Dos moldes más por
cada una, uno por puerta.

### 6.2 Cómo se FUERZA cada rama difícil

**Las ramas busy.** El `busy_timeout` vive en una constante de paquete
aplicada por el constructor; quien tiene que RECIBIR el busy es el `Store`
bajo prueba, no el atacante, y **no hay pomo corto por la puerta pública**.

1. **«claim saltado por trigger con params intactos»** se fuerza con un
   `BEFORE UPDATE ON approvals` + `RAISE(IGNORE)`: `changes()` vale 0 **sin
   concurrencia y sin error de driver**. Determinista y barato. **Su desenlace
   exigido es `not_started_params_held`**, por AS-104 y por §13-bis — no el
   `params_unaccounted` que la «tercera vía» de §11 FR-API-21 escribe, que es
   el CONFLICTO K2 y contradice al bullet de su propia sección que empieza
   «`RowsAffected() == 0` ⇒ centinela INTERNO del claim». La v3 citaba esa
   frase como fuente del mecanismo sin ver que su desenlace era el contrario.
2. **«busy por un commit ajeno con la fila intacta»** se fuerza sosteniendo la
   transacción competidora más de 5 s. Caro pero realizable.
3. **«competidor que gana el claim y vacía la fila»** exige un vaciado
   COMMITEADO dentro de nuestra ventana de lectura-escritura. Sin seam eso es
   lotería. **NO VERIFICABLE tal como está descrito** hasta decidir cómo
   forzarlo.
4. **«error de `RowsAffected()`»** —distinto del anterior: no es `n==0`, es el
   `err`— **sí tiene molde y sí tiene mecanismo**: es AS-104-bis, con driver
   envuelto y desenlace `not_started_params_held`, y la propia AS lo declara
   «el único molde de la lista que necesita una **costura de test en
   producción**». Corregido tras el hallazgo 35: la v3 lo declaró NO
   VERIFICABLE citando §15-quater, que trata de **otros tres**
   `RowsAffected()` —el del decide, `transitionTx` y `closeCrashOrphan`— que
   este tren no cura y a los que la spec expresamente no les pide molde. Era
   grave: aceptar aquel «NO VERIFICABLE» metía la cura 17 en el árbol sin su
   molde, que es lo que la doctrina prohíbe. **Ésta es la TERCERA costura.**

**«rechazo de la CLI confirmado entre la primera y la segunda lectura».** Una
llamada atómica no admite un commit ajeno en medio por la puerta pública, así
que la mutación de §13-bis no enrojecería **sin** un punto de sincronización.
**Y ese punto ya está declarado y aprobado en AS-100**, con su mecanismo
entero: el molde vive en el almacén, abre la transacción del detalle, hace su
primera lectura, espera en un canal a que la segunda conexión confirme el
`Commit` de su rechazo, y solo entonces sigue leyendo dentro de la misma
transacción. Nivel, el que AS-100 declara: **dos conexiones reales con punto de
sincronización dentro del handler**. La v2 «descubrió» que no se podía forzar,
la v3 y la v4 pidieron tu visto para la costura: las tres por no haber leído
§12. No hace falta ningún visto.

**«una sola resolución de la ley».** Con el perfil intacto,
`BuildApprovalExecutor` resuelve la MISMA caja, así que la mutación de §13-bis
—llamarlo en vez de a `…FromCage`— **no enrojece por sí sola**. Hace falta una
**sonda de conteo**, y `ResolveApprovalLaw` y `ResolveEffectiveCage` son
funciones de paquete **sin seam de inyección**: o el molde vive dentro de
`internal/app`, o nace otro gancho. **Segundo seam que necesita tu visto**; la
v2 pidió el primero y calló éste.

### 6.3 Niveles de evidencia, etiquetados sin inflar

- **[1conn]** — un almacén SQLite real, un handle. La mayoría.
- **[2conn]** — dos `*sql.DB` sobre el mismo fichero. **La tabla de evidencia
  de §12 manda**, y etiqueta «dos conexiones reales» a más de una docena de AS
  —AS-96, AS-98, AS-100, AS-101, AS-102, AS-105 a AS-107, AS-113 a AS-118,
  AS-120— porque la ley de la verificación cruzada exige el ataque **desde
  fuera** del componente, no porque haga falta una carrera. Para AS-113 el
  segundo handle es load-bearing: el `PRAGMA foreign_keys=OFF` va en la
  conexión **atacante**. Corregido tras el hallazgo 36: la v3 escribió «[2conn]
  **solo** tres» y con eso degradaba en bloque una docena de moldes aprobados.
  Lo único que este papel reetiqueta a **[1conn]** son dos que la v1 había
  inflado por su cuenta y que §12 no manda: «decidida entre lista y detalle» y
  «params mutados entre commit y claim», los dos secuenciales.
- **Oráculos por imposibilidad:** «sin consumir» y «sigue `PENDING`» con
  **triggers que aborten**, no comparando dumps. «Una sola resolución» con
  **sonda de conteo** (§6.2).

---

## 7. Riesgos abiertos, sin resolver

**R1 — UN molde NO VERIFICABLE como está descrito:** «competidor que gana el
claim y vacía la fila», que exige un vaciado commiteado dentro de la ventana
lectura-escritura del claim. El otro que la v3 declaraba —«error de
`RowsAffected()`»— **sí es verificable**: es AS-104-bis, con driver envuelto.
§6.2.

**R2 — REABIERTO, y la v5 lo retiró mal.** AS-100 se puede leer de dos maneras
y **no son compatibles**. Su frase describe al MOLDE como sujeto —«el molde
vive en el almacén, **abre** la transacción del detalle, **hace** su primera
lectura, **espera** en un canal»—, pero su mutación es de PRODUCCIÓN —«sacar
las lecturas del detalle de su transacción única»— y su última línea dice
«punto de sincronización **dentro del handler**».

- Si manda la primera lectura, el molde **nunca llama a la puerta de la cura
  20**: ejecuta su propio SQL, y cambiar la puerta de `tx` a `s.db` **no lo
  enrojece**. Sería un molde que prueba el aislamiento de instantánea de
  SQLite, que es justo lo que AS-103 declara que **no es** un test de garantía.
- Si manda la segunda, hace falta un gancho de pausa **dentro** de la puerta de
  producción: costura que la fila 20 de §17 F no autoriza y que choca con el
  «único» de AS-104.

**Qué mitad de AS-100 manda es tuyo.** Yo retiré este bloqueo apoyándome en la
primera lectura sin ver que dejaba G2 sin test que la rompa.

**R3 — `isBusyClass` guarda por TEXTO y es no exportada.** Guardar por texto es
la clase (g) de la RULE 2; no exportada obliga a exportarla —fuera de las 25—
o a una tercera copia del guard. Ninguna de las dos está autorizada.

**R4 — El TOCTOU de la ejecución (V10, B9) no lo cura este tren.** Único camino
que produce efecto irreversible real, y ninguna de las 25 curas lo toca.
Fichado con su reproducción. **Si lo quieres dentro, sube el alcance.**

**R5 — Las dos copias del canal no se cruzan.** La previa guarda el canal
recortado; `actions.source_channel` lo guarda sin recortar, y ningún cinturón
compara las dos. No lo cura este tren. *(La v2 tenía aquí una acusación a la
spec madre que era falsa y queda retirada: la previa sí sella el canal, y
FR-API-1 tiene razón.)*

**R6 — `DecideApprovalUnderLaw` señala en banda por cuatro salidas (V8) y no
tiene molde.** Un adaptador que lea solo el `error` publicaría el recibo de una
decisión que no ocurrió. **Necesita tu adjudicación.**

**R7 — La cuenta del rojo.** 38 de §13-bis, **−1** y **+2** por el CONFLICTO
1 (el único que sigue siéndolo), **+7** huecos —1, 2, 3A, 3B, 4, 5 y 6— = **46**;
con R6 y R14, 48. De ellos **uno no es ejecutable hoy** (R1), así que el
denominador de la línea diaria sería **47**. El CONFLICTO 3 dejó de restar
porque dejó de ser conflicto. No es 21 en ninguna lectura.

**R14 — El camino del RECHAZO no tiene nombre para el fallo de su re-lectura.**
La cura 25 obliga a re-leer `decision_receipt_id` porque
`DecideApprovalUnderLaw` no lo devuelve; si esa re-lectura falla **después** de
un rechazo commiteado, ninguno de los 22 sirve: `params_unreadable` habla de
una ejecución que en un rechazo no existe. **Necesita tu adjudicación**: o un
nombre nuevo, o una frase propia para ese caso.

**R8 — El deadlock de una conexión (V11).** Al mover el detalle a una
transacción, ninguna rama puede quedarse leyendo de `s.db`, o el detalle se
cuelga hasta el deadline y el operador lee «reintenta» sobre un interbloqueo
nuestro. Hoy `verifyApprovalStory` se invoca con los dos receptores.

**R9 — El tamaño.** Cuarenta y seis moldes sobre los **cuatro** paquetes que
§17 F nombra, seis curas de almacén y un adaptador nuevo, en un solo rojo. §17 H ya adjudicó que el tren no
se parte y no lo discuto; dejo escrito el riesgo.

**R10 — §13-bis está parcialmente rancia y nadie la ha revisado entera contra
§11.** Encontré UN conflicto verificándola fila a fila contra las ramas que
este papel toca. No he cruzado las 38 contra toda la §11. **Puede haber más.**

**R11 — D3 sin molde.** El techo `write_irreversible` con única herramienta
`critical` se deniega por `effect_ceiling` y no aparca nada (FR-API-6). El
molde de la quinta condición no lo cubre. Un `brains_can_park` que cuente ese
cerebro publica un número falso en V1 y E4 y nada enrojece.

**R12 — TRES costuras, y las tres ya están declaradas en §12.** La de la
instantánea (AS-100), la del busy sobre otra aprobación (AS-102) y la del
driver envuelto (AS-104-bis). **Ninguna necesita visto nuevo.** Lo único que
queda abierto es la **sonda de conteo** de la resolución única, que §12 no
declara y que §6.2 describe: `ResolveApprovalLaw` y `ResolveEffectiveCage` son
funciones de paquete sin inyección, así que o el molde vive dentro de
`internal/app` o nace un gancho. Y queda el K5: AS-104-bis se llama «el único»
y son tres.

**R13 — `brain_gone` no tiene centinela y ninguna cura lo crea.** Su única
fuente es un `fmt.Errorf` plano, duplicado en dos ficheros de `internal/app`.
Nombrarlo hoy exige `strings.Contains`, que FR-API-15 llama un hallazgo. Y su
único molde es de jsdom (AS-94), así que del lado del servidor no hay nada que
enrojezca si el texto cambia. **O entra un centinela —fuera de las 25— o el
nombre se declara no servible por este tren.** Necesita tu adjudicación.

---

## 8. Lo que este papel NO resuelve

El adaptador no está escrito; la superficie sí, y en verde contra un almacén
falso. Los trece hechos de §2 son lectura de fichero con su ancla, salvo la
comprobación marcada en V11.

**Lo que bloquea el rojo y necesita tu mano:**

1. **Las cuatro contradicciones de la spec madre** (§0-quater K1-K4; la K5 la
   había fabricado yo truncando una cita y queda retirada).
2. **R2, reabierto** — qué mitad de AS-100 manda. De eso depende que G2 tenga
   o no un test que la rompa.
3. **La sonda de conteo** de la resolución única, la única costura que §12 no
   declara.
4. **El molde NO VERIFICABLE** (R1).
5. **R13** — `brain_gone` sin centinela.
6. **R14** — el rechazo sin nombre para el fallo de su re-lectura.
7. **R4** — el TOCTOU de la ejecución: dentro del tren o fichado.
8. **R6** — el molde de la señal en banda.
9. **R7** — el denominador: 46 moldes, 47 con R6 y R14, uno no ejecutable.

**Los tres defectos del árbol que este papel destapó**, y que no son de papel
ni se arreglan adjudicando:

- **V10** — el cinturón de la ejecución juzga una terna leída dos
  transacciones antes, con la aprobación ya consumida.
- **V12** — una columna de tiempo corrupta en UNA fila se lleva la LISTA
  entera.
- **V13** — `transitionTx` rehúsa con error plano y **ninguno de los 22
  nombres lo cubre**: la transacción rueda atrás entera y el operador leería
  «no sabemos si el efecto llegó a ocurrir» sobre algo que no hizo nada.

**Sobre el método, y es lo último que digo yo.** Cinco pasadas, cinco VETO
MANTENIDO. La quinta confirmó que lo que más había costado ya está sólido —la
mutación del HUECO 2 a la tercera, V12, la aritmética— y encontró tres P1 más,
dos de ellos en texto que yo escribí para curar la cuarta. Ese es el patrón y
no se ha roto: **curar produce superficie nueva, y la superficie nueva no ha
pasado por nadie**.

La regla de parada la aplico yo sin consultar, sobre la CLASE del hallazgo, y
no voy a rebajar un P1 real para cerrar antes. Pero la clase no ha bajado en
cinco intentos, y de los nueve bloqueos que quedan **seis son adjudicaciones
tuyas que ninguna pasada más va a resolver**. La decisión de si esto sigue en
papel o el resto se cura en el rojo —donde una mutación que no enrojece se ve
en un minuto en vez de discutirse en prosa— es tuya.
