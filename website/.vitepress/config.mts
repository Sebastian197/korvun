import { defineConfig } from 'vitepress'

// Site config (design spec 2026-08-02, ADR-0040).
// base '/korvun/' is the GitHub *project page* subdirectory — VitePress
// prepends it to every internal URL starting with '/' and to static assets
// (Context7-verified); the custom-domain future extension flips it to '/'.
// Locales: root = EN (the source of truth, authored first), es = a FULL
// MIRROR of the EN tree (Chano's 2026-08-02 decision, FR-I18N-1 as
// amended) — the locale-parity gate keeps both sides identical.
export default defineConfig({
  base: '/korvun/',
  // VitePress builds EVERY .md under the source root — and Playwright
  // writes error-context.md artifacts into test-results/, which ended up
  // PUBLISHED in dist (the docs-map gate caught them). Exclude test
  // artifacts from the site source (srcExclude verified in the installed
  // 1.6.4 types).
  srcExclude: ['**/test-results/**', '**/playwright-report/**'],
  title: 'Korvun',
  description:
    'Self-hosted AI messaging gateway, multi-model router, and multi-brain orchestrator in a single Go binary.',

  // Social cards (FR fold-in): absolute URLs on purpose — scrapers do not
  // resolve relative paths. The image is the repo social preview, copied
  // into the site's own assets (same-origin posture).
  head: [
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:title', content: 'Korvun' }],
    [
      'meta',
      {
        property: 'og:description',
        content:
          'Self-hosted AI messaging gateway, multi-model router, and multi-brain orchestrator in a single Go binary.',
      },
    ],
    [
      'meta',
      {
        property: 'og:image',
        content:
          'https://sebastian197.github.io/korvun/brand/korvun-social-preview.png',
      },
    ],
    [
      'meta',
      { property: 'og:url', content: 'https://sebastian197.github.io/korvun/' },
    ],
    ['meta', { name: 'twitter:card', content: 'summary_large_image' }],
  ],

  themeConfig: {
    // In-browser full-text search (MiniSearch) — no external service
    // (zero-CDN/zero-analytics posture, ADR-0040 §3). The es strings ride
    // search.options.locales (the Context7-verified shape) so the search
    // UI speaks Spanish on /es/ (FR-I18N-2 / AS-5).
    search: {
      provider: 'local',
      options: {
        locales: {
          es: {
            translations: {
              button: {
                buttonText: 'Buscar',
                buttonAriaLabel: 'Buscar',
              },
              modal: {
                displayDetails: 'Mostrar lista detallada',
                resetButtonTitle: 'Borrar búsqueda',
                backButtonTitle: 'Cerrar búsqueda',
                noResultsText: 'Sin resultados para',
                footer: {
                  selectText: 'seleccionar',
                  selectKeyAriaLabel: 'intro',
                  navigateText: 'para navegar',
                  navigateUpKeyAriaLabel: 'flecha arriba',
                  navigateDownKeyAriaLabel: 'flecha abajo',
                  closeText: 'cerrar',
                  closeKeyAriaLabel: 'escape',
                },
              },
            },
          },
        },
      },
    },
    // ES is a FULL MIRROR of EN (Chano's 2026-08-02 decision, SP4b):
    // every page has its twin, so the default per-page i18n routing is
    // correct again — the switcher goes to the SAME page in the other
    // language. The locale-parity gate (scripts/check-parity.mjs)
    // guarantees no future page ships without its twin, which is what
    // made the SP2b-era `i18nRouting: false` workaround retirable.
  },

  locales: {
    root: {
      label: 'English',
      lang: 'en',
      themeConfig: {
        nav: [
          { text: 'Guide', link: '/guide/what-is-korvun' },
          { text: 'Reference', link: '/reference/configuration' },
          { text: 'Releases', link: '/releases/' },
          { text: 'GitHub', link: 'https://github.com/Sebastian197/korvun' },
        ],
        sidebar: {
          '/guide/': [
            {
              text: 'Guide',
              items: [
                { text: 'What is Korvun?', link: '/guide/what-is-korvun' },
                { text: 'Install', link: '/guide/install' },
                { text: 'Quickstart', link: '/guide/quickstart' },
                { text: 'The visual builder', link: '/guide/builder' },
              ],
            },
            {
              text: 'Channels',
              items: [
                { text: 'Telegram', link: '/channels/telegram' },
                { text: 'Discord', link: '/channels/discord' },
                { text: 'Webhook', link: '/channels/webhook' },
              ],
            },
          ],
          '/channels/': [
            {
              text: 'Channels',
              items: [
                { text: 'Telegram', link: '/channels/telegram' },
                { text: 'Discord', link: '/channels/discord' },
                { text: 'Webhook', link: '/channels/webhook' },
              ],
            },
          ],
          '/reference/': [
            {
              text: 'Reference',
              items: [
                { text: 'Configuration', link: '/reference/configuration' },
              ],
            },
          ],
        },
      },
    },
    es: {
      label: 'Español',
      lang: 'es',
      link: '/es/',
      description:
        'Pasarela de mensajería con IA autoalojada: router multimodelo y orquestador multicerebro en un único binario Go.',
      // Full structural mirror of the EN tree (SP4b) — the parity gate
      // and the i18n e2e keep both sides identical page for page.
      themeConfig: {
        nav: [
          { text: 'Guía', link: '/es/guide/what-is-korvun' },
          { text: 'Referencia', link: '/es/reference/configuration' },
          { text: 'Releases', link: '/es/releases/' },
          { text: 'GitHub', link: 'https://github.com/Sebastian197/korvun' },
        ],
        sidebar: {
          '/es/guide/': [
            {
              text: 'Guía',
              items: [
                { text: '¿Qué es Korvun?', link: '/es/guide/what-is-korvun' },
                { text: 'Instalación', link: '/es/guide/install' },
                { text: 'Inicio rápido', link: '/es/guide/quickstart' },
                { text: 'El builder visual', link: '/es/guide/builder' },
              ],
            },
            {
              text: 'Canales',
              items: [
                { text: 'Telegram', link: '/es/channels/telegram' },
                { text: 'Discord', link: '/es/channels/discord' },
                { text: 'Webhook', link: '/es/channels/webhook' },
              ],
            },
          ],
          '/es/channels/': [
            {
              text: 'Canales',
              items: [
                { text: 'Telegram', link: '/es/channels/telegram' },
                { text: 'Discord', link: '/es/channels/discord' },
                { text: 'Webhook', link: '/es/channels/webhook' },
              ],
            },
          ],
          '/es/reference/': [
            {
              text: 'Referencia',
              items: [
                {
                  text: 'Configuración',
                  link: '/es/reference/configuration',
                },
              ],
            },
          ],
        },
      },
    },
  },
})
