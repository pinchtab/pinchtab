import "@testing-library/jest-dom/vitest";

class MemoryStorage implements Storage {
  private store = new Map<string, string>();

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

const ensureStorage = (name: "localStorage" | "sessionStorage") => {
  const current = globalThis[name];
  if (
    !current ||
    typeof current.getItem !== "function" ||
    typeof current.setItem !== "function" ||
    typeof current.removeItem !== "function" ||
    typeof current.clear !== "function"
  ) {
    Object.defineProperty(globalThis, name, {
      value: new MemoryStorage(),
      configurable: true,
      writable: true,
    });
    return;
  }

  current.clear();
};

ensureStorage("localStorage");
ensureStorage("sessionStorage");
