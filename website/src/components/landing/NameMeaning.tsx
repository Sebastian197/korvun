import type { LandingCopy } from "./landingCopy";
import styles from "./landing.module.css";

/* The governed K in characters (the A1 brand mark): three signals enter,
 * the kernel decides, one governed reply exits. Static and aria-hidden —
 * the rhyme text carries the meaning for assistive tech. */
const kMark = [
  "    ╭──────────╮",
  "●───┤  ██  ██  │",
  "●───┤  ████    │",
  "●───┤  ██  ██  ◆──▶ ●",
  "    ╰──────────╯",
].join("\n");

/* NameMeaning unfolds the acronym (approved variant A, 2026-08-30): the
 * word segmented into K / OR / VUN, three aligned readings, and the
 * governed K beside the rhyme line making the name↔logo rhyme explicit.
 * Content truth: the official acronym of the master document. Static
 * section — reveals ride the existing data-motion controller; the brand
 * line crosses it like every other [data-k-section]. */
export function NameMeaning({ copy }: { copy: LandingCopy["name"] }) {
  const segments = ["K", "OR", "VUN"] as const;
  return (
    <section className={styles.section} data-k-section="name">
      <div className={styles.shell}>
        <div className={styles.sectionIntro} data-motion>
          <p className={styles.kicker}>{copy.kicker}</p>
          <h2>{copy.title}</h2>
        </div>
        <p className={styles.nameWord} aria-hidden="true" data-motion>
          {segments.map((segment) => (
            <span key={segment} data-k-name-segment={segment.toLowerCase()}>
              {segment}
            </span>
          ))}
        </p>
        <div className={styles.nameReadings}>
          {copy.readings.map((reading) => (
            <div
              className={styles.nameReading}
              data-k-name-segment={reading.letters.toLowerCase()}
              key={reading.letters}
              data-motion
            >
              <h3>{reading.title}</h3>
              <p>{reading.body}</p>
            </div>
          ))}
        </div>
        <div className={styles.nameRhyme} data-motion>
          <pre className={styles.nameMark} aria-hidden="true">
            {kMark}
          </pre>
          <p>
            <strong>{copy.rhymeLead}</strong> {copy.rhymeBody}
          </p>
        </div>
      </div>
    </section>
  );
}
