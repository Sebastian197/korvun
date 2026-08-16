import { Capabilities } from './Capabilities'
import { Demo } from './Demo'
import { FinalCta } from './FinalCta'
import { Hero } from './Hero'
import { InstallProof } from './InstallProof'
import type { LandingCopy } from './landingCopy'
import { PrivacyDiagram } from './PrivacyDiagram'
import styles from './landing.module.css'

export function LandingPage({
  copy,
  locale,
}: {
  copy: LandingCopy
  locale: 'en' | 'es'
}) {
  const localize = (route: string) => (locale === 'es' ? `/es${route}` : route)

  return (
    <main className={styles.page}>
      <Hero copy={copy.hero} quickstart={localize('/guide/quickstart')} />
      <InstallProof copy={copy.install} installGuide={localize('/guide/install')} />
      <Capabilities copy={copy.capabilities} localize={localize} />
      <PrivacyDiagram
        copy={copy.privacy}
        reference={localize('/reference/configuration')}
      />
      <Demo copy={copy.demo} />
      <FinalCta copy={copy.final} quickstart={localize('/guide/quickstart')} />
    </main>
  )
}
