import type { SidebarsConfig } from '@docusaurus/plugin-content-docs'

const sidebars: SidebarsConfig = {
  mainSidebar: [
    {
      type: 'category',
      label: 'Guide',
      collapsed: false,
      items: [
        'guide/what-is-korvun',
        'guide/install',
        'guide/quickstart',
        'guide/builder',
        'guide/chat',
        'guide/tools-and-skills',
        'guide/memory',
      ],
    },
    {
      type: 'category',
      label: 'Channels',
      collapsed: false,
      items: [
        'channels/telegram',
        'channels/discord',
        'channels/webhook',
      ],
    },
    {
      type: 'category',
      label: 'Reference',
      items: ['reference/configuration'],
    },
    'releases/index',
  ],
}

export default sidebars
