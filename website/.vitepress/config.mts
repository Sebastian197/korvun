import { defineConfig } from 'vitepress'

// Site config (design spec 2026-08-02, ADR-0040).
// base '/korvun/' is the GitHub *project page* subdirectory — VitePress
// prepends it to every internal URL starting with '/' and to static assets
// (Context7-verified); the custom-domain future extension flips it to '/'.
// Locales: root = EN (the complete source of truth), es = the ES layer
// (landing + quickstart today, expandable page-by-page — FR-I18N-1).
export default defineConfig({
  base: '/korvun/',
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
    // (zero-CDN/zero-analytics posture, ADR-0040 §3).
    search: {
      provider: 'local',
    },
    // ES is a partial layer (FR-I18N-1): most EN pages have no ES
    // counterpart, and with i18nRouting enabled the switcher links
    // page-to-page — a dead link on every untranslated page (the harness
    // caught it). `false` routes the switcher to the target locale ROOT
    // (verified against the INSTALLED 1.6.4 source: types/default-theme.d.ts
    // declares `i18nRouting?: boolean`, and langs.js gates the
    // corresponding-page link on `i18nRouting !== false`; the function form
    // Context7 shows is the unreleased 2.x). SP4 may refine per-page links
    // where a counterpart exists (FR-I18N-2).
    i18nRouting: false,
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
      themeConfig: {
        nav: [
          { text: 'Guía', link: '/es/guide/quickstart' },
          { text: 'GitHub', link: 'https://github.com/Sebastian197/korvun' },
        ],
        sidebar: {
          '/es/guide/': [
            {
              text: 'Guía',
              items: [{ text: 'Inicio rápido', link: '/es/guide/quickstart' }],
            },
          ],
        },
      },
    },
  },
})
