import Link from '@docusaurus/Link'
import { useState } from 'react'
import type { LandingCopy } from './landingCopy'
import styles from './landing.module.css'

const installCommands = `VERSION=0.8.0
curl -LO https://github.com/Sebastian197/korvun/releases/download/v\${VERSION}/korvun_\${VERSION}_linux_amd64.tar.gz
curl -LO https://github.com/Sebastian197/korvun/releases/download/v\${VERSION}/checksums.txt
sha256sum --check checksums.txt --ignore-missing
tar -xzf korvun_\${VERSION}_linux_amd64.tar.gz
./korvun version`

export function InstallProof({
  copy,
  installGuide,
}: {
  copy: LandingCopy['install']
  installGuide: string
}) {
  const [copied, setCopied] = useState(false)

  const copyCommands = async () => {
    await navigator.clipboard.writeText(installCommands)
    setCopied(true)
    window.setTimeout(() => setCopied(false), 1800)
  }

  return (
    <section className={styles.section} data-k-section="install">
      <div className={styles.shell}>
        <div className={styles.sectionIntro} data-motion>
          <p className={styles.kicker}>{copy.kicker}</p>
          <h2>{copy.title}</h2>
          <p>{copy.body}</p>
        </div>

        <div className={styles.installGrid}>
          <div className={styles.proofList}>
            {copy.proof.map((item, index) => (
              <div className={styles.proofItem} key={item} data-motion>
                <span>{String(index + 1).padStart(2, '0')}</span>
                <strong>{item}</strong>
              </div>
            ))}
            <Link className={styles.textLink} to={installGuide}>
              {copy.guide} <span aria-hidden="true">→</span>
            </Link>
          </div>

          <div className={`k-terminal ${styles.terminal}`}>
            <div className={styles.terminalBar}>
              <div aria-hidden="true" className={styles.terminalDots}>
                <span />
                <span />
                <span />
              </div>
              <span>korvun · install</span>
              <button type="button" onClick={copyCommands}>
                {copied ? copy.copied : copy.copy}
              </button>
            </div>
            <pre aria-label={copy.terminalAria} role="region" tabIndex={0}>
              <code>{installCommands}</code>
            </pre>
            <div className={styles.terminalStatus}>
              <span>✓ SHA-256</span>
              <span>✓ cosign</span>
              <span>✓ SBOM</span>
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}
