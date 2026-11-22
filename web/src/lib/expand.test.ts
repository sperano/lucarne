import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { EXPAND_ALL_KEY, deviceStorage, loadExpandAll, saveExpandAll } from './expand';

class MemoryStorage implements Storage {
  private readonly items = new Map<string, string>();
  get length(): number {
    return this.items.size;
  }
  clear(): void {
    this.items.clear();
  }
  getItem(key: string): string | null {
    return this.items.get(key) ?? null;
  }
  key(index: number): string | null {
    return [...this.items.keys()][index] ?? null;
  }
  removeItem(key: string): void {
    this.items.delete(key);
  }
  setItem(key: string, value: string): void {
    this.items.set(key, value);
  }
}

class RefusingStorage extends MemoryStorage {
  override getItem(): string | null {
    throw new DOMException('denied', 'SecurityError');
  }
  override setItem(): void {
    throw new DOMException('full', 'QuotaExceededError');
  }
  override removeItem(): void {
    throw new DOMException('denied', 'SecurityError');
  }
}

let storage: MemoryStorage;

beforeEach(() => {
  storage = new MemoryStorage();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('deviceStorage', () => {
  it("returns the window's localStorage", () => {
    vi.stubGlobal('window', { localStorage: storage });
    expect(deviceStorage()).toBe(storage);
  });

  it('returns undefined where reading localStorage throws', () => {
    const refusing = {};
    Object.defineProperty(refusing, 'localStorage', {
      get(): never {
        throw new DOMException('denied', 'SecurityError');
      },
    });
    vi.stubGlobal('window', refusing);
    expect(deviceStorage()).toBeUndefined();
    expect(loadExpandAll(deviceStorage())).toBe(true);
  });
});

describe('expand all preference', () => {
  it('is on when nothing was saved or there is no storage', () => {
    expect(loadExpandAll(storage)).toBe(true);
    expect(loadExpandAll(undefined)).toBe(true);
  });

  it('keeps the choice off, then forgets it when turned back on', () => {
    saveExpandAll(storage, false);
    expect(storage.getItem(EXPAND_ALL_KEY)).toBe('false');
    expect(loadExpandAll(storage)).toBe(false);
    saveExpandAll(storage, true);
    expect(storage.getItem(EXPAND_ALL_KEY)).toBeNull();
    expect(loadExpandAll(storage)).toBe(true);
  });

  it('reads any other saved value as on', () => {
    storage.setItem(EXPAND_ALL_KEY, '1');
    expect(loadExpandAll(storage)).toBe(true);
  });

  it('reads as on and saves nothing where storage refuses it', () => {
    const refusing = new RefusingStorage();
    expect(loadExpandAll(refusing)).toBe(true);
    expect(() => {
      saveExpandAll(refusing, true);
    }).not.toThrow();
    expect(() => {
      saveExpandAll(refusing, false);
    }).not.toThrow();
    expect(() => {
      saveExpandAll(undefined, true);
    }).not.toThrow();
  });
});
