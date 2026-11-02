import React from 'react';
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
    <Layout title="Dendrite Documentation" description="Autonomous AI Development & Distributed Infrastructure Control Plane">
      <HeroBanner />
      <main>
        <section style={{padding: '4rem 0'}}>
          <div className="container">
            <div className="row">
              {FeatureList.map((props, idx) => (
                <div key={idx} className={clsx('col col--3')} style={{marginBottom: '2rem'}}>
                  <div className="share-card" style={{height: '100%'}}>
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
