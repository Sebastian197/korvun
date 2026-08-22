import Link from '@docusaurus/Link'
import useBaseUrl from '@docusaurus/useBaseUrl'
import type { LandingCopy } from './landingCopy'
import styles from './landing.module.css'

export function Demo({ copy }: { copy: LandingCopy['demo'] }) {
  const poster = useBaseUrl('/media/gateway-demo-poster.jpg')
  const video = useBaseUrl('/media/gateway-demo.mp4')

  return (
    <section className={styles.section} data-k-section="demo">
      <div className={styles.shell}>
        <div className={styles.demoHeader}>
          <div className={styles.sectionIntro} data-motion>
            <p className={styles.kicker}>{copy.kicker}</p>
            <h2>{copy.title}</h2>
            <p>{copy.body}</p>
          </div>
          <Link
            className={styles.textLink}
            href="https://github.com/Sebastian197/korvun/releases/tag/v0.9.0"
          >
            {copy.release} <span aria-hidden="true">↗</span>
          </Link>
        </div>

        <div className={styles.videoFrame} data-motion>
          <div className={styles.videoChrome} aria-hidden="true">
            <span>korvun.desktop</span>
            <span>01:00</span>
          </div>
          <video controls playsInline preload="metadata" poster={poster} aria-label={copy.ariaLabel}>
            <source src={video} type="video/mp4" />
          </video>
        </div>
      </div>
    </section>
  )
}
