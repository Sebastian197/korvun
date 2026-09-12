# Petición de diseño — la pantalla de aprobaciones

Para Chano, por la sexta ley: esta pantalla no abre RED sin tu visto bueno.
Tres decisiones, y solo tres. El resto del tren ya está resuelto y los
cuatro endpoints arrancan en paralelo.

## Qué hace la pantalla

Una acción irreversible aparca. El operador la ve, lee **exactamente** lo
que va a ocurrir, y pone el dedo en el sí o en el no. Hoy eso solo se
puede hacer en un terminal; esto lo trae a la ventana.

Lo que hay que enseñar por cada petición aparcada:

| Dato | De dónde sale |
|---|---|
| **el digest** | lo que el dedo aprueba, y lo único que el servidor re-verifica |
| la operación y su clase de efecto | `write_irreversible`, con su frase de irreversibilidad |
| los parámetros crudos | la URL y el cuerpo reales, tal cual |
| la ley que la exigió | versión y digest de la política |
| cuándo expira | la decisión se juzga en el toque |

## Decisión 1 — dónde va el digest, y cómo se hace inevitable leerlo

La CLI lo imprime **primero y prominente**, antes del preview entero. Una
pantalla tiene más opciones y peores defaults: el digest puede acabar en
una esquina, plegado, o debajo de los botones.

La pregunta: **¿cómo se garantiza que el operador ha visto el digest antes
de que aprobar sea alcanzable?** No pido una respuesta técnica; pido la
forma. Ejemplos de familias posibles, no propuestas: el digest ocupa la
posición dominante y los botones viven por debajo del pliegue; los botones
no existen hasta que el preview se ha desplegado; el digest se repite en
la confirmación.

## Decisión 2 — qué forma tiene la confirmación de un sí irreversible

Un clic que dispara un efecto externo irreversible. Sin vuelta atrás, sin
deshacer documentado.

La pregunta: **¿qué se interpone entre el clic y el efecto?** Nada, un
segundo clic, escribir algo, un temporizador. Es decisión de producto: la
fricción que protege también molesta, y la que molesta se aprende a saltar
sin mirar.

## Decisión 3 — cómo se distinguen aprobar y rechazar

Nadie debe equivocarse de botón, y el coste de los dos errores no es el
mismo: rechazar por error se rehace; aprobar por error dispara el efecto.

La pregunta: **¿qué los separa además del color?** Posición, tamaño,
distancia entre ellos, forma, el orden en que aparecen, si el destructivo
es el primario o el secundario. El color solo no vale — ni para quien no
lo distingue, ni para quien va rápido.

## Restricciones que el diseño hereda

| | |
|---|---|
| Estado explícito con el núcleo parado | el proxy responde `503 core stopped`; **no** es una lista vacía |
| Estado explícito con aprobaciones apagadas | **no** es una lista vacía; lo dice con sus palabras |
| Sin lotes | una petición, una decisión, un digest |
| Sin editar lo aparcado | aprobar ejecuta el objeto guardado o no ejecuta |
| Sin notificaciones ni badge | la lista se consulta, no avisa |
| Solo escritorio | ni móvil ni web |

## Qué NO necesito que decidas ahora

La estética, la tipografía, el tema. Solo las tres de arriba, porque son
las que cambian el comportamiento y las que no se pueden corregir después
sin rehacer la pantalla.
