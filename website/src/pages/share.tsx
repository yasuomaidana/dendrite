import React from 'react';
import Layout from '@theme/Layout';
import Link from '@docusaurus/Link';

export default function ShareHub(): JSX.Element {
  return (
    <Layout title="Project Sharing Hub" description="Dendrite project distribution and ecosystem sharing hub">
      <div className="container" style={{padding: '3rem 1rem'}}>
        <div style={{textAlign: 'center', marginBottom: '3rem'}}>
          <span className="share-badge">Tier: federated</span>
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
                <code>dendrite doc docusaurus share --out dendrite-docs.tar.gz</code>
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
                Exposes high-speed Nginx HTTP service on port 3000.
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
