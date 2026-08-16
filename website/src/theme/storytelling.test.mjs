import assert from 'node:assert/strict'
import test from 'node:test'

class FakeClassList {
  values = new Set()

  add(...names) {
    names.forEach((name) => this.values.add(name))
  }

  remove(...names) {
    names.forEach((name) => this.values.delete(name))
  }

  contains(name) {
    return this.values.has(name)
  }
}

class FakeStyle {
  values = new Map()

  setProperty(name, value) {
    this.values.set(name, value)
  }

  removeProperty(name) {
    this.values.delete(name)
  }

  getPropertyValue(name) {
    return this.values.get(name) ?? ''
  }
}

class FakeElement {
  classList = new FakeClassList()
  style = new FakeStyle()

  constructor(section, targets = []) {
    this.section = section
    this.targets = targets
  }

  getAttribute(name) {
    return name === 'data-k-section' ? this.section : null
  }

  querySelectorAll(selector) {
    return selector === '[data-motion]' ? this.targets : []
  }
}

async function loadStorytelling() {
  const module = await import('./storytelling.ts').catch(() => null)
  assert.ok(module, 'the scroll-storytelling controller must exist')
  return module
}

function createHarness({ reducedMotion = false, intersectionObserver = true } = {}) {
  const heroTarget = new FakeElement()
  const installTargets = [new FakeElement(), new FakeElement()]
  const capabilityTargets = [new FakeElement(), new FakeElement(), new FakeElement()]
  const sections = [
    new FakeElement('hero', [heroTarget]),
    new FakeElement('install', installTargets),
    new FakeElement('capabilities', capabilityTargets),
  ]
  const documentElement = new FakeElement()
  const document = {
    documentElement,
    querySelector(selector) {
      return selector === '[data-k-section="hero"]' ? sections[0] : null
    },
    querySelectorAll(selector) {
      return selector === '[data-k-section]' ? sections : []
    },
  }

  let observer
  class FakeIntersectionObserver {
    observed = new Set()

    constructor(callback) {
      this.callback = callback
      observer = this
    }

    observe(target) {
      this.observed.add(target)
    }

    unobserve(target) {
      this.observed.delete(target)
    }

    disconnect() {
      this.observed.clear()
    }

    reveal(target) {
      this.callback([{ isIntersecting: true, target }], this)
    }
  }

  const runtime = {
    matchMedia: () => ({ matches: !reducedMotion }),
  }
  if (intersectionObserver) runtime.IntersectionObserver = FakeIntersectionObserver

  return { capabilityTargets, document, documentElement, heroTarget, installTargets, runtime, observer: () => observer }
}

test('arms below-fold sections and reveals each target once', async () => {
  const { armStorytelling } = await loadStorytelling()
  const harness = createHarness()

  const cleanup = armStorytelling(harness.document, harness.runtime)
  const observer = harness.observer()

  assert.equal(harness.documentElement.classList.contains('k-motion'), true)
  assert.equal(observer.observed.has(harness.heroTarget), false)
  assert.equal(observer.observed.size, 5)
  assert.equal(harness.installTargets[0].style.getPropertyValue('--k-motion-delay'), '0ms')
  assert.equal(harness.installTargets[1].style.getPropertyValue('--k-motion-delay'), '70ms')
  assert.equal(harness.capabilityTargets[0].style.getPropertyValue('--k-motion-delay'), '0ms')

  observer.reveal(harness.installTargets[0])
  assert.equal(harness.installTargets[0].classList.contains('k-in'), true)
  assert.equal(observer.observed.has(harness.installTargets[0]), false)

  cleanup()
  assert.equal(harness.documentElement.classList.contains('k-motion'), false)
})

test('keeps every section visible when reduced motion is requested', async () => {
  const { armStorytelling } = await loadStorytelling()
  const harness = createHarness({ reducedMotion: true })

  armStorytelling(harness.document, harness.runtime)

  assert.equal(harness.documentElement.classList.contains('k-motion'), false)
  assert.equal(harness.observer(), undefined)
  assert.equal(harness.installTargets[0].classList.contains('k-reveal'), false)
})

test('keeps every section visible without IntersectionObserver', async () => {
  const { armStorytelling } = await loadStorytelling()
  const harness = createHarness({ intersectionObserver: false })

  armStorytelling(harness.document, harness.runtime)

  assert.equal(harness.documentElement.classList.contains('k-motion'), false)
  assert.equal(harness.observer(), undefined)
})
