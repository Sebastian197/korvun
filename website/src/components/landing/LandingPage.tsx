import { useEffect } from "react";
import { armBrandMotion, armRoutingJourney } from "../../theme/brandMotion";
import { armStorytelling } from "../../theme/storytelling";
import { Capabilities } from "./Capabilities";
import { Demo } from "./Demo";
import { FinalCta } from "./FinalCta";
import { Hero } from "./Hero";
import { InstallProof } from "./InstallProof";
import { NameMeaning } from "./NameMeaning";
import type { LandingCopy } from "./landingCopy";
import { PrivacyDiagram } from "./PrivacyDiagram";
import { RoutingJourney } from "./RoutingJourney";
import styles from "./landing.module.css";

export function LandingPage({ copy }: { copy: LandingCopy }) {
  // The landing owns its motion controllers: arming on MOUNT is the only
  // moment that works for both a fresh load and a client-side navigation
  // (Docusaurus keeps the previous page's DOM alive while the next chunk
  // loads, so route- or children-keyed effects in Root measure the wrong
  // page). Cleanup on unmount removes every listener and attribute.
  useEffect(() => {
    const cleanups = [armStorytelling(), armBrandMotion(), armRoutingJourney()];
    return () => cleanups.forEach((cleanup) => cleanup());
  }, []);

  return (
    <main className={styles.page}>
      <RoutingJourney />
      <Hero copy={copy.hero} quickstart="/guide/quickstart" />
      <NameMeaning copy={copy.name} />
      <InstallProof copy={copy.install} installGuide="/guide/install" />
      <Capabilities copy={copy.capabilities} />
      <PrivacyDiagram
        copy={copy.privacy}
        reference="/reference/configuration"
      />
      <Demo copy={copy.demo} />
      <FinalCta copy={copy.final} quickstart="/guide/quickstart" />
      <div className={styles.status} aria-hidden="true">
        <span data-k-route-status>Routing · hero</span>
      </div>
    </main>
  );
}
