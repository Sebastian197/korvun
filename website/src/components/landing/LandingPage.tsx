import { Capabilities } from './Capabilities'
import { Demo } from './Demo'
import { FinalCta } from './FinalCta'
import { Hero } from './Hero'
import { InstallProof } from './InstallProof'
import type { LandingCopy } from './landingCopy'
import { PrivacyDiagram } from './PrivacyDiagram'
import { RoutingJourney } from './RoutingJourney'
import styles from './landing.module.css'

export function LandingPage({ copy }: { copy: LandingCopy }) {
  return (
    <main className={styles.page}>
      <RoutingJourney />
      <Hero copy={copy.hero} quickstart="/guide/quickstart" />
      <InstallProof copy={copy.install} installGuide="/guide/install" />
      <Capabilities copy={copy.capabilities} />
      <PrivacyDiagram copy={copy.privacy} reference="/reference/configuration" />
      <Demo copy={copy.demo} />
      <FinalCta copy={copy.final} quickstart="/guide/quickstart" />
    </main>
  )
}
