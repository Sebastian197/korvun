import "@fontsource/geist-sans/400.css";
import "@fontsource/geist-sans/500.css";
import "@fontsource/geist-sans/600.css";
import "@fontsource/geist-sans/700.css";
import "@fontsource/geist-mono/400.css";
import "@fontsource/geist-mono/500.css";
import type { ReactNode } from "react";

// The landing arms its own motion controllers on mount (LandingPage.tsx):
// Docusaurus keeps the previous page's DOM alive while a navigation's chunk
// loads, so any Root-level arming keyed on route or children measures the
// WRONG page. Root only carries the font faces.
export default function Root({ children }: { children: ReactNode }) {
  return children;
}
