export type Capability = {
  number: string
  title: string
  body: string
  linkLabel: string
  to: string
}

export type LandingCopy = {
  metaDescription: string
  hero: {
    eyebrow: string
    lines: [string, string, string]
    body: string
    primary: string
    secondary: string
    releaseLabel: string
  }
  install: {
    kicker: string
    title: string
    body: string
    proof: [string, string, string]
    copy: string
    copied: string
    guide: string
    terminalAria: string
  }
  capabilities: {
    kicker: string
    title: string
    body: string
    items: Capability[]
  }
  privacy: {
    kicker: string
    title: string
    body: string
    localLabel: string
    localModel: string
    cloudLabel: string
    cloudModel: string
    excluded: string
    reference: string
  }
  demo: {
    kicker: string
    title: string
    body: string
    ariaLabel: string
    release: string
  }
  final: {
    kicker: string
    title: string
    body: string
    primary: string
    secondary: string
  }
}

const sharedRoutes = {
  what: '/guide/what-is-korvun',
  install: '/guide/install',
  quickstart: '/guide/quickstart',
  builder: '/guide/builder',
  memory: '/guide/memory',
  configuration: '/reference/configuration',
  telegram: '/channels/telegram',
  releases: '/releases',
}

export const landingCopy: Record<'en' | 'es', LandingCopy> = {
  en: {
    metaDescription:
      'Self-hosted AI messaging gateway, multi-model router, and multi-brain orchestrator in one Go binary.',
    hero: {
      eyebrow: 'Self-hosted AI control plane',
      lines: ['One binary.', 'Your models.', 'Your rules.'],
      body:
        'Connect real messaging channels to local and cloud models, then decide how every conversation is routed. Korvun keeps the gateway, policies, and orchestration under your control.',
      primary: 'Start the quickstart',
      secondary: 'View on GitHub',
      releaseLabel: 'Current release',
    },
    install: {
      kicker: 'Verified delivery',
      title: 'Install with proof, not promises.',
      body:
        'v0.10.0 — Beta publishes six headless OS/architecture archives. The release also includes a cosign-signed checksum manifest and an SBOM for every archive.',
      proof: ['6 headless archives', 'cosign-signed checksums', 'SBOM per archive'],
      copy: 'Copy commands',
      copied: 'Copied',
      guide: 'Read the complete install and verification guide',
      terminalAria: 'Install commands',
    },
    capabilities: {
      kicker: 'Product map',
      title: 'Eight pillars. One binary.',
      body:
        'Each part solves a concrete step between an incoming message and a governed model response.',
      items: [
        {
          number: '01',
          title: 'Messaging gateway',
          body: 'Receive and reply through Telegram, Discord, or a generic webhook from one process.',
          linkLabel: 'Explore channels',
          to: sharedRoutes.telegram,
        },
        {
          number: '02',
          title: 'Multi-model routing',
          body: 'Place local Ollama and cloud Groq models behind the same routing layer.',
          linkLabel: 'Understand the model',
          to: sharedRoutes.what,
        },
        {
          number: '03',
          title: 'Multi-brain orchestration',
          body: 'Give each brain its own models, dispatch policy, and persona.',
          linkLabel: 'See the architecture',
          to: sharedRoutes.what,
        },
        {
          number: '04',
          title: 'Dispatch policies',
          body: 'Configure how eligible models run and how Korvun chooses the response.',
          linkLabel: 'Open configuration',
          to: sharedRoutes.configuration,
        },
        {
          number: '05',
          title: 'Visual builder',
          body: 'Compose channels, brains, models, and validated connections on a canvas.',
          linkLabel: 'Meet the builder',
          to: sharedRoutes.builder,
        },
        {
          number: '06',
          title: 'Korvun Desktop',
          body: 'Run the gateway and builder from the packaged desktop application.',
          linkLabel: 'Install Korvun',
          to: sharedRoutes.install,
        },
        {
          number: '07',
          title: 'Governed memory',
          body: 'Brains store bounded notes through a governed tool and recall a session deliberately — nothing is remembered by accident.',
          linkLabel: 'Meet governed memory',
          to: sharedRoutes.memory,
        },
        {
          number: '08',
          title: 'Signed releases',
          body: 'Verify the checksum manifest and inspect the supplied SBOM before running a binary.',
          linkLabel: 'Browse releases',
          to: sharedRoutes.releases,
        },
      ],
    },
    privacy: {
      kicker: 'Routing you can inspect',
      title: 'Sensitive data never leaves the machine.',
      body:
        'When a brain is marked private, cloud models are excluded from its eligible targets. The visual builder shows that exclusion as part of the graph.',
      localLabel: 'private brain',
      localModel: 'local model',
      cloudLabel: 'private brain',
      cloudModel: 'cloud model',
      excluded: 'excluded',
      reference: 'Inspect the configuration reference',
    },
    demo: {
      kicker: 'Product proof',
      title: 'A cloud model, governed tools, one config.',
      body:
        'Sixty seconds inside the Korvun desktop app: gpt-4o-mini — reached through the v0.9.0 universal gateway — is asked to remember something, the governance gate authorizes the memory tool, /notes shows the stored note, and the model answers from memory.',
      ariaLabel:
        'Korvun v0.9.0 desktop app demo: a cloud model driving the governed memory tool through the universal gateway',
      release: 'Read the v0.10.0 release notes',
    },
    final: {
      kicker: 'Build your first route',
      title: 'Run it today.',
      body:
        'Start with one channel, one brain, and one model. The quickstart keeps the first configuration small and explicit.',
      primary: 'Open the quickstart',
      secondary: 'Review the source',
    },
  },
  es: {
    metaDescription:
      'Pasarela de mensajería con IA autoalojada, router multimodelo y orquestador multicerebro en un único binario Go.',
    hero: {
      eyebrow: 'Plano de control de IA autoalojado',
      lines: ['Un binario.', 'Tus modelos.', 'Tus reglas.'],
      body:
        'Conecta canales de mensajería reales a modelos locales y cloud, y decide cómo se enruta cada conversación. Korvun mantiene la pasarela, las políticas y la orquestación bajo tu control.',
      primary: 'Empezar el inicio rápido',
      secondary: 'Ver en GitHub',
      releaseLabel: 'Release actual',
    },
    install: {
      kicker: 'Entrega verificable',
      title: 'Instala con pruebas, no promesas.',
      body:
        'v0.10.0 — Beta publica seis archivos headless para distintas combinaciones de sistema y arquitectura. La release incluye un manifiesto de checksums firmado con cosign y un SBOM por archivo.',
      proof: ['6 archivos headless', 'checksums firmados con cosign', 'SBOM por archivo'],
      copy: 'Copiar comandos',
      copied: 'Copiado',
      guide: 'Leer la guía completa de instalación y verificación',
      terminalAria: 'Comandos de instalación',
    },
    capabilities: {
      kicker: 'Mapa del producto',
      title: 'Ocho pilares. Un binario.',
      body:
        'Cada pieza resuelve un paso concreto entre un mensaje entrante y una respuesta gobernada del modelo.',
      items: [
        {
          number: '01',
          title: 'Pasarela de mensajería',
          body: 'Recibe y responde por Telegram, Discord o un webhook genérico desde un único proceso.',
          linkLabel: 'Explorar canales',
          to: sharedRoutes.telegram,
        },
        {
          number: '02',
          title: 'Enrutado multimodelo',
          body: 'Coloca modelos Ollama locales y modelos Groq cloud tras la misma capa de enrutado.',
          linkLabel: 'Entender el modelo',
          to: sharedRoutes.what,
        },
        {
          number: '03',
          title: 'Orquestación multicerebro',
          body: 'Da a cada cerebro sus propios modelos, política de despacho y personalidad.',
          linkLabel: 'Ver la arquitectura',
          to: sharedRoutes.what,
        },
        {
          number: '04',
          title: 'Políticas de despacho',
          body: 'Configura cómo se ejecutan los modelos elegibles y cómo Korvun selecciona la respuesta.',
          linkLabel: 'Abrir configuración',
          to: sharedRoutes.configuration,
        },
        {
          number: '05',
          title: 'Builder visual',
          body: 'Compón canales, cerebros, modelos y conexiones validadas sobre un lienzo.',
          linkLabel: 'Conocer el builder',
          to: sharedRoutes.builder,
        },
        {
          number: '06',
          title: 'Korvun Desktop',
          body: 'Ejecuta la pasarela y el builder desde la aplicación de escritorio empaquetada.',
          linkLabel: 'Instalar Korvun',
          to: sharedRoutes.install,
        },
        {
          number: '07',
          title: 'Memoria gobernada',
          body: 'Los cerebros guardan notas acotadas mediante una herramienta gobernada y recuperan una sesión de forma deliberada — nada se recuerda por accidente.',
          linkLabel: 'Conocer la memoria gobernada',
          to: sharedRoutes.memory,
        },
        {
          number: '08',
          title: 'Releases firmadas',
          body: 'Verifica el manifiesto de checksums y revisa el SBOM suministrado antes de ejecutar un binario.',
          linkLabel: 'Ver releases',
          to: sharedRoutes.releases,
        },
      ],
    },
    privacy: {
      kicker: 'Enrutado que puedes inspeccionar',
      title: 'Los datos sensibles no salen de tu equipo.',
      body:
        'Cuando un cerebro se marca como privado, los modelos cloud quedan fuera de sus destinos elegibles. El builder visual muestra esa exclusión como parte del grafo.',
      localLabel: 'cerebro privado',
      localModel: 'modelo local',
      cloudLabel: 'cerebro privado',
      cloudModel: 'modelo cloud',
      excluded: 'excluido',
      reference: 'Consultar la referencia de configuración',
    },
    demo: {
      kicker: 'Prueba de producto',
      title: 'Un modelo de nube, herramientas gobernadas, una sola config.',
      body:
        'Sesenta segundos dentro de la app de escritorio de Korvun: a gpt-4o-mini — conectado por la pasarela universal de v0.9.0 — se le pide recordar algo, la puerta de gobernanza autoriza la herramienta de memoria, /notes muestra la nota guardada y el modelo responde desde la memoria.',
      ariaLabel:
        'Demo de la app de escritorio de Korvun v0.9.0: un modelo de nube usando la herramienta de memoria gobernada por la pasarela universal',
      release: 'Leer las notas de v0.10.0',
    },
    final: {
      kicker: 'Crea tu primera ruta',
      title: 'Ponlo en marcha hoy.',
      body:
        'Empieza con un canal, un cerebro y un modelo. El inicio rápido mantiene la primera configuración pequeña y explícita.',
      primary: 'Abrir el inicio rápido',
      secondary: 'Revisar el código',
    },
  },
}
