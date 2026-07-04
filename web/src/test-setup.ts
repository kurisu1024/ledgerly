import "@testing-library/jest-dom/vitest";
import { webcrypto } from "node:crypto";
import { afterEach } from "vitest";
import { cleanup } from "@testing-library/react";

// vitest doesn't auto-register RTL's cleanup without `test.globals: true`;
// this project keeps explicit `import { afterEach } from "vitest"` in test
// files instead, so cleanup has to be wired up here.
afterEach(() => {
  cleanup();
});

// Node 22+ defines a global `localStorage`/`sessionStorage` accessor that
// returns undefined unless launched with --localstorage-file, and it shadows
// jsdom's own window.localStorage on the shared global object. Patch in a
// minimal in-memory Storage so TokenGate's sessionStorage usage works under
// jsdom the same way it does in a real browser.
class MemoryStorage implements Storage {
  private readonly store = new Map<string, string>();

  get length(): number {
    return this.store.size;
  }

  clear(): void {
    this.store.clear();
  }

  getItem(key: string): string | null {
    return this.store.has(key) ? this.store.get(key)! : null;
  }

  key(index: number): string | null {
    return Array.from(this.store.keys())[index] ?? null;
  }

  removeItem(key: string): void {
    this.store.delete(key);
  }

  setItem(key: string, value: string): void {
    this.store.set(key, String(value));
  }
}

for (const name of ["localStorage", "sessionStorage"] as const) {
  if (!globalThis[name]) {
    Object.defineProperty(globalThis, name, {
      value: new MemoryStorage(),
      configurable: true,
      writable: true,
    });
  }
}

// Some Node/jsdom combinations expose a globalThis.crypto without SubtleCrypto.
// The verification library depends on crypto.subtle, so alias Node's webcrypto
// implementation in when it's missing.
if (!globalThis.crypto?.subtle) {
  Object.defineProperty(globalThis, "crypto", {
    value: webcrypto,
    configurable: true,
  });
}
