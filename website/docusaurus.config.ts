import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

const config: Config = {
  title: 'Dendrite Documentation',
  tagline: 'Autonomous AI Development & Distributed Infrastructure Control Plane',
  favicon: 'img/favicon.ico',
  url: 'https://dendrite.local',
  baseUrl: '/',
  organizationName: 'dendrite',
  projectName: 'dendrite',
  onBrokenLinks: 'warn',
  onBrokenMarkdownLinks: 'warn',
  markdown: {
    mermaid: true,
    format: 'detect',
  },
  themes: ['@docusaurus/theme-mermaid'],
  presets: [
    [
      'classic',
      {
        docs: {
          sidebarPath: './sidebars.ts',
          routeBasePath: 'docs',
        },
        theme: {
          customCss: './src/css/custom.css',
        },
      } satisfies Preset.Options,
    ],
  ],
  themeConfig: {
    navbar: {
      title: 'Dendrite Documentation',
      logo: {
        alt: 'Dendrite Documentation Logo',
        src: 'img/logo.svg',
      },
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'docsSidebar',
          position: 'left',
          label: 'Documentation',
        },
        {
          to: '/share',
          label: 'Sharing Hub',
          position: 'left',
        },
        {
          href: 'https://github.com/yasuomaidana/Dendrite',
          label: 'GitHub',
          position: 'right',
        },
      ],
    },
    footer: {
      style: 'dark',
      links: [
        {
          title: 'Docs',
          items: [
            {label: 'Architecture', to: '/docs/architecture'},
            {label: 'Features', to: '/docs/features'},
            {label: 'Guides', to: '/docs/guides'},
          ],
        },
        {
          title: 'Project Sharing',
          items: [
            {label: 'Sharing Hub', to: '/share'},
            {label: 'Ecosystem & Federation', to: '/docs/project-sharing/federated-ecosystem'},
            {label: 'Containerized Deployment', to: '/docs/project-sharing/export-and-deployment'},
          ],
        },
        {
          title: 'Community & Tracking',
          items: [
            {label: 'Plane Tracker', href: 'http://localhost:8090'},
            {label: 'Repository', href: 'https://github.com/yasuomaidana/Dendrite'},
          ],
        },
      ],
      copyright: 'Copyright © ' + new Date().getFullYear() + ' Dendrite Documentation. Built with Dendrite Docusaurus Engine.',
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
      additionalLanguages: ['bash', 'json', 'yaml', 'go', 'rust', 'python', 'docker'],
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
