import "@testing-library/jest-dom/vitest";
import { vi } from "vitest";

// antd reads window.matchMedia (responsive) and ResizeObserver, neither of which
// jsdom implements. Provide minimal stubs so components render in tests.
Object.defineProperty(window, "matchMedia", {
  writable: true,
  value: (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }),
});

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
(globalThis as unknown as { ResizeObserver: unknown }).ResizeObserver = ResizeObserverStub;

// jsdom doesn't implement the 2-arg getComputedStyle(el, pseudoEl) form that
// antd's scrollbar measurement uses; drop the pseudo arg to avoid noisy
// "Not implemented" errors (tests pass regardless).
const realGetComputedStyle = window.getComputedStyle.bind(window);
window.getComputedStyle = ((el: Element) => realGetComputedStyle(el)) as typeof window.getComputedStyle;

// jsdom has no localStorage in some configs; ensure a working one.
if (!("localStorage" in globalThis)) {
  const store = new Map<string, string>();
  (globalThis as unknown as { localStorage: Storage }).localStorage = {
    getItem: (k) => store.get(k) ?? null,
    setItem: (k, v) => void store.set(k, String(v)),
    removeItem: (k) => void store.delete(k),
    clear: () => store.clear(),
    key: (i) => Array.from(store.keys())[i] ?? null,
    get length() {
      return store.size;
    },
  } as Storage;
}
