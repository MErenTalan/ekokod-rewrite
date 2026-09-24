import '@testing-library/jest-dom/vitest';

import { vi } from 'vitest';

import { setMatchMedia } from './src/test/match-media';
import { mockPathname, mockRouter } from './src/test/navigation';

// `@vitest-environment node` files (scripts, lint, fonts) share this setup but have no DOM.
if (typeof window !== 'undefined') {
  // jsdom lacks the layout APIs Radix and Recharts touch.
  class ResizeObserverStub {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  globalThis.ResizeObserver ??= ResizeObserverStub as unknown as typeof ResizeObserver;
  Element.prototype.scrollIntoView ??= function scrollIntoView() {};
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.releasePointerCapture ??= () => {};
  setMatchMedia(() => false);

  // Recharts' ResponsiveContainer re-measures on mount and jsdom reports 0×0, which would hide every chart.
  const nativeRect = Element.prototype.getBoundingClientRect;
  Element.prototype.getBoundingClientRect = function getBoundingClientRect(this: Element) {
    if (!this.classList.contains('recharts-responsive-container')) return nativeRect.call(this);
    const height = parseFloat((this as HTMLElement).style.height) || 320;
    return { x: 0, y: 0, top: 0, left: 0, width: 640, height, right: 640, bottom: height, toJSON: () => ({}) } as DOMRect;
  };

  // nwsapi 2.2.27 resolves :modal/:fullscreen/:open/:closed by calling node.matches, which jsdom routes back into
  // nwsapi: it recurses to a stack overflow per call (~15 s per Radix popper via floating-ui's isTopLayer).
  // jsdom has no top layer, so these states are always false.
  const TOP_LAYER_STATES = /^:(?:modal|fullscreen|popover-open|open|closed)$/;
  const nativeMatches = Element.prototype.matches;
  Element.prototype.matches = function matches(this: Element, selector: string) {
    return TOP_LAYER_STATES.test(selector.trim()) ? false : nativeMatches.call(this, selector);
  };
}

vi.mock('next/navigation', () => ({
  usePathname: () => mockPathname.current,
  useRouter: () => mockRouter,
  useSearchParams: () => new URLSearchParams(),
}));
