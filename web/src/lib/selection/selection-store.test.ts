import { describe, expect, it, vi } from 'vitest';

import { createSelectionStore, selectionKey } from './selection-store';

function memoryStorage(initial: Record<string, string> = {}): Storage {
  const data = new Map(Object.entries(initial));
  return {
    get length() {
      return data.size;
    },
    clear: () => data.clear(),
    getItem: (k) => data.get(k) ?? null,
    key: (i) => [...data.keys()][i] ?? null,
    removeItem: (k) => void data.delete(k),
    setItem: (k, v) => void data.set(k, v),
  };
}

describe('selection store', () => {
  it('persists per user and never leaks between users', () => {
    const storage = memoryStorage();
    const ayse = createSelectionStore('ayse', storage);
    ayse.set({ buildingId: 'b1', analyzerId: 'a1' });
    expect(JSON.parse(storage.getItem(selectionKey('ayse'))!)).toEqual({ buildingId: 'b1', analyzerId: 'a1' });
    expect(createSelectionStore('mehmet', storage).get()).toEqual({});
    expect(createSelectionStore('ayse', storage).get()).toEqual({ buildingId: 'b1', analyzerId: 'a1' });
  });

  it('switching company clears building and analyzer', () => {
    const store = createSelectionStore('admin', memoryStorage());
    store.set({ companyId: 'c1', buildingId: 'b1', analyzerId: 'a1' });
    store.set({ companyId: 'c2' });
    expect(store.get()).toEqual({ companyId: 'c2' });
    store.set({ companyId: undefined });
    expect(store.get()).toEqual({});
  });

  it('switching building clears the analyzer unless one is given', () => {
    const store = createSelectionStore('u', memoryStorage());
    store.set({ buildingId: 'b1', analyzerId: 'a1' });
    store.set({ buildingId: 'b2' });
    expect(store.get()).toEqual({ buildingId: 'b2' });
    store.set({ buildingId: 'b3', analyzerId: 'a3' });
    expect(store.get()).toEqual({ buildingId: 'b3', analyzerId: 'a3' });
  });

  it('ignores corrupted or foreign stored values', () => {
    expect(createSelectionStore('u', memoryStorage({ [selectionKey('u')]: '{not json' })).get()).toEqual({});
    expect(createSelectionStore('u', memoryStorage({ [selectionKey('u')]: '{"buildingId":7,"x":"y"}' })).get()).toEqual({});
  });

  it('notifies subscribers and survives a storage that throws', () => {
    const storage = memoryStorage();
    storage.setItem = () => {
      throw new Error('quota');
    };
    const store = createSelectionStore('u', storage);
    const listener = vi.fn();
    store.subscribe(listener);
    store.set({ buildingId: 'b1' });
    expect(listener).toHaveBeenCalledTimes(1);
    expect(store.get()).toEqual({ buildingId: 'b1' });
  });
});
