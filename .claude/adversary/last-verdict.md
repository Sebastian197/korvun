VETO LEVANTADO ef1f7c2a3216f2a6b89b15bba836f683e38002b1

El adversario interno devolvió VETO MANTENIDO en su OCTAVA pasada: dos P1, tres
P2 y seis P3. Los once están curados en el commit que este marcador nombra, cada
cura con su mutación probatoria ejecutada y su rojo capturado. NINGUNA pasada
interna ha juzgado esas curas.

El veto lo levanta EL DIRECTOR, no el adversario, y lo hace con la vara escrita:
ocho pasadas y decenas de defectos reales demuestran que el bucle interno hace
su trabajo y también que no converge solo — cada cura abre superficie nueva. La
puerta EXTERNA existe para esto. El tren va a Codex con cero P1 exigidos allí, y
un veto interno mantenido se declara en el cuerpo de la pull request en vez de
esconderse.

Qué encontró la octava, para que nadie tenga que buscarlo:

- P1-1 — la frontera post-entrega estaba en el sitio equivocado por segunda vez,
  y la segunda se publicó como verdadera en una nota de release y en la
  ceremonia. Un host que lee el POST entero y cuelga devuelve un EOF pelado
  dentro del bloque de error de Do: el libro lo cerraba FALLIDO. La frontera ya
  no se razona, se OBSERVA con httptrace.
- P1-2 — el centinela se leía en una de las dos rutas de ejecución.
  internal/brain ejecuta la herramienta irreversible cuando la puerta no aparca,
  y cerraba FALLIDO para todo error mientras su propio comentario decía «the
  tool refused before any effect».
- P2-1 — el guardián estructural, en su tercera forma: por nombre de herramienta
  no podía enrojecer por una rama, y por posición sintáctica dejaba pasar dos
  formas de retorno y enrojecía en falso una tercera.
- P2-2 y P2-3 — dos frases que seguían diciendo lo contrario del commit que las
  tocó.
- Seis P3, entre ellos una cura que el canto anterior DECLARÓ y el diff no
  llevaba, y una cita a un molde inexistente en la tabla de mutaciones.

Decisión del director, 2026-09-13.
