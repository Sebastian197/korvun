# Mutaciones probatorias — el almacén y el adaptador de aprobaciones (2026-09-12)

Cuarenta y ocho mutaciones sobre `internal/action/sqlite` e `internal/app`, cada
una aplicada SOLA, los dos paquetes ejecutados enteros, la mutación revertida.
Nivel de evidencia: un almacén SQLite real; los ataques del adaptador entran por
un `*sql.DB` segundo sobre el mismo fichero.

**Cuarenta y nueve moldes, y ninguno queda sin una mutación que lo enrojezca.**

| # | Qué neutraliza | Moldes que enrojece |
|---|---|---|
| A1 | condicion 1: el almacen de acciones deja de exigirse | `TestApprovalsGate_countsOnlyTheBrainsThatCanActuallyPark`, `1_·_no_action_store:_the_storage_block_is_gone` |
| A2 | condicion 2: el bloque agent deja de exigirse | `TestApprovalsGate_countsOnlyTheBrainsThatCanActuallyPark`, `2_·_not_an_agent_brain:_no_agent_block` |
| A3 | el techo deja de acotar: ni la condicion 3 ni el rango | `TestApprovalsGate_countsOnlyTheBrainsThatCanActuallyPark`, `3_·_the_ceiling_does_not_reach_write_irreversible` |
| A4 | la clase aparcable deja de exigirse | `TestApprovalsGate_countsOnlyTheBrainsThatCanActuallyPark`, `4_·_no_tool_of_a_parkable_class_in_the_cage` |
| A5 | condicion 5: la gobernanza deja de consultarse | `TestApprovalsGate_countsOnlyTheBrainsThatCanActuallyPark`, `5_·_governance_denies_the_only_parkable_tool` |
| A6 | el gate apagado contesta una lista vacia | `TestApprovalsGate_offMeansOffAndNeverAnEmptyList` |
| A7 | la caducada se pinta como ya decidida | `TestAdapter_expiredComesFromASweptRow` |
| A8 | la ley vigente deja de viajar | `TestAdapter_invalidatedCarriesTheCurrentLawInItsOwnField` |
| A9 | la corrupcion en la lectura se sirve como transitoria | `TestAdapter_aMutatedStoryIsEvidenceCorruptOnTheRead` |
| A10 | la frontera del commit desaparece: nada esta sellado | `TestAdapter_theSameCorruptionAfterACommittedDecideIsDecide` |
| A11 | el digest del operador deja de compararse | `TestAdapter_aStaleDigestIsRefusedWithoutConsumingTheApprov` |
| A13 | el status desconocido sirve el documento | `TestAdapter_aRowDecidedBetweenTheListAndTheDetailIsAlready`, `TestAdapter_anUnknownStatusFailsClosed` |
| A14 | brain_gone pierde su centinela | `TestAdapter_brainGoneHasItsOwnSentinel` |
| A15 | la PENDING en el POST se manda a ya cerrada | `TestExecuteApprovedAction_aPendingRowIsNotDecided` |
| A16 | la cuenta de filas saltadas no llega al wire | `TestAdapter_theListSurfacesTheRowsItHadToSkip` |
| A17 | la accion que ya no estaba pendiente dice no saber | `TestAdapter_theActionThatWasNoLongerPendingGetsItsName` |
| A18 | la ley se resuelve dos veces | `TestAdapter_resolvesTheLawExactlyOnce` |
| A19 | la senal en banda se ignora | `TestAdapter_readsTheInBandRuleAsARefusal` |
| A20 | el recibo del rechazo no se relee | `TestAdapter_carriesTheReceiptItHadToReReadIt` |
| A21 | el fallo de relectura del recibo dice que la ejecucion no arranco | `TestAdapter_aFailedReceiptReReadNeverSaysTheExecutionDidNo` |
| A23 | ningun cerebro puede aparcar nunca (el control) | `TestApprovalsGate_countsOnlyTheBrainsThatCanActuallyPark`, `all_five:_the_control` |
| S1 | la lista esconde la fila cuya previa no parsea | `TestListPendingApprovals_anUnreadablePreviewKeepsItsRow` |
| S2 | el canal se indexa sin comprobar la longitud | `TestListPendingApprovals_anEmptyResourcesSetIsNotAChannel` |
| S3 | el canal nunca se da por conocido | `TestListPendingApprovals_aLegitimatelyEmptyChannelIsKnown` |
| S4 | la cota de la lista sirve una fila de mas | `TestListPendingApprovals_isBoundedAtTheLimit` |
| S5 | una fila que no escanea tumba la lista entera | `TestListPendingApprovals_aCorruptTimeDoesNotTakeTheListDow` |
| S6 | la fila saltada no dice por que | `TestListPendingApprovals_aCorruptTimeDoesNotTakeTheListDow` |
| S7 | la terna sale de la previa, sin version | `TestApprovalDetail_bringsTheTernaAndTheParamsTogether` |
| S8 | la version se fija a la constante de produccion | `TestApprovalDetail_refusesAMutatedOpVersion`, `TestClaimApprovalParamsUnderDigest_judgesTheTernaItReadIts` |
| S10 | la fila ausente se lee como fallo de driver | `TestApprovalDetail_separatesTheMissingRowFromTheDriverFail` |
| S11 | la columna corrupta se lee como transitoria | `TestApprovalDetail_namesEachBeltThatRefuses`, `requested_at_unparseable` |
| S12 | la previa impaseable sale sin centinela | `TestApprovalDetail_namesEachBeltThatRefuses`, `preview_unparseable` |
| S13 | el binding de la previa sale sin centinela | `TestApprovalDetail_namesEachBeltThatRefuses`, `preview_binding` |
| S14 | el cinturon de historia sale sin centinela (effect_class) | `TestApprovalDetail_namesEachBeltThatRefuses`, `story:_effect_class` |
| S15 | el cinturon de historia sale sin centinela (operacion) | `TestApprovalDetail_namesEachBeltThatRefuses`, `story:_operation` |
| S16 | el cinturon de decision sale sin centinela (policy) | `TestApprovalDetail_namesEachBeltThatRefuses`, `story:_decision_policy` |
| S17 | la ley movida se pliega en corrupcion | `TestApprovalDetail_theLawThatMovedHasItsOwnSentinel` |
| S18 | la columna vacia se clasifica como present | `TestApprovalDetail_classifiesTheParamsState`, `empty:_born_without_arguments`, `TestAdapter_aRowBornWithoutArgumentsIsEmptyInA200` |
| S19 | el claim colapsa la columna vacia en fila ausente | `TestClaimApprovalParams_tellsItsThreeRefusalsApart`, `the_row_is_there_and_the_column_is_empty` |
| S20 | ApprovalParams colapsa la columna vacia en fila ausente | `TestApprovalParams_tellsTheMissingRowFromTheEmptyColumn` |
| S21 | el claim vuelve a descartar el error de RowsAffected | `TestClaimApprovalParams_propagatesTheRowsAffectedError` |
| S22 | el claim no juzga el digest de la fila que leyo | `TestClaimApprovalParamsUnderDigest_judgesTheTernaItReadIts` |
| S23 | el rehuse de transitionTx pierde su nombre | `TestDecideApprovalUnderLaw_namesTheActionThatWasNoLongerPe` |
| S24 | el cinturon del detalle corre despues de clasificar | `TestApprovalDetail_refusesAMutatedOpVersion`, `TestAdapter_mutatedParamsRefuseBeforeClassifying` |
| S25 | la fila de actions ausente cae en el residual del cinturon | `TestApprovalDetail_anAbsentActionRowIsCorruptionNotAbsence` |
| S27 | un cuerpo legible se clasifica como no disponible | `TestApprovalDetail_classifiesTheParamsState`, `present` |
| S28 | el claim colapsa la fila ausente en columna vacia | `TestClaimApprovalParams_tellsItsThreeRefusalsApart`, `the_row_is_not_there` |
| S29 | la lista corre el cinturon (fuera del bucle) y esconde la fila que no verifica | `TestListPendingApprovals_bringsTheRowWithoutRunningTheBelt` |

## Las que hubo que rehacer, y por qué

Una mutación que no enrojece es un hallazgo, pero hay que distinguir el agujero
del molde de la mutación mal hecha. Las nueve que fallaron en la primera pasada:

| # | Qué pasó | Qué era |
|---|---|---|
| S5, S22, A10, A20 | no compilaban (dejaban una variable sin usar) | mutación mal hecha; rehechas compilables, enrojecen |
| S21 | ancla ambigua: el mismo literal en los dos claims | mutación mal hecha; anclada al cuerpo entero |
| A12 | un `break` en un `case` de Go no hace nada | **inerte por construcción**; retirada — la rama la vigila A13 |
| S9 | ataca la rama de `ternaOf` que el cinturón anterior ya defiende | **código inalcanzable por esa puerta**; sustituida por S25, que sí la recorre |
| A4 | el caso usaba `time_now`, que **no tiene descriptor**: el molde pasaba por ausencia, no por clase | **agujero del molde**; el caso pasa a `read_file`, que es `read_external` declarado |
| S26 | una consulta anidada dentro del bucle de filas se cuelga sobre un pool de UNA conexión | mutación mal hecha — y es el interbloqueo que R8 fichó; rehecha fuera del bucle como S29 |

## Un agujero del propio banco

La primera versión del banco corría **solo el paquete del fichero mutado**, así
que una mutación del almacén que enrojece un molde del adaptador se contaba como
muda. Corregido: cada mutación ejecuta los dos paquetes. Sin esa corrección,
`S18` y `S24` habrían pasado por mudas siendo correctas.

## Segunda tanda — las mutaciones de lo CURADO tras el veto (2026-09-13)

Nueve mutaciones sobre las tres curas del veto. Ocho enrojecen.

| # | Qué neutraliza | Rojas |
|---|---|---|
| C1 | GetApproval vuelve a devolver el ErrNotFound generico | 4 |
| C2 | GetApprovalByAction vuelve al generico | 2 |
| C3 | approvalTx vuelve al generico | 2 |
| C4 | dos centinelas se responden el uno al otro | 1 |
| C5 | el camino unico vuelve a juzgar una terna leida fuera del claim | 0 |
| C6 | el corte PENDING se manda a ya cerrada | 1 |
| C7 | una ejecucion fallida vuelve a salir por el canal de error | 2 |
| C8 | el desenlace fallido se publica como ejecutado | 1 |
| C9 | la cuenta de filas saltadas deja de viajar | 1 |

**C5 sale MUDA y es correcto, declarado.** Neutraliza que el camino único
ejecute la terna que el claim juzgó, sustituyéndola por la que leyó `store.Get`
antes. No enrojece porque **su rama está defendida aguas arriba**: la terna
entra en `action.Digest`, así que un competidor que la mueva hace que el
digest deje de re-derivar DENTRO de la transacción del claim y el claim
rehúsa antes de que nadie ejecute nada. Usar la terna devuelta es defensa en
profundidad, no el guardián que carga el peso, y este papel no lo vende como
tal. Sin un punto de sincronización —el que AS-100 declara— las dos lecturas
coinciden por construcción y la mutación no puede distinguirse.

**C1 enrojece cuatro moldes y C3 dos**: la dirección de los centinelas no era
un fallo de un sitio, era de clase, y el molde que la vigila recorre las siete
puertas.

## Tercera tanda — las tres del segundo veto (2026-09-13)

| # | Qué neutraliza | Rojas |
|---|---|---|
| D1 | el cinturon del digest vuelve a correr antes de la columna vacia | 3 |
| D2 | el fallo de scan de GetApproval vuelve a salir desnudo | 2 |
| D3 | la previa impaseable de GetApproval vuelve a salir desnuda | 2 |
| D4 | toda columna vacia vuelve a llamarse gone | 2 |
| D5 | la re-lectura deja de re-derivar: todo vacio es nacido vacio | 2 |
| D6 | la sonda de la ley deja de ver la resolución que prohíbe | 1 |

**D6 se ejecutó a mano** y es la que importa: es la mutación que el molde
DECLARABA y que antes lo dejaba verde. Con la sonda movida a
`ResolveApprovalLaw` —por donde pasa toda resolución— sustituir
`BuildApprovalExecutorFromCage` por `BuildApprovalExecutor` da
«law resolutions = 2, want exactly 1» y el molde enrojece.

**D1 enrojece tres moldes.** Devolver el cinturón del digest delante de la
comprobación de columna vacía se lleva por delante los tres peldaños que
distinguen «no arrancó» de «algo se los llevó»: era un orden, no un detalle.

## Cuarta tanda — las tres reglas estructurales (2026-09-13)

| # | Qué neutraliza | Rojas |
|---|---|---|
| E1 | el rehuse por ley movida del DECIDE vuelve a salir desnudo | 2 |
| E3 | el fallo de store.Get pierde su nombre y cae en la escalera | 1 |
| E4 | la escalera deja de distinguir: todo vacio es held | 2 |
| E5 | el enrutado de produccion a la escalera desaparece | 4 |

**E5 es la que importa.** Borrar el enrutado de producción hacia la escalera
enrojece **los cuatro peldaños**. La versión anterior del molde entraba por
`nameClaim` directamente y esa misma mutación la dejaba VERDE: un molde que
entra por una puerta que producción no usa no puede ver quién más entra.

**E2 salió MUDA y destapó un molde mío que decía otra cosa de la que hacía.**
El cuarto peldaño afirmaba ejercitar la rama corrupta de la RE-LECTURA; en
realidad la corrupción la caza el CLAIM, que lee la fila de aprobación antes
que los params. La rama de la re-lectura queda declarada como defensa en
profundidad y NO como cubierta — ni en el código ni en el molde. E2 se retira
por eso: no había rama que mutar, había una frase que corregir.
