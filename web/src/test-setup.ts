import "@testing-library/jest-dom/vitest";
import { webcrypto } from "node:crypto";

// Some Node/jsdom combinations expose a globalThis.crypto without SubtleCrypto.
// The verification library depends on crypto.subtle, so alias Node's webcrypto
// implementation in when it's missing.
if (!globalThis.crypto?.subtle) {
  Object.defineProperty(globalThis, "crypto", {
    value: webcrypto,
    configurable: true,
  });
}
