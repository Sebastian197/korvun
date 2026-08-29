import assert from "node:assert/strict";
import test from "node:test";

class FakeClassList {
  values = new Set();

  add(...names) {
    names.forEach((name) => this.values.add(name));
  }

  remove(...names) {
    names.forEach((name) => this.values.delete(name));
  }

  contains(name) {
    return this.values.has(name);
  }
}

class FakeStyle {
  values = new Map();

  setProperty(name, value) {
    this.values.set(name, value);
  }

  removeProperty(name) {
    this.values.delete(name);
  }

  getPropertyValue(name) {
    return this.values.get(name) ?? "";
  }
}

class FakeElement {
  classList = new FakeClassList();
  style = new FakeStyle();

  constructor(section, targets = []) {
    this.section = section;
    this.targets = targets;
  }

  getAttribute(name) {
    return name === "data-k-section" ? this.section : null;
  }

  querySelectorAll(selector) {
    return selector === "[data-motion]" ? this.targets : [];
  }
}

async function loadStorytelling() {
  const module = await import("./storytelling.ts").catch(() => null);
  assert.ok(module, "the scroll-storytelling controller must exist");
  return module;
}

function createHarness({
  reducedMotion = false,
  intersectionObserver = true,
} = {}) {
  const heroTarget = new FakeElement();
  const installTargets = [new FakeElement(), new FakeElement()];
  const capabilityTargets = [
    new FakeElement(),
    new FakeElement(),
    new FakeElement(),
  ];
  const sections = [
    new FakeElement("hero", [heroTarget]),
    new FakeElement("install", installTargets),
    new FakeElement("capabilities", capabilityTargets),
  ];
  const documentElement = new FakeElement();
  const document = {
    documentElement,
    querySelector(selector) {
      return selector === '[data-k-section="hero"]' ? sections[0] : null;
    },
    querySelectorAll(selector) {
      return selector === "[data-k-section]" ? sections : [];
    },
  };

  let observer;
  class FakeIntersectionObserver {
    observed = new Set();

    constructor(callback) {
      this.callback = callback;
      observer = this;
    }

    observe(target) {
      this.observed.add(target);
    }

    unobserve(target) {
      this.observed.delete(target);
    }

    disconnect() {
      this.observed.clear();
    }

    reveal(target) {
      this.callback([{ isIntersecting: true, target }], this);
    }
  }

  const runtime = {
    matchMedia: () => ({ matches: !reducedMotion }),
  };
  if (intersectionObserver)
    runtime.IntersectionObserver = FakeIntersectionObserver;

  return {
    capabilityTargets,
    document,
    documentElement,
    heroTarget,
    installTargets,
    runtime,
    observer: () => observer,
  };
}

test("arms below-fold sections and reveals each target once", async () => {
  const { armStorytelling } = await loadStorytelling();
  const harness = createHarness();

  const cleanup = armStorytelling(harness.document, harness.runtime);
  const observer = harness.observer();

  assert.equal(harness.documentElement.classList.contains("k-motion"), true);
  assert.equal(observer.observed.has(harness.heroTarget), false);
  assert.equal(observer.observed.size, 5);
  assert.equal(
    harness.installTargets[0].style.getPropertyValue("--k-motion-delay"),
    "0ms",
  );
  assert.equal(
    harness.installTargets[1].style.getPropertyValue("--k-motion-delay"),
    "70ms",
  );
  assert.equal(
    harness.capabilityTargets[0].style.getPropertyValue("--k-motion-delay"),
    "0ms",
  );

  observer.reveal(harness.installTargets[0]);
  assert.equal(harness.installTargets[0].classList.contains("k-in"), true);
  assert.equal(observer.observed.has(harness.installTargets[0]), false);

  cleanup();
  assert.equal(harness.documentElement.classList.contains("k-motion"), false);
});

test("keeps every section visible when reduced motion is requested", async () => {
  const { armStorytelling } = await loadStorytelling();
  const harness = createHarness({ reducedMotion: true });

  armStorytelling(harness.document, harness.runtime);

  assert.equal(harness.documentElement.classList.contains("k-motion"), false);
  assert.equal(harness.observer(), undefined);
  assert.equal(harness.installTargets[0].classList.contains("k-reveal"), false);
});

test("keeps every section visible without IntersectionObserver", async () => {
  const { armStorytelling } = await loadStorytelling();
  const harness = createHarness({ intersectionObserver: false });

  armStorytelling(harness.document, harness.runtime);

  assert.equal(harness.documentElement.classList.contains("k-motion"), false);
  assert.equal(harness.observer(), undefined);
});

// ---- Brand-motion masthead (brand-motion spec, Task 2) ---------------------

class FakeMasthead {
  attributes = new Map();

  setAttribute(name, value) {
    this.attributes.set(name, value);
  }

  getAttribute(name) {
    return this.attributes.get(name) ?? null;
  }

  removeAttribute(name) {
    this.attributes.delete(name);
  }

  hasAttribute(name) {
    return this.attributes.has(name);
  }
}

function createBrandHarness({
  reducedMotion = false,
  intersectionObserver = true,
} = {}) {
  const masthead = new FakeMasthead();
  const document = {
    querySelector(selector) {
      return selector === "[data-k-masthead]" ? masthead : null;
    },
  };
  const visibility = {
    created: false,
    observer: null,
    reveal(isIntersecting) {
      assert.ok(this.observer, "masthead observer must exist before reveal");
      this.observer.callback([{ isIntersecting }], this.observer);
    },
  };
  class FakeIntersectionObserver {
    constructor(callback) {
      this.callback = callback;
      visibility.created = true;
      visibility.observer = this;
      this.observed = new Set();
    }

    observe(target) {
      this.observed.add(target);
    }

    disconnect() {
      this.observed.clear();
    }
  }
  const runtime = {
    matchMedia: (query) => ({
      matches: query.includes("no-preference") ? !reducedMotion : reducedMotion,
    }),
  };
  if (intersectionObserver)
    runtime.IntersectionObserver = FakeIntersectionObserver;
  return { document, runtime, masthead, visibility };
}

test("runs the masthead only while visible and motion is allowed", async () => {
  const { armBrandMotion } = await import("./brandMotion.ts");
  const harness = createBrandHarness();
  const cleanup = armBrandMotion(harness.document, harness.runtime);
  harness.visibility.reveal(true);
  assert.equal(harness.masthead.getAttribute("data-k-running"), "true");
  harness.visibility.reveal(false);
  assert.equal(harness.masthead.getAttribute("data-k-running"), "false");
  cleanup();
  assert.equal(harness.masthead.hasAttribute("data-k-running"), false);
});

test("keeps the masthead static under reduced motion", async () => {
  const { armBrandMotion } = await import("./brandMotion.ts");
  const harness = createBrandHarness({ reducedMotion: true });
  armBrandMotion(harness.document, harness.runtime);
  assert.equal(harness.visibility.created, false);
  assert.equal(harness.masthead.hasAttribute("data-k-running"), false);
});

test("arms nothing without a masthead or without observer support", async () => {
  const { armBrandMotion } = await import("./brandMotion.ts");
  const empty = { querySelector: () => null };
  const cleanupNone = armBrandMotion(empty, {
    matchMedia: () => ({ matches: true }),
  });
  assert.equal(typeof cleanupNone, "function");
  cleanupNone();
  const harness = createBrandHarness({ intersectionObserver: false });
  const cleanup = armBrandMotion(harness.document, harness.runtime);
  assert.equal(harness.visibility.created, false);
  cleanup();
});

// ---- Scroll routing journey (brand-motion spec, Task 3) --------------------

class FakeJourneyElement {
  attributes = new Map();
  dataset = {};
  style = new FakeStyle();

  constructor(section = null) {
    this.section = section;
  }

  setAttribute(name, value) {
    this.attributes.set(name, value);
    if (name.startsWith("data-")) {
      const key = name.slice(5).replace(/-([a-z])/g, (_, c) => c.toUpperCase());
      this.dataset[key] = value;
    }
  }

  getAttribute(name) {
    return this.attributes.get(name) ?? null;
  }

  removeAttribute(name) {
    this.attributes.delete(name);
    const key = name.slice(5).replace(/-([a-z])/g, (_, c) => c.toUpperCase());
    delete this.dataset[key];
  }
}

function createJourneyHarness({
  reducedMotion = false,
  resizeObserver = true,
} = {}) {
  const journey = new FakeJourneyElement();
  journey.getBoundingClientRect = () => ({
    top: 0,
    left: 0,
    width: 1200,
    height: 3600,
  });
  journey.ownerDocument = { documentElement: { scrollHeight: 4200 } };
  const signal = new FakeJourneyElement();
  const sections = [
    "hero",
    "install",
    "capabilities",
    "privacy",
    "demo",
    "final",
  ].map((name) => new FakeJourneyElement(name));
  const geometry = { readCount: 0 };
  const ports = sections.map((_, index) => {
    const port = new FakeJourneyElement();
    port.getBoundingClientRect = () => {
      geometry.readCount++;
      return { top: index * 600, left: 40, width: 12, height: 12 };
    };
    return port;
  });
  sections.forEach((section, index) => {
    section.getBoundingClientRect = () => ({
      top: index * 600,
      left: 0,
      width: 1200,
      height: 590,
    });
    section.setAttribute("data-k-section", section.section);
    section.querySelector = (selector) =>
      selector === "[data-k-route-port]" ? ports[index] : null;
  });
  const listeners = new Map();
  const frames = {
    queue: [],
    pendingCount() {
      return this.queue.length;
    },
    flush() {
      const jobs = [...this.queue];
      this.queue = [];
      jobs.forEach((job) => job());
    },
  };
  const document = {
    querySelector(selector) {
      if (selector === "[data-k-journey]") return journey;
      if (selector === "[data-k-route-signal]") return signal;
      return null;
    },
    querySelectorAll(selector) {
      return selector === "[data-k-section]" ? sections : [];
    },
  };
  const runtime = {
    matchMedia: (query) => ({
      matches: query.includes("no-preference") ? !reducedMotion : reducedMotion,
    }),
    scrollY: 0,
    innerHeight: 600,
    requestAnimationFrame: (job) => {
      frames.queue.push(job);
      return frames.queue.length;
    },
    cancelAnimationFrame: () => {
      frames.queue = [];
    },
    addEventListener: (name, fn) => listeners.set(name, fn),
    removeEventListener: (name) => listeners.delete(name),
  };
  if (resizeObserver) {
    runtime.ResizeObserver = class {
      constructor(callback) {
        runtime.resizeCallback = callback;
      }

      observe() {}

      disconnect() {}
    };
  }
  const scroll = {
    fire(y = runtime.scrollY) {
      runtime.scrollY = y;
      listeners.get("scroll")?.();
    },
  };
  return {
    document,
    runtime,
    journey,
    signal,
    sections,
    geometry,
    frames,
    scroll,
    listeners,
  };
}

test("routeStates orders pending/active/complete around the active index", async () => {
  const { routeStates } = await import("./brandMotion.ts");
  assert.deepEqual(routeStates(2, 6), [
    "complete",
    "complete",
    "active",
    "pending",
    "pending",
    "pending",
  ]);
  assert.deepEqual(routeStates(0, 3), ["active", "pending", "pending"]);
});

test("coalesces scroll into one frame, writes progress from cached geometry", async () => {
  const { armRoutingJourney } = await import("./brandMotion.ts");
  const harness = createJourneyHarness();
  const cleanup = armRoutingJourney(harness.document, harness.runtime);
  const initialReadCount = harness.geometry.readCount;
  assert.ok(initialReadCount > 0, "geometry must be measured at init");

  // Scroll to the midpoint of the scrollable range (progress is the global
  // scroll fraction, like the mockup): two events must schedule exactly ONE
  // frame.
  harness.scroll.fire(1200);
  harness.scroll.fire(1200);
  assert.equal(harness.frames.pendingCount(), 1);
  harness.frames.flush();
  // The fixture route leaves each port downward to the inter-section band
  // (next section top minus 32), jogs to the left gutter (x=16), drops,
  // and enters the next port at x=46 — 660 of Manhattan arc per leg.
  // Milestone knots put the signal on port 2 (arc 1320) at scroll 888 and
  // port 3 (arc 1980) at 1488; at scroll 1200 the arc interpolates to
  // 1663.2 — still on the leg's opening drop at (46, 1549.2) — and the
  // clip reveals 1549.2/3600 of the layer.
  assert.equal(
    harness.journey.style.getPropertyValue("--k-route-progress"),
    "0.43",
  );
  assert.equal(
    harness.signal.style.getPropertyValue("transform"),
    "translate(46px, 1549.2px)",
  );
  assert.deepEqual(
    harness.sections.map((section) => section.dataset.kRouteState),
    ["complete", "complete", "active", "pending", "pending", "pending"],
  );
  // No per-frame geometry reads: the render frame reuses the cache.
  assert.equal(harness.geometry.readCount, initialReadCount);
  cleanup();
});

test("resize recomputes geometry; cleanup clears every trace", async () => {
  const { armRoutingJourney } = await import("./brandMotion.ts");
  const harness = createJourneyHarness();
  const cleanup = armRoutingJourney(harness.document, harness.runtime);
  const afterInit = harness.geometry.readCount;
  harness.runtime.resizeCallback?.([]);
  assert.ok(
    harness.geometry.readCount > afterInit,
    "resize must re-measure ports",
  );

  harness.scroll.fire(600);
  harness.frames.flush();
  cleanup();
  assert.equal(
    harness.journey.style.getPropertyValue("--k-route-progress"),
    "",
  );
  assert.ok(
    harness.sections.every(
      (section) => section.dataset.kRouteState === undefined,
    ),
    "cleanup must remove every data-k-route-state",
  );
  assert.equal(harness.listeners.size, 0, "cleanup must remove listeners");
});

test("reduced motion marks the circuit static while states still track", async () => {
  const { armRoutingJourney } = await import("./brandMotion.ts");
  const harness = createJourneyHarness({ reducedMotion: true });
  const cleanup = armRoutingJourney(harness.document, harness.runtime);
  assert.equal(harness.journey.getAttribute("data-k-static"), "true");
  // The mockup keeps routing states alive under reduced motion — only the
  // moving parts (signal, clip reveal, transitions) go static via CSS.
  harness.scroll.fire(1200);
  harness.frames.flush();
  assert.equal(harness.sections[2].dataset.kRouteState, "active");
  cleanup();
  assert.equal(harness.journey.getAttribute("data-k-static"), null);
});
