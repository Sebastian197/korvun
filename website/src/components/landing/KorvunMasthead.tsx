import styles from './landing.module.css'

// The cinematic A1 masthead (brand-motion spec, Task 2): the governed K
// with two orbits, three input signals, one decision, one output, and four
// deterministic particles — the mockup's exact geometry, rendered inline so
// CSS layers can animate it. Decorative on purpose (aria-hidden): the
// adjacent H1 carries the message. All animation is gated on the
// data-k-running attribute armBrandMotion maintains, and every layer
// animates transform/opacity/filter only.
export function KorvunMasthead() {
  return (
    <svg
      className={styles.masthead}
      data-k-masthead
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
        data-layer="route-bases"
        fill="none"
        stroke="#FFFFFF"
        strokeOpacity="0.14"
        strokeWidth="1.2"
        strokeLinecap="round"
      >
        <path d="M18 142C92 142 126 166 204 180" />
        <path d="M18 220H202" />
        <path d="M18 298C92 298 126 274 204 260" />
      </g>
      <g data-layer="input-signals">
        <circle className={`${styles.mastheadPacket} ${styles.mastheadPacketTop}`} cx="18" cy="142" r="4" fill="#2BC8B7" />
        <circle className={`${styles.mastheadPacket} ${styles.mastheadPacketMid}`} cx="18" cy="220" r="4" fill="#69AECF" />
        <circle className={`${styles.mastheadPacket} ${styles.mastheadPacketLow}`} cx="18" cy="298" r="4" fill="#7A5AF5" />
      </g>
      <g data-layer="input-nodes">
        <circle cx="18" cy="142" r="6" fill="#2BC8B7" />
        <circle cx="18" cy="220" r="6" fill="#69AECF" />
        <circle cx="18" cy="298" r="6" fill="#7A5AF5" />
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
        <circle className={styles.mastheadDecision} data-layer="decision" cx="374" cy="220" r="8" fill="#FFFFFF" />
      </g>
      <path
        data-layer="output-signal"
        d="M374 220C444 220 477 197 542 174"
        fill="none"
        stroke="#F4F4F6"
        strokeOpacity="0.14"
        strokeWidth="1.2"
        strokeLinecap="round"
      />
      <circle className={styles.mastheadOut} cx="374" cy="220" r="4" fill="#F4F4F6" />
      <circle data-layer="output-node" cx="542" cy="174" r="5" fill="#F4F4F6" />
      <g data-layer="particles">
        <circle className={styles.mastheadParticle} cx="90" cy="102" r="4" fill="#2BC8B7" />
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
  )
}
