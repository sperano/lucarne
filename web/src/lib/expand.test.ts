import { beforeEach, describe, expect, it } from 'vitest';
import { EXPAND_ALL_KEY, loadExpandAll, saveExpandAll } from './expand';

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

describe('expand all preference', () => {
  it('is off when nothing was saved or there is no storage', () => {
    expect(loadExpandAll(storage)).toBe(false);
    expect(loadExpandAll(undefined)).toBe(false);
  });

  it('keeps the choice on, then forgets it when turned off', () => {
    saveExpandAll(storage, true);
    expect(storage.getItem(EXPAND_ALL_KEY)).toBe('true');
    expect(loadExpandAll(storage)).toBe(true);
    saveExpandAll(storage, false);
    expect(storage.getItem(EXPAND_ALL_KEY)).toBeNull();
    expect(loadExpandAll(storage)).toBe(false);
  });

  it('reads any other saved value as off', () => {
    storage.setItem(EXPAND_ALL_KEY, '1');
    expect(loadExpandAll(storage)).toBe(false);
  });

  it('reads as off and saves nothing where storage refuses it', () => {
    const refusing = new RefusingStorage();
    expect(loadExpandAll(refusing)).toBe(false);
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
