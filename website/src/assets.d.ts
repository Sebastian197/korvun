// Docusaurus resolves font imports through its webpack fonts rule
// (url-loader → hashed URL under assets/fonts/), but ships no ambient
// type for them; this mirrors the pattern of its own asset declarations.
declare module '*.woff2' {
  const url: string
  export default url
}
