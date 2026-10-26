import { beforeEach, describe, expect, it } from 'vitest';
import { DESTINATION_TTL_MS, SIGN_IN_INTERVAL_MS, restoreDestination, signIn } from './signin';

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

const PATH = '/runs/example/agents/3f8e2a4c-1b2d-4e5f-8a9b-0c1d2e3f4a5b';
const SEARCH = '?attempt=7c9d0e1f-2a3b-4c5d-9e6f-7a8b9c0d1e2f.claude-code.0';
const HASH = '#event=7c9d0e1f-2a3b-4c5d-9e6f-7a8b9c0d1e2f.claude-code.0:e3b0c44298fc1c14:7:0';
const PLACE = { pathname: PATH, search: SEARCH, hash: HASH };
const NOW = 1_800_000_000_000;

let storage: MemoryStorage;
let visited: string[];
const navigate = (href: string) => visited.push(href);

beforeEach(() => {
  storage = new MemoryStorage();
  visited = [];
});

describe('signIn', () => {
  it('loads the complete address again', () => {
    expect(signIn(PLACE, storage, navigate, { now: NOW })).toBe(true);
    expect(visited).toEqual([PATH + SEARCH + HASH]);
  });

  it('does not loop on a server that keeps asking, unless the reader asks', () => {
    signIn(PLACE, storage, navigate, { now: NOW });
    expect(signIn(PLACE, storage, navigate, { now: NOW + 1 })).toBe(false);
    expect(signIn(PLACE, storage, navigate, { now: NOW + 1, force: true })).toBe(true);
    expect(signIn(PLACE, storage, navigate, { now: NOW + 1 + SIGN_IN_INTERVAL_MS })).toBe(true);
    expect(visited).toHaveLength(3);
  });

  it('does not go on its own without storage to guard it', () => {
    expect(signIn(PLACE, undefined, navigate, { now: NOW })).toBe(false);
    expect(visited).toEqual([]);
  });
});

describe('restoreDestination', () => {
  beforeEach(() => {
    signIn(PLACE, storage, navigate, { now: NOW });
  });

  it('puts back the fragment, or the query and fragment, the sign-in lost', () => {
    expect(restoreDestination({ ...PLACE, hash: '' }, storage, NOW + 1)).toBe(PATH + SEARCH + HASH);
  });

  it('puts back the query and fragment when both were lost', () => {
    expect(restoreDestination({ pathname: PATH, search: '', hash: '' }, storage, NOW + 1)).toBe(
      PATH + SEARCH + HASH,
    );
  });

  it('restores once', () => {
    restoreDestination({ ...PLACE, hash: '' }, storage, NOW + 1);
    expect(restoreDestination({ ...PLACE, hash: '' }, storage, NOW + 2)).toBeUndefined();
  });

  it('leaves another page, another query, or a stale destination alone', () => {
    expect(
      restoreDestination({ pathname: '/', search: '', hash: '' }, storage, NOW + 1),
    ).toBeUndefined();
    expect(
      restoreDestination({ ...PLACE, search: '?view=inspector', hash: '' }, storage, NOW + 1),
    ).toBeUndefined();
    expect(
      restoreDestination({ ...PLACE, hash: '' }, storage, NOW + DESTINATION_TTL_MS),
    ).toBeUndefined();
  });

  it('has nothing to do when the address came back whole', () => {
    expect(restoreDestination(PLACE, storage, NOW + 1)).toBeUndefined();
  });

  it('ignores a destination on another site', () => {
    storage.setItem(
      'lucarne.destination',
      JSON.stringify({ href: '//example.com/x', at: NOW, restored: false }),
    );
    expect(
      restoreDestination({ pathname: '/x', search: '', hash: '' }, storage, NOW + 1),
    ).toBeUndefined();
  });
});
