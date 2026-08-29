// Brand-motion runtime (brand-motion spec, Tasks 2–3): the masthead runs
// only while visible and motion is allowed, and the scroll journey drives
// route state from cached geometry inside one coalesced animation frame.
// Additive to armStorytelling — never touches the reveal-once contract.

type BrandMotionRuntime = {
  IntersectionObserver?: typeof IntersectionObserver;
  matchMedia: (query: string) => Pick<MediaQueryList, "matches">;
};

type BrandMotionRoot = Pick<Document, "querySelector">;

/** Arm the masthead visibility gate: `data-k-running` follows viewport
 * intersection; reduced motion never observes (the CSS static frame is the
 * complete experience); cleanup removes the attribute. */
export function armBrandMotion(
  root: BrandMotionRoot = document,
  runtime: BrandMotionRuntime = window,
): () => void {
  const masthead = root.querySelector<HTMLElement>("[data-k-masthead]");
  if (!masthead) return () => undefined;
  if (!runtime.matchMedia("(prefers-reduced-motion: no-preference)").matches) {
    return () => masthead.removeAttribute("data-k-running");
  }
  if (typeof runtime.IntersectionObserver !== "function")
    return () => undefined;
  const observer = new runtime.IntersectionObserver(
    ([entry]) => {
      masthead.setAttribute(
        "data-k-running",
        entry?.isIntersecting ? "true" : "false",
      );
    },
    { threshold: 0.1 },
  );
  observer.observe(masthead);
  return () => {
    observer.disconnect();
    masthead.removeAttribute("data-k-running");
  };
}

// ---- Scroll routing journey (Task 3) --------------------------------------

type RouteState = "pending" | "active" | "complete";

/** Pure state fan-out around the active index. */
export function routeStates(activeIndex: number, count: number): RouteState[] {
  return Array.from({ length: count }, (_, index) =>
    index < activeIndex
      ? "complete"
      : index === activeIndex
        ? "active"
        : "pending",
  );
}

type JourneyRuntime = {
  matchMedia: (query: string) => Pick<MediaQueryList, "matches">;
  scrollY: number;
  innerHeight: number;
  innerWidth?: number;
  requestAnimationFrame: (job: () => void) => number;
  cancelAnimationFrame: (handle: number) => void;
  addEventListener: (
    name: string,
    fn: () => void,
    opts?: AddEventListenerOptions,
  ) => void;
  removeEventListener: (name: string, fn: () => void) => void;
  ResizeObserver?: typeof ResizeObserver;
};

type JourneyRoot = Pick<Document, "querySelector" | "querySelectorAll">;

/** Arm the scroll journey, transplanted from the approved mockup: the
 * signal travels the weave and ARRIVES at each section's port exactly as
 * that section activates (the visible mockup contract), interpolating the
 * path arc between port milestones; the clip reveals the live route up to
 * the signal's own Y (the path only ever descends, so the reveal always
 * ends at the signal). Geometry (ports, section tops, the polyline, arc
 * table and scroll knots) is measured at init and resize ONLY; scroll
 * coalesces into one animation frame. Under reduced motion the circuit is
 * static and complete (data-k-static) while section states still track the
 * scroll, exactly like the mockup. Cleanup removes every listener,
 * observer, frame, attribute and property. */
export function armRoutingJourney(
  root: JourneyRoot = document,
  runtime: JourneyRuntime = window,
): () => void {
  const journey = root.querySelector<HTMLElement>("[data-k-journey]");
  if (!journey) return () => undefined;
  const sections = Array.from(
    root.querySelectorAll<HTMLElement>("[data-k-section]"),
  );
  if (sections.length === 0) return () => undefined;

  if (!runtime.matchMedia("(prefers-reduced-motion: no-preference)").matches) {
    journey.setAttribute("data-k-static", "true");
  }

  const signal = root.querySelector<HTMLElement>("[data-k-route-signal]");
  const status = root.querySelector<HTMLElement>("[data-k-route-status]");

  let sectionTops: number[] = [];
  let vertices: Array<{ x: number; y: number }> = [];
  let cumulative: number[] = [];
  let knots: Array<{ scroll: number; arc: number }> = [];
  let height = 1;
  let lastActive = -1;

  const measure = () => {
    const journeyRect = journey.getBoundingClientRect?.();
    const baseTop = (journeyRect?.top ?? 0) + runtime.scrollY;
    const baseLeft = journeyRect?.left ?? 0;
    height = Math.max(1, journeyRect?.height ?? 1);
    const mobile = (runtime.innerWidth ?? 1200) < 996;
    journey.setAttribute("data-k-journey-mode", mobile ? "mobile" : "desktop");
    const owner = (
      journey as {
        ownerDocument?: { documentElement?: { scrollHeight?: number } };
      }
    ).ownerDocument;
    const documentHeight =
      owner?.documentElement?.scrollHeight ?? baseTop + height;
    const maxScroll = Math.max(1, documentHeight - runtime.innerHeight);
    sectionTops = sections.map(
      (section) =>
        (section.getBoundingClientRect?.()?.top ?? 0) + runtime.scrollY,
    );
    const points: Array<{ x: number; y: number }> = [];
    for (const section of sections) {
      const port = section.querySelector<HTMLElement>("[data-k-route-port]");
      if (!port) continue;
      const rect = port.getBoundingClientRect();
      points.push({
        x: rect.left + rect.width / 2 - baseLeft,
        y: rect.top + rect.height / 2 + runtime.scrollY - baseTop,
      });
    }
    const first = points[0];
    if (!first) return;
    // The mockup's literal weave as a polyline (every segment axis-aligned):
    // desktop sends the first leg out to the right rail, then a vertical
    // drop and a horizontal run into each next port; mobile folds the same
    // journey onto a left rail so the circuit never crosses readable text.
    vertices = [first];
    const portArcs: number[] = [];
    const push = (x: number, y: number) => {
      const previous = vertices[vertices.length - 1]!;
      if (previous.x !== x || previous.y !== y) vertices.push({ x, y });
    };
    if (mobile) {
      for (const point of points.slice(1)) {
        push(14, vertices[vertices.length - 1]!.y);
        push(14, point.y);
        push(point.x, point.y);
        portArcs.push(vertices.length - 1);
      }
      push(vertices[vertices.length - 1]!.x, Math.max(0, height - 40));
    } else {
      const rail = Math.max(...points.map((point) => point.x));
      const second = points[1];
      if (second) {
        push(rail, first.y);
        push(rail, second.y);
        push(second.x, second.y);
        portArcs.push(vertices.length - 1);
      }
      for (const point of points.slice(2)) {
        // Mockup order (e.g. "V1540H40"): descend at the current x to the
        // next port's y, then run horizontally into the port.
        push(vertices[vertices.length - 1]!.x, point.y);
        push(point.x, point.y);
        portArcs.push(vertices.length - 1);
      }
      push(vertices[vertices.length - 1]!.x, Math.max(0, height - 60));
    }
    cumulative = [0];
    for (let index = 1; index < vertices.length; index++) {
      const a = vertices[index - 1]!;
      const b = vertices[index]!;
      cumulative.push(
        cumulative[index - 1]! + Math.abs(b.x - a.x) + Math.abs(b.y - a.y),
      );
    }
    const total = cumulative[cumulative.length - 1] ?? 0;
    // Scroll knots: the signal sits ON port i exactly when section i crosses
    // the 52% focus line, and runs out the tail at the very bottom.
    knots = [{ scroll: 0, arc: 0 }];
    points.forEach((_, index) => {
      if (index === 0) return;
      const top = sectionTops[index] ?? 0;
      const scroll = Math.max(
        (knots[knots.length - 1]?.scroll ?? 0) + 1,
        top - runtime.innerHeight * 0.52,
      );
      knots.push({ scroll, arc: cumulative[portArcs[index - 1] ?? 0] ?? 0 });
    });
    knots.push({
      scroll: Math.max(maxScroll, (knots[knots.length - 1]?.scroll ?? 0) + 1),
      arc: total,
    });
    const d = vertices
      .map((point, index) => `${index === 0 ? "M" : "L"}${point.x} ${point.y}`)
      .join("");
    const base = journey.querySelector?.("[data-k-route-path-base]");
    const active = journey.querySelector?.("[data-k-route-path-active]");
    base?.setAttribute("d", d);
    active?.setAttribute("d", d);
    const svg = journey.querySelector?.("svg");
    if (journeyRect) {
      svg?.setAttribute(
        "viewBox",
        `0 0 ${Math.max(1, journeyRect.width)} ${height}`,
      );
    }
  };
  measure();

  const pointAtArc = (arc: number): { x: number; y: number } => {
    const first = vertices[0];
    if (!first) return { x: 0, y: 0 };
    for (let index = 1; index < vertices.length; index++) {
      if (arc <= (cumulative[index] ?? 0)) {
        const a = vertices[index - 1]!;
        const b = vertices[index]!;
        const span = (cumulative[index] ?? 0) - (cumulative[index - 1] ?? 0);
        const t = span > 0 ? (arc - (cumulative[index - 1] ?? 0)) / span : 1;
        return { x: a.x + (b.x - a.x) * t, y: a.y + (b.y - a.y) * t };
      }
    }
    return vertices[vertices.length - 1] ?? first;
  };

  const arcAtScroll = (scroll: number): number => {
    const first = knots[0];
    const last = knots[knots.length - 1];
    if (!first || !last) return 0;
    if (scroll <= first.scroll) return first.arc;
    if (scroll >= last.scroll) return last.arc;
    for (let index = 1; index < knots.length; index++) {
      const b = knots[index]!;
      if (scroll <= b.scroll) {
        const a = knots[index - 1]!;
        const t = (scroll - a.scroll) / Math.max(1, b.scroll - a.scroll);
        return a.arc + (b.arc - a.arc) * t;
      }
    }
    return last.arc;
  };

  let frame: number | null = null;
  const render = () => {
    frame = null;
    const point = pointAtArc(arcAtScroll(runtime.scrollY));
    const progress = Math.min(1, Math.max(0.005, point.y / height));
    journey.style.setProperty(
      "--k-route-progress",
      String(Math.round(progress * 1000) / 1000),
    );
    signal?.style.setProperty(
      "transform",
      `translate(${point.x}px, ${point.y}px)`,
    );
    const focus = runtime.scrollY + runtime.innerHeight * 0.52;
    let active = 0;
    for (let index = 0; index < sectionTops.length; index++) {
      if ((sectionTops[index] ?? Number.POSITIVE_INFINITY) <= focus)
        active = index;
    }
    if (active !== lastActive) {
      lastActive = active;
      const states = routeStates(active, sections.length);
      sections.forEach((section, index) => {
        section.setAttribute("data-k-route-state", states[index] ?? "pending");
      });
      if (status) {
        status.textContent = `Routing · ${sections[active]?.getAttribute("data-k-section") ?? ""}`;
      }
    }
  };

  const onScroll = () => {
    if (frame !== null) return;
    frame = runtime.requestAnimationFrame(render);
  };
  runtime.addEventListener("scroll", onScroll, { passive: true });

  let resizeObserver: ResizeObserver | null = null;
  const onResize = () => {
    measure();
    lastActive = -1;
    onScroll();
  };
  if (typeof runtime.ResizeObserver === "function") {
    resizeObserver = new runtime.ResizeObserver(onResize);
    resizeObserver.observe(journey);
  } else {
    runtime.addEventListener("resize", onResize);
  }

  render();

  return () => {
    if (frame !== null) runtime.cancelAnimationFrame(frame);
    frame = null;
    runtime.removeEventListener("scroll", onScroll);
    if (resizeObserver) resizeObserver.disconnect();
    else runtime.removeEventListener("resize", onResize);
    journey.style.removeProperty("--k-route-progress");
    journey.removeAttribute("data-k-journey-mode");
    journey.removeAttribute("data-k-static");
    signal?.style.removeProperty("transform");
    sections.forEach((section) =>
      section.removeAttribute("data-k-route-state"),
    );
  };
}
