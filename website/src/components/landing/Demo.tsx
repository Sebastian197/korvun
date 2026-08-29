import Link from "@docusaurus/Link";
import useBaseUrl from "@docusaurus/useBaseUrl";
import type { LandingCopy } from "./landingCopy";
import styles from "./landing.module.css";

export function Demo({ copy }: { copy: LandingCopy["demo"] }) {
  const poster = useBaseUrl("/media/gateway-demo-poster.jpg");
  const video = useBaseUrl("/media/gateway-demo.mp4");

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
            href="https://github.com/Sebastian197/korvun/releases/tag/v0.9.2"
          >
            {copy.release} <span aria-hidden="true">↗</span>
          </Link>
        </div>

        {/* Frame + port on the wrapper: the video frame clips overflow. */}
        <div className={styles.journeyTarget} data-motion>
          <span
            aria-hidden="true"
            data-k-route-port
            className={styles.routePort}
          />
          <div className={styles.videoFrame}>
            <div className={styles.videoChrome} aria-hidden="true">
              <span>korvun.desktop</span>
              <span>01:00</span>
            </div>
            <video
              controls
              playsInline
              preload="metadata"
              poster={poster}
              aria-label={copy.ariaLabel}
            >
              <source src={video} type="video/mp4" />
            </video>
          </div>
        </div>
      </div>
    </section>
  );
}
