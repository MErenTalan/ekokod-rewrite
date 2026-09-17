'use client';

import { createContext, createElement, useContext, useMemo, useSyncExternalStore, type ReactNode } from 'react';

export type Selection = { companyId?: string; buildingId?: string; analyzerId?: string };

export type SelectionStore = {
  get: () => Selection;
  set: (patch: Partial<Selection>) => void;
  subscribe: (listener: () => void) => () => void;
};

const KEYS = ['companyId', 'buildingId', 'analyzerId'] as const;
const EMPTY: Selection = {};

export const selectionKey = (userId: string) => `ekokod:selection:${userId}`;

function read(storage: Storage | undefined, key: string): Selection {
  try {
    const raw = storage?.getItem(key);
    const parsed: unknown = raw ? JSON.parse(raw) : null;
    if (!parsed || typeof parsed !== 'object') return EMPTY;
    const out: Selection = {};
    for (const k of KEYS) {
      const v = (parsed as Record<string, unknown>)[k];
      if (typeof v === 'string' && v) out[k] = v;
    }
    return out;
  } catch {
    return EMPTY;
  }
}

/** R170: one remembered selection per user; switching company clears the building and analyzer. */
export function createSelectionStore(userId: string, storage: Storage | undefined): SelectionStore {
  const key = selectionKey(userId);
  let current = read(storage, key);
  const listeners = new Set<() => void>();
  return {
    get: () => current,
    set: (patch) => {
      let next: Selection = { ...current, ...patch };
      if ('companyId' in patch && patch.companyId !== current.companyId) next = { companyId: patch.companyId, ...pick(patch) };
      if ('buildingId' in patch && patch.buildingId !== current.buildingId && !('analyzerId' in patch)) delete next.analyzerId;
      for (const k of KEYS) if (!next[k]) delete next[k];
      current = next;
      try {
        storage?.setItem(key, JSON.stringify(current));
      } catch {
        // Private mode or quota: the selection still lives for this page.
      }
      listeners.forEach((l) => l());
    },
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
}

function pick(patch: Partial<Selection>): Selection {
  const out: Selection = {};
  if (patch.buildingId) out.buildingId = patch.buildingId;
  if (patch.analyzerId) out.analyzerId = patch.analyzerId;
  return out;
}

type SelectionContextValue = { store: SelectionStore; isAdmin: boolean };
const SelectionContext = createContext<SelectionContextValue | null>(null);

export function SelectionProvider({ userId, isAdmin, children }: { userId: string; isAdmin: boolean; children: ReactNode }) {
  const store = useMemo(() => createSelectionStore(userId, typeof window === 'undefined' ? undefined : window.localStorage), [userId]);
  const value = useMemo(() => ({ store, isAdmin }), [store, isAdmin]);
  return createElement(SelectionContext.Provider, { value }, children);
}

function useStore(): SelectionContextValue {
  const value = useContext(SelectionContext);
  if (!value) throw new Error('useSelection must be used inside <SessionProvider>');
  return value;
}

export function useSelection(): Selection & { set: (patch: Partial<Selection>) => void } {
  const { store } = useStore();
  const selection = useSyncExternalStore(store.subscribe, store.get, () => EMPTY);
  return { ...selection, set: store.set };
}

/** Query parameters every scoped API call adds: an admin acting for another company sends `company_id` (R139). */
export function useScopeParams(): { company_id?: string } {
  const { isAdmin } = useStore();
  const { companyId } = useSelection();
  return isAdmin && companyId ? { company_id: companyId } : {};
}
