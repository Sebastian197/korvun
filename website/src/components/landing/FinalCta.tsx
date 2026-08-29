import Link from "@docusaurus/Link";
import type { LandingCopy } from "./landingCopy";
import styles from "./landing.module.css";

export function FinalCta({
  copy,
  quickstart,
}: {
  copy: LandingCopy["final"];
  quickstart: string;
}) {
  return (
    <section
      className={`${styles.section} ${styles.final}`}
      data-k-section="final"
    >
      <div className={styles.shell}>
        <div
          className={`${styles.finalInner} ${styles.journeyTarget}`}
          data-motion
        >
          <span
            aria-hidden="true"
            data-k-route-port
            className={`${styles.routePort} ${styles.portFinal}`}
          />
          <p className={styles.kicker}>{copy.kicker}</p>
          <h2>{copy.title}</h2>
          <p>{copy.body}</p>
          <div className={styles.actions}>
            <Link className={styles.primaryButton} to={quickstart}>
              {copy.primary} <span aria-hidden="true">→</span>
            </Link>
            <Link
              className={styles.secondaryButton}
              href="https://github.com/Sebastian197/korvun"
            >
              {copy.secondary} <span aria-hidden="true">↗</span>
            </Link>
          </div>
        </div>
      </div>
    </section>
  );
}
