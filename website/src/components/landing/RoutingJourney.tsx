import styles from "./landing.module.css";

// The decorative routing layer (brand-motion spec, Task 4): one absolute
// SVG below interactive content. The base path and the active overlay share
// a geometry the controller writes at measure time (init + resize only) —
// rail plus rounded orthogonal elbows to each section's port; the overlay
// is revealed by a clip RECTANGLE scaled with --k-route-progress (a
// transform, per the motion whitelist) and the signal head is a direct
// transform write. Reduced motion styles the complete static circuit and
// hides the signal (data-k-static, set by the controller).
export function RoutingJourney() {
  return (
    <div className={styles.journey} data-k-journey aria-hidden="true">
      <svg
        className={styles.journeySvg}
        preserveAspectRatio="none"
        focusable="false"
      >
        <defs>
          <linearGradient id="k-route-gradient" x1="0" y1="0" x2="1" y2="1">
            <stop stopColor="#2BC8B7" />
            <stop offset="1" stopColor="#7A5AF5" />
          </linearGradient>
          <clipPath id="k-route-clip">
            <rect data-k-route-clip x="0" y="0" width="100%" height="100%" />
          </clipPath>
        </defs>
        <path data-k-route-path-base className={styles.journeyBase} d="" />
        <g clipPath="url(#k-route-clip)">
          <path
            data-k-route-path-active
            className={styles.journeyActive}
            d=""
          />
        </g>
      </svg>
      <span className={styles.journeySignal} data-k-route-signal />
    </div>
  );
}
