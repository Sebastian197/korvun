import Link from '@docusaurus/Link'
import useBaseUrl from '@docusaurus/useBaseUrl'
import type { LandingCopy } from './landingCopy'
import styles from './landing.module.css'

export function Demo({ copy }: { copy: LandingCopy['demo'] }) {
  const poster = useBaseUrl('/media/korvun-v060-clip-poster.jpg')
  const video = useBaseUrl('/media/korvun-v060-clip-1920x1080.mp4')

  return (
    <section className={styles.section} data-k-section="demo">
      <div className={styles.shell}>
        <div className={styles.demoHeader}>
          <div className={styles.sectionIntro}>
            <p className={styles.kicker}>{copy.kicker}</p>
            <h2>{copy.title}</h2>
            <p>{copy.body}</p>
          </div>
          <Link
            className={styles.textLink}
            href="https://github.com/Sebastian197/korvun/releases/tag/v0.6.0"
          >
            {copy.release} <span aria-hidden="true">↗</span>
          </Link>
        </div>

        <div className={styles.videoFrame} data-motion>
          <div className={styles.videoChrome} aria-hidden="true">
            <span>korvun.builder</span>
            <span>00:27</span>
          </div>
          <video controls playsInline preload="metadata" poster={poster} aria-label={copy.ariaLabel}>
            <source src={video} type="video/mp4" />
          </video>
        </div>
      </div>
    </section>
  )
}
