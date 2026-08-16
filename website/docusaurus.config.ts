import type { Config } from '@docusaurus/types'
import type * as Preset from '@docusaurus/preset-classic'
import { themes as prismThemes } from 'prism-react-renderer'

const normalizeBaseUrl = (value: string) => {
  const segment = value.trim().replace(/^\/+|\/+$/g, '')
  return segment ? `/${segment}/` : '/'
}

const siteUrl = (process.env.SITE_URL ?? 'https://sebastian197.github.io').replace(
  /\/$/,
  '',
)
const baseUrl = normalizeBaseUrl(process.env.SITE_BASE_URL ?? '/korvun/')

const config: Config = {
  title: 'Korvun',
  tagline: 'One binary. Your models. Your rules.',
  favicon: 'brand/korvun-logo-mono.svg',
  url: siteUrl,
  baseUrl,
  organizationName: 'Sebastian197',
  projectName: 'korvun',
  trailingSlash: true,
  onBrokenLinks: 'throw',
  onBrokenMarkdownLinks: 'throw',
  i18n: {
    defaultLocale: 'en',
    locales: ['en', 'es'],
    localeConfigs: {
      en: { label: 'English', htmlLang: 'en' },
      es: { label: 'Español', htmlLang: 'es' },
    },
  },
  presets: [
    [
      'classic',
      {
        docs: {
          routeBasePath: '/',
          sidebarPath: './sidebars.ts',
          breadcrumbs: true,
          showLastUpdateAuthor: false,
          showLastUpdateTime: false,
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
        sitemap: {
          changefreq: 'weekly',
          priority: 0.5,
        },
      } satisfies Preset.Options,
    ],
  ],
  themeConfig: {
    image: 'brand/korvun-social-preview.png',
    metadata: [
      {
        name: 'description',
        content:
          'Self-hosted AI messaging gateway, multi-model router, and multi-brain orchestrator in a single Go binary.',
      },
      { name: 'theme-color', content: '#0a0a0c' },
    ],
    colorMode: {
      defaultMode: 'dark',
      disableSwitch: true,
      respectPrefersColorScheme: false,
    },
    navbar: {
      title: 'Korvun',
      logo: {
        alt: 'Korvun',
        src: 'brand/korvun-logo-mono.svg',
      },
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'mainSidebar',
          position: 'left',
          label: 'Guide',
        },
        {
          to: '/reference/configuration',
          label: 'Reference',
          position: 'left',
        },
        { to: '/releases', label: 'Releases', position: 'left' },
        {
          href: 'https://github.com/Sebastian197/korvun',
          label: 'GitHub',
          position: 'right',
        },
        { type: 'localeDropdown', position: 'right' },
      ],
    },
    footer: {
      style: 'dark',
      links: [
        {
          title: 'Build',
          items: [
            { label: 'Quickstart', to: '/guide/quickstart' },
            { label: 'Configuration', to: '/reference/configuration' },
          ],
        },
        {
          title: 'Connect',
          items: [
            { label: 'Telegram', to: '/channels/telegram' },
            { label: 'Discord', to: '/channels/discord' },
            { label: 'Webhook', to: '/channels/webhook' },
          ],
        },
        {
          title: 'Project',
          items: [
            { label: 'Releases', to: '/releases' },
            {
              label: 'GitHub',
              href: 'https://github.com/Sebastian197/korvun',
            },
          ],
        },
      ],
      copyright: `Apache-2.0 · ${new Date().getFullYear()} Korvun`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
      additionalLanguages: ['bash', 'json'],
    },
  } satisfies Preset.ThemeConfig,
}

export default config
