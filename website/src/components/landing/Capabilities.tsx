import Link from '@docusaurus/Link'
import type { LandingCopy } from './landingCopy'
import styles from './landing.module.css'

export function Capabilities({ copy }: { copy: LandingCopy['capabilities'] }) {
  return (
    <section className={`${styles.section} ${styles.capabilities}`} data-k-section="capabilities">
      <div className={styles.shell}>
        <div className={styles.sectionIntro} data-motion>
          <p className={styles.kicker}>{copy.kicker}</p>
          <h2>{copy.title}</h2>
          <p>{copy.body}</p>
        </div>

        <div className={styles.capabilityGrid}>
          <span aria-hidden="true" data-k-route-port className={styles.routePort} />
          {copy.items.map((item) => (
            <article className={styles.capabilityCard} key={item.number} data-motion>
              <div className={styles.cardNumber}>{item.number}</div>
              <h3>{item.title}</h3>
              <p>{item.body}</p>
              <Link to={item.to}>
                {item.linkLabel} <span aria-hidden="true">↗</span>
              </Link>
            </article>
          ))}
        </div>
      </div>
    </section>
  )
}
