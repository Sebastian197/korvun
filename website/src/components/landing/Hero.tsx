import Link from '@docusaurus/Link'
import useBaseUrl from '@docusaurus/useBaseUrl'
import type { LandingCopy } from './landingCopy'
import styles from './landing.module.css'

type Props = {
  copy: LandingCopy['hero']
  quickstart: string
}

export function Hero({ copy, quickstart }: Props) {
  const mark = useBaseUrl('/brand/korvun-logo-hero.svg')

  return (
    <section className={styles.hero} data-k-section="hero">
      <div className={styles.heroGlow} aria-hidden="true" />
      <div className={styles.heroGrid} aria-hidden="true" />
      <div className={styles.shell}>
        <div className={styles.heroLayout}>
          <div className={styles.heroContent} data-motion>
            <p className={styles.eyebrow}>
              <span className={styles.liveDot} aria-hidden="true" />
              {copy.eyebrow}
            </p>
            <h1 className={styles.heroTitle}>
              <span>{copy.lines[0]}</span>
              <span>{copy.lines[1]}</span>
              <span className={styles.gradientText}>{copy.lines[2]}</span>
            </h1>
            <p className={styles.heroBody}>{copy.body}</p>
            <div className={styles.actions}>
              <Link className={styles.primaryButton} to={quickstart}>
                {copy.primary}
                <span aria-hidden="true">→</span>
              </Link>
              <Link
                className={styles.secondaryButton}
                href="https://github.com/Sebastian197/korvun"
              >
                {copy.secondary}
                <span aria-hidden="true">↗</span>
              </Link>
            </div>
            <div className={styles.releaseLine} aria-label={`${copy.releaseLabel} v0.9.0 — Beta`}>
              <span>{copy.releaseLabel}</span>
              <strong>v0.9.0 — Beta</strong>
              <span>Apache-2.0</span>
            </div>
          </div>

          <div className={styles.heroMarkWrap} data-motion aria-hidden="true">
            <div className={styles.heroOrbit} />
            <img className={styles.heroMark} src={mark} alt="" width="640" height="640" />
            <div className={styles.heroBadge}>01 · 08</div>
          </div>
        </div>
      </div>
      <div className={styles.heroRail} aria-hidden="true">
        <span>TELEGRAM</span>
        <span>DISCORD</span>
        <span>WEBHOOK</span>
        <span>OLLAMA</span>
        <span>GROQ</span>
      </div>
    </section>
  )
}
