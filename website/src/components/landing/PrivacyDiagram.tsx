import Link from "@docusaurus/Link";
import type { LandingCopy } from "./landingCopy";
import styles from "./landing.module.css";

export function PrivacyDiagram({
  copy,
  reference,
}: {
  copy: LandingCopy["privacy"];
  reference: string;
}) {
  return (
    <section
      className={`${styles.section} ${styles.privacy}`}
      data-k-section="privacy"
    >
      <div className={styles.shell}>
        <div className={styles.privacyLayout}>
          <div className={styles.sectionIntro} data-motion>
            <p className={styles.kicker}>{copy.kicker}</p>
            <h2>{copy.title}</h2>
            <p>{copy.body}</p>
            <Link className={styles.textLink} to={reference}>
              {copy.reference} <span aria-hidden="true">→</span>
            </Link>
          </div>

          <div
            className={`${styles.routingDiagram} ${styles.journeyTarget}`}
            aria-label={copy.title}
            data-motion
          >
            <span
              aria-hidden="true"
              data-k-route-port
              className={`${styles.routePort} ${styles.portRight}`}
            />
            <div className={styles.routeRow}>
              <span className={styles.routeNode}>{copy.localLabel}</span>
              <span className={styles.routeLine} aria-hidden="true">
                <i />
              </span>
              <span className={`${styles.routeNode} ${styles.routeNodeActive}`}>
                <small>LOCAL</small>
                {copy.localModel}
              </span>
            </div>
            <div className={`${styles.routeRow} ${styles.routeRowExcluded}`}>
              <span className={styles.routeNode}>{copy.cloudLabel}</span>
              <span className={styles.routeLine} aria-hidden="true">
                <i />
              </span>
              <span className={styles.routeNode}>
                <small>CLOUD · {copy.excluded}</small>
                {copy.cloudModel}
              </span>
            </div>
            <div className={styles.policyStamp}>privacy: local_only</div>
          </div>
        </div>
      </div>
    </section>
  );
}
