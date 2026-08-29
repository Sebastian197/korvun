import styles from "./landing.module.css";

// The cinematic A1 masthead, transplanted from the approved mockup
// (design-drafts/korvun-brand-motion/mockup/index.html) — its geometry, its
// layers and its 3.1s choreography are the pixel contract. Two invisible
// adaptations keep the motion gate intact (transform/opacity/filter only):
// the flowing input dashes are sprites translated along points sampled from
// the mockup's own Béziers (stroke-dashoffset is banned), and the output
// line draws through a mask rectangle that scales on X instead of animating
// stroke-dasharray. Decorative on purpose (aria-hidden): the H1 carries the
// message. Everything is paused until armBrandMotion sets data-k-running on
// the wrapping masthead container.
export function KorvunMasthead() {
  const dash = "M-9 0H9";
  return (
    <svg
      className={styles.mastheadSvg}
      viewBox="0 0 560 440"
      aria-hidden="true"
      focusable="false"
    >
      <defs>
        <linearGradient
          id="k-masthead-gradient"
          x1="152"
          y1="92"
          x2="405"
          y2="347"
          gradientUnits="userSpaceOnUse"
        >
          <stop stopColor="#2BC8B7" />
          <stop offset="1" stopColor="#7A5AF5" />
        </linearGradient>
        <mask id="k-masthead-mask">
          <rect x="178" y="86" width="230" height="268" rx="58" fill="white" />
          <path
            d="M254 150V290M350 154L290 220L350 286"
            fill="none"
            stroke="black"
            strokeWidth="29"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </mask>
        <clipPath id="k-flow-clip-top">
          <rect x="18" y="130" width="186" height="60" />
        </clipPath>
        <clipPath id="k-flow-clip-mid">
          <rect x="18" y="212" width="184" height="16" />
        </clipPath>
        <clipPath id="k-flow-clip-low">
          <rect x="18" y="250" width="186" height="60" />
        </clipPath>
        <mask
          id="k-out-mask"
          maskUnits="userSpaceOnUse"
          x="0"
          y="0"
          width="560"
          height="440"
        >
          <rect
            className={styles.mastheadOutReveal}
            x="372"
            y="165"
            width="182"
            height="64"
            fill="white"
          />
        </mask>
      </defs>
      <g className={styles.mastheadOrbit} data-layer="orbits">
        <ellipse
          cx="294"
          cy="220"
          rx="238"
          ry="128"
          transform="rotate(-18 294 220)"
          fill="none"
          stroke="#7A5AF5"
          strokeOpacity="0.34"
          strokeWidth="1.5"
          strokeDasharray="8 8"
        />
      </g>
      <g className={`${styles.mastheadOrbit} ${styles.mastheadOrbitReverse}`}>
        <ellipse
          cx="294"
          cy="220"
          rx="192"
          ry="174"
          transform="rotate(24 294 220)"
          fill="none"
          stroke="#2BC8B7"
          strokeOpacity="0.22"
          strokeWidth="1.5"
          strokeDasharray="8 8"
        />
      </g>
      <g
        data-layer="input-signals"
        fill="none"
        strokeWidth="3"
        strokeLinecap="round"
      >
        <g clipPath="url(#k-flow-clip-top)" stroke="#2BC8B7">
          <path className={styles.flowTop} d={dash} />
          <path className={`${styles.flowTop} ${styles.flowSecond}`} d={dash} />
        </g>
        <g clipPath="url(#k-flow-clip-mid)" stroke="#69AECF">
          <path className={styles.flowMid} d={dash} />
          <path
            className={`${styles.flowMid} ${styles.flowMidSecond}`}
            d={dash}
          />
        </g>
        <g clipPath="url(#k-flow-clip-low)" stroke="#7A5AF5">
          <path className={styles.flowLow} d={dash} />
          <path
            className={`${styles.flowLow} ${styles.flowLowSecond}`}
            d={dash}
          />
        </g>
      </g>
      <g className={styles.mastheadTilt} data-layer="tilting-mark">
        <rect
          data-layer="governed-k"
          x="178"
          y="86"
          width="230"
          height="268"
          rx="58"
          fill="url(#k-masthead-gradient)"
          mask="url(#k-masthead-mask)"
        />
        <circle
          className={styles.mastheadDecision}
          data-layer="decision"
          cx="374"
          cy="220"
          r="8"
          fill="#FFFFFF"
        />
      </g>
      <g mask="url(#k-out-mask)">
        <path
          data-layer="output-signal"
          d="M374 220C444 220 477 197 542 174"
          fill="none"
          stroke="#F4F4F6"
          strokeWidth="3"
          strokeLinecap="round"
        />
      </g>
      <g data-layer="particles">
        <circle
          className={styles.mastheadParticle}
          cx="90"
          cy="102"
          r="4"
          fill="#2BC8B7"
        />
        <circle
          className={`${styles.mastheadParticle} ${styles.mastheadParticle2}`}
          cx="473"
          cy="112"
          r="5"
          fill="#7A5AF5"
        />
        <circle
          className={`${styles.mastheadParticle} ${styles.mastheadParticle3}`}
          cx="462"
          cy="330"
          r="3"
          fill="#69AECF"
        />
        <circle
          className={`${styles.mastheadParticle} ${styles.mastheadParticle4}`}
          cx="123"
          cy="354"
          r="4"
          fill="#8C71F8"
        />
      </g>
    </svg>
  );
}
