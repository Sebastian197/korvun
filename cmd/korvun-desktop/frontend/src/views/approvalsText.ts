// The literals of the approvals screen, in one place, because they ARE the
// contract: §12 of the spec asserts them byte for byte, and R15 taught what
// happens when operator-facing words live only inside a render and nothing
// watches them.
//
// Rendering is BY NAME, never by the English text of the body (E9). This file
// is the map from a name to what a Spanish-speaking operator reads, and every
// entry has an anchor in §12-ter.

/** A response the screen understood well enough to name. */
export interface NamedError {
  name: string
  message: string
  currentLawDigest?: string
  status: number
}

/** The body did not parse, or the transport failed. */
export interface UnreadableAnswer {
  kind: 'unreadable'
  detail: string
}

export const ESC_LINE = 'Esc rechaza mientras esta petición esté abierta y sin decidir.'

export const PERMANENT_LINE = 'Esto no es transitorio. Guarda el identificador y mira el libro.'

/** E9's last two rows: nothing degrades to an empty list or a generic shrug. */
export const UNREADABLE_TITLE = 'El núcleo no ha contestado nada legible.'
export const UNKNOWN_NAME_TITLE = 'Respuesta que esta pantalla no reconoce.'

/** The nine belt names E8 can print. The screen prints whatever comes without
 * presuming this list — it exists only to document what the store can say. */
export const SURFACE_NOT_MOUNTED =
  'Esta ventana no encuentra la puerta de aprobaciones en el núcleo. Suele ser un perfil sin bloque admin: sin él no se genera credencial y la superficie de mutación no se monta.'

/** The execution outcomes, after a decision that is already committed (P4).
 * Each one says what its evidence sustains and not a word more; the column
 * "did the effect happen" is what FR-UI-18 fixes and several of these answer
 * "no se sabe" on purpose. */
export const OUTCOME_TEXT: Record<string, string> = {
  not_started_params_held:
    'La decisión quedó registrada y esta ejecución no llegó a salir. La petición conserva sus parámetros ahora, y el libro solo cierra las aprobadas que ya no los conservan, así que ninguna pasada automática la ha cerrado. Sobre otros ejecutores esta pantalla no se pronuncia: el almacén es de varios procesos y ninguna lectura de aquí puede prometer que nadie más la ejecute. El camino es korvun approvals execute una vez restaurada la causa.',
  not_started_params_gone:
    'La decisión quedó registrada y esta ejecución no llegó a salir. Esta petición ya no conserva sus parámetros: si la acción sigue en el libro, el próximo arranque la cerrará como desenlace desconocido, con su recibo.',
  params_unaccounted:
    'Esta ejecución no arrancó, y esta pantalla no puede afirmar que la petición no se haya ejecutado: al volver a leerla, sus parámetros ya no estaban donde estaban, o la fila de aprobación no estaba. Compruébalo antes de repetir nada.',
  params_unreadable:
    'Esta ejecución no arrancó, y el almacén no se pudo leer. Por eso esta pantalla no puede afirmar que la petición no se haya ejecutado. No dice que nadie se los llevara: dice que no lo sabe. Reintenta la lectura antes de repetir nada.',
  decided_evidence_corrupt:
    'La decisión quedó registrada y sellada, con su recibo, y esta ejecución no llegó a salir. Lo que ya no verifica es la evidencia guardada. Esto es permanente. Sobre lo que haya hecho otro ejecutor esta pantalla no se pronuncia, y sobre el estado en que queda la petición tampoco. Mira el libro antes de repetir nada.',
  already_closed: 'Esta petición no está esperando ejecución. Esta ejecución no ha hecho nada.',
  not_decided:
    'El almacén dice que esta petición sigue esperando decisión, así que esta ejecución no ha hecho nada. Si acabas de decidirla desde aquí, alguien ha reescrito su estado por debajo: mira el libro antes de repetir nada.',
  unknown_outcome:
    'La decisión salió de esta ventana. No sabemos si el efecto llegó a ocurrir. Compruébalo antes de repetir nada.',
  close_failed:
    'La decisión salió de esta ventana. No sabemos si el efecto llegó a ocurrir, y además esta ejecución no pudo cerrar el registro. Compruébalo antes de repetir nada.',
}

/** The four parameters_state literals of FR-UI-16. `present` is the only one
 * that offers the yes. */
export const PARAMS_STATE_TEXT: Record<string, string> = {
  empty:
    'esta acción se aparcó sin parámetros, y así no se puede ejecutar: el claim rechaza la fila vacía',
  unavailable:
    'los parámetros no se pudieron leer en este instante; es transitorio y no dice nada sobre la evidencia',
  too_large: 'esta pantalla no puede enseñarte esta petición entera, así que no te ofrece el sí',
}

/** §3's ladder: the class decides the gate BY RANK, never by a text. */
export interface ClassBanner {
  label: string
  phrase: string
}
export function classBanner(effectClass: string): ClassBanner {
  switch (effectClass) {
    case 'write_irreversible':
      return { label: 'IRREVERSIBLE', phrase: 'escribe sin deshacer y sin compensación conocida' }
    case 'critical':
      return { label: 'CRÍTICO', phrase: 'mueve dinero, credenciales o equivalente' }
    case '':
      return {
        label: 'SIN CLASE LEGIBLE',
        phrase:
          'no se ha podido leer la previa de esta petición; ábrela para ver qué dice el almacén',
      }
    default:
      if (KNOWN_CLASSES.has(effectClass)) {
        return {
          label: `ANOMALÍA · ${effectClass}`,
          phrase:
            'Esta petición no debería existir: el gate solo aparca irreversible y crítico. Trátala como sospechosa.',
        }
      }
      return {
        label: 'CLASE DESCONOCIDA',
        phrase: 'fuera de la escalera: se trata por encima de crítico',
      }
  }
}

/** The ladder of internal/action/effect.go. A value outside it is not an
 * anomaly but an unknown class, and the two get different banners. */
const KNOWN_CLASSES = new Set([
  'pure',
  'read_external',
  'write_reversible',
  'write_compensatable',
  'write_irreversible',
  'critical',
])
