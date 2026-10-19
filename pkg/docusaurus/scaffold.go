package docusaurus

import (
	"fmt"
	"os"
	"path/filepath"
)

// Scaffold writes the core configuration and layout files for Docusaurus v3.
func Scaffold(targetDir string, cfg *SiteConfig) error {
	if cfg == nil {
		return ErrInvalidConfig
	}
	if err := createDirectories(targetDir); err != nil {
		return fmt.Errorf("%w: failed to create directories: %v", ErrScaffoldFailed, err)
	}

	files := map[string]string{
		"package.json":              renderPackageJSON(cfg),
		"docusaurus.config.ts":      renderDocusaurusConfig(cfg),
		"sidebars.ts":               renderSidebarsConfig(),
		"src/css/custom.css":        renderCustomCSS(),
		"src/pages/index.tsx":       renderIndexPage(cfg),
		"src/pages/share.tsx":       renderSharePage(cfg),
		"Dockerfile":                renderDockerfile(),
		"docker-compose.share.yaml": renderDockerCompose(cfg),
		"nginx.conf":                renderNginxConf(),
		"static/img/logo.svg":       renderLogoSVG(),
		".gitignore":                renderGitignore(),
	}

	for relPath, content := range files {
		fullPath := filepath.Join(targetDir, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return fmt.Errorf("%w: cannot create dir for %s: %v", ErrScaffoldFailed, relPath, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("%w: cannot write %s: %v", ErrScaffoldFailed, relPath, err)
		}
	}

	return nil
}

func createDirectories(baseDir string) error {
	dirs := []string{
		filepath.Join(baseDir, "docs"),
		filepath.Join(baseDir, "src", "pages"),
		filepath.Join(baseDir, "src", "css"),
		filepath.Join(baseDir, "static", "img"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	return nil
}

func renderPackageJSON(cfg *SiteConfig) string {
	return fmt.Sprintf(`{
  "name": "%s-docs",
  "version": "1.0.0",
  "private": true,
  "scripts": {
    "docusaurus": "docusaurus",
    "start": "docusaurus start --port %d",
    "build": "docusaurus build",
    "swizzle": "docusaurus swizzle",
    "deploy": "docusaurus deploy",
    "clear": "docusaurus clear",
    "serve": "docusaurus serve --port %d"
  },
  "dependencies": {
    "@docusaurus/core": "^3.5.2",
    "@docusaurus/preset-classic": "^3.5.2",
    "@docusaurus/theme-mermaid": "^3.5.2",
    "@mdx-js/react": "^3.0.0",
    "clsx": "^2.0.0",
    "prism-react-renderer": "^2.3.0",
    "react": "^18.2.0",
    "react-dom": "^18.2.0"
  },
  "devDependencies": {
    "@docusaurus/module-type-aliases": "^3.5.2",
    "@docusaurus/types": "^3.5.2",
    "typescript": "^5.2.2"
  },
  "browserslist": {
    "production": [
      ">0.5%%",
      "not dead",
      "not op_mini all"
    ],
    "development": [
      "last 3 chrome version",
      "last 3 firefox version",
      "last 5 safari version"
    ]
  }
}
`, cfg.ProjectName, cfg.Port, cfg.Port)
}

func renderDocusaurusConfig(cfg *SiteConfig) string {
	return fmt.Sprintf(`import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

const config: Config = {
  title: '%s',
  tagline: '%s',
  favicon: '%s',
  url: '%s',
  baseUrl: '%s',
  organizationName: '%s',
  projectName: '%s',
  onBrokenLinks: 'warn',
  onBrokenMarkdownLinks: 'warn',
  markdown: {
    mermaid: %t,
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
      title: '%s',
      logo: {
        alt: '%s Logo',
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
          href: '%s',
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
            {label: 'Repository', href: '%s'},
          ],
        },
      ],
      copyright: 'Copyright © ' + new Date().getFullYear() + ' %s. Built with Dendrite Docusaurus Engine.',
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
      additionalLanguages: ['bash', 'json', 'yaml', 'go', 'rust', 'python', 'docker'],
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
`, cfg.Title, cfg.Tagline, cfg.Favicon, cfg.URL, cfg.BaseURL, cfg.OrganizationName, cfg.ProjectName,
		cfg.EnableMermaid, cfg.Title, cfg.Title, cfg.GitRepo, cfg.GitRepo, cfg.Title)
}

func renderSidebarsConfig() string {
	return `import type {SidebarsConfig} from '@docusaurus/types';

const sidebars: SidebarsConfig = {
  docsSidebar: [
    {
      type: 'autogenerated',
      dirName: '.',
    },
  ],
};

export default sidebars;
`
}

func renderCustomCSS() string {
	return `:root {
  --ifm-color-primary: #10b981;
  --ifm-color-primary-dark: #059669;
  --ifm-color-primary-darker: #047857;
  --ifm-color-primary-darkest: #065f46;
  --ifm-color-primary-light: #34d399;
  --ifm-color-primary-lighter: #6ee7b7;
  --ifm-color-primary-lightest: #a7f3d0;
  --ifm-code-font-size: 92%;
  --docusaurus-highlighted-code-line-bg: rgba(16, 185, 129, 0.1);
}

[data-theme='dark'] {
  --ifm-color-primary: #34d399;
  --ifm-color-primary-dark: #10b981;
  --ifm-color-primary-darker: #059669;
  --ifm-color-primary-darkest: #047857;
  --ifm-color-primary-light: #6ee7b7;
  --ifm-color-primary-lighter: #a7f3d0;
  --ifm-color-primary-lightest: #d1fae5;
  --ifm-background-color: #0f172a;
  --ifm-background-surface-color: #1e293b;
}

.hero--primary {
  background: linear-gradient(135deg, #064e3b 0%, #0f172a 100%);
  color: #f8fafc;
}

.share-card {
  border: 1px solid var(--ifm-color-emphasis-200);
  border-radius: 12px;
  padding: 1.5rem;
  background: var(--ifm-card-background-color);
  transition: transform 0.2s ease, box-shadow 0.2s ease;
}

.share-card:hover {
  transform: translateY(-4px);
  box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.15);
}

.share-badge {
  display: inline-block;
  padding: 0.25rem 0.75rem;
  font-size: 0.8rem;
  font-weight: 600;
  border-radius: 9999px;
  background: var(--ifm-color-primary);
  color: #0f172a;
}
`
}

func renderIndexPage(cfg *SiteConfig) string {
	return fmt.Sprintf(`import React from 'react';
import clsx from 'clsx';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';

function HeroBanner() {
  const {siteConfig} = useDocusaurusContext();
  return (
    <header className={clsx('hero hero--primary')}>
      <div className="container" style={{textAlign: 'center', padding: '4rem 1rem'}}>
        <h1 className="hero__title">{siteConfig.title}</h1>
        <p className="hero__subtitle" style={{maxWidth: '800px', margin: '1rem auto 2rem'}}>
          {siteConfig.tagline}
        </p>
        <div style={{display: 'flex', gap: '1rem', justifyContent: 'center', flexWrap: 'wrap'}}>
          <Link className="button button--secondary button--lg" to="/docs/overview">
            Explore Documentation 📚
          </Link>
          <Link className="button button--primary button--lg" to="/share">
            Project Sharing Hub 🌐
          </Link>
        </div>
      </div>
    </header>
  );
}

const FeatureList = [
  {
    title: 'Autonomous Coding Agents',
    description: 'Harness for Aider, OpenHands, OpenCode, and AGY with token-budget enforcement and sandbox isolation.',
  },
  {
    title: 'Distributed Codebase Ecosystem',
    description: 'Priority-tiered capability discovery, modular REA decomposition, and cross-project federated linking.',
  },
  {
    title: 'Docusaurus Docs-as-Code & Sharing',
    description: 'Automated documentation synthesis, interactive sharing portal, and portable cryptographic manifest bundles.',
  },
  {
    title: 'JetBrains IDE & MCP Integration',
    description: 'Deep IDE bridge with 20+ MCP tools for inspections, PSI search, run configs, and autonomous loops.',
  },
];

export default function Home(): JSX.Element {
  return (
    <Layout title="%s" description="%s">
      <HeroBanner />
      <main>
        <section style={{padding: '4rem 0'}}>
          <div className="container">
            <div className="row">
              {FeatureList.map((props, idx) => (
                <div key={idx} className={clsx('col col--3')} style={{marginBottom: '2rem'}}>
                  <div className="share-card" style={{height: '100%%'}}>
                    <h3>{props.title}</h3>
                    <p>{props.description}</p>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </section>
      </main>
    </Layout>
  );
}
`, cfg.Title, cfg.Tagline)
}

func renderSharePage(cfg *SiteConfig) string {
	return fmt.Sprintf(`import React from 'react';
import Layout from '@theme/Layout';
import Link from '@docusaurus/Link';

export default function ShareHub(): JSX.Element {
  return (
    <Layout title="Project Sharing Hub" description="Dendrite project distribution and ecosystem sharing hub">
      <div className="container" style={{padding: '3rem 1rem'}}>
        <div style={{textAlign: 'center', marginBottom: '3rem'}}>
          <span className="share-badge">Tier: %s</span>
          <h1 style={{marginTop: '1rem'}}>Project Sharing & Collaboration Hub</h1>
          <p style={{fontSize: '1.2rem', color: 'var(--ifm-color-emphasis-700)', maxWidth: '750px', margin: '0 auto'}}>
            Facilitate cross-project knowledge transfer, portable documentation distribution, and federated codebase interoperability.
          </p>
        </div>

        <div className="row">
          <div className="col col--6" style={{marginBottom: '2rem'}}>
            <div className="share-card">
              <h2>📦 Standalone Documentation Bundle</h2>
              <p>
                Export an offline, portable archive containing the complete Docs-as-Code suite and cryptographic SHA-256 manifest.
              </p>
              <pre style={{padding: '1rem', borderRadius: '8px'}}>
                <code>dendrite codex site share --out dendrite-docs.tar.gz</code>
              </pre>
              <p style={{fontSize: '0.9rem', color: 'var(--ifm-color-emphasis-600)'}}>
                Verify integrity via <code>sharing-manifest.json</code>.
              </p>
            </div>
          </div>

          <div className="col col--6" style={{marginBottom: '2rem'}}>
            <div className="share-card">
              <h2>🐳 Containerized Intranet Sharing</h2>
              <p>
                Deploy this documentation site in 10 seconds via Docker or Docker Compose for team-wide intranet access.
              </p>
              <pre style={{padding: '1rem', borderRadius: '8px'}}>
                <code>docker compose -f docker-compose.share.yaml up -d</code>
              </pre>
              <p style={{fontSize: '0.9rem', color: 'var(--ifm-color-emphasis-600)'}}>
                Exposes high-speed Nginx HTTP service on port %d.
              </p>
            </div>
          </div>
        </div>

        <div className="row" style={{marginTop: '1rem'}}>
          <div className="col col--12">
            <div className="share-card">
              <h2>🔗 Distributed Codebase Federation</h2>
              <p>
                Connects across multiple repositories and partner projects:
              </p>
              <ul>
                <li><strong>Docula / Nayapima ERP</strong>: Integrated enterprise schemas and business specs.</li>
                <li><strong>semdag / Oxipix</strong>: High-performance visual and compute engine libraries.</li>
                <li><strong>Plane v2 / Jira</strong>: Live authoritative ticket tracing and milestone synchronization.</li>
              </ul>
              <div style={{marginTop: '1.5rem'}}>
                <Link className="button button--primary" to="/docs/project-sharing/federated-ecosystem">
                  Read Federation Guide →
                </Link>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Layout>
  );
}
`, cfg.SharingTier, cfg.Port)
}

func renderDockerfile() string {
	return `FROM node:20-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci || npm install
COPY . .
RUN npm run build

FROM nginx:alpine
COPY --from=builder /app/build /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
`
}

func renderDockerCompose(cfg *SiteConfig) string {
	return fmt.Sprintf(`version: '3.8'

services:
  docs-share:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: %s-docs-portal
    ports:
      - "%d:80"
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:80/"]
      interval: 10s
      timeout: 3s
      retries: 3
`, cfg.ProjectName, cfg.Port)
}

func renderNginxConf() string {
	return `server {
    listen 80;
    server_name localhost;

    location / {
        root /usr/share/nginx/html;
        index index.html index.htm;
        try_files $uri $uri/ /index.html;
    }

    gzip on;
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml application/xml+rss text/javascript;
}
`
}

func renderLogoSVG() string {
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100" width="100" height="100">
  <circle cx="50" cy="50" r="45" fill="#0f172a" stroke="#10b981" stroke-width="4"/>
  <circle cx="50" cy="50" r="12" fill="#10b981"/>
  <circle cx="25" cy="35" r="7" fill="#34d399"/>
  <circle cx="75" cy="35" r="7" fill="#34d399"/>
  <circle cx="30" cy="70" r="7" fill="#34d399"/>
  <circle cx="70" cy="70" r="7" fill="#34d399"/>
  <line x1="50" y1="50" x2="25" y2="35" stroke="#10b981" stroke-width="3"/>
  <line x1="50" y1="50" x2="75" y2="35" stroke="#10b981" stroke-width="3"/>
  <line x1="50" y1="50" x2="30" y2="70" stroke="#10b981" stroke-width="3"/>
  <line x1="50" y1="50" x2="70" y2="70" stroke="#10b981" stroke-width="3"/>
</svg>
`
}

func renderGitignore() string {
	return `node_modules/
build/
.docusaurus/
.cache-loader/
*.log
.DS_Store
dendrite-docs.tar.gz
`
}
