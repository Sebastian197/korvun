import Head from '@docusaurus/Head'
import useDocusaurusContext from '@docusaurus/useDocusaurusContext'
import Layout from '@theme/Layout'
import { LandingPage } from '../components/landing/LandingPage'
import { landingCopy } from '../components/landing/landingCopy'

export default function Home() {
  const { i18n } = useDocusaurusContext()
  const locale = i18n.currentLocale === 'es' ? 'es' : 'en'
  const copy = landingCopy[locale]

  return (
    <Layout title="Korvun" description={copy.metaDescription}>
      <Head>
        <meta property="og:description" content={copy.metaDescription} />
        <meta name="twitter:description" content={copy.metaDescription} />
      </Head>
      <LandingPage copy={copy} locale={locale} />
    </Layout>
  )
}
