'use client';

import { createContext, useContext, useMemo, type ReactNode } from 'react';

import { usePreferenceSync } from '@/features/preferences/sync-preferences';
import type { MeResponse } from '@/lib/api/errors';
import { SelectionProvider } from '@/lib/selection/selection-store';

import type { Permission } from './permissions';

type Session = { me: MeResponse; can: (permission: Permission) => boolean };

const SessionContext = createContext<Session | null>(null);

/** The signed-in user from the server layout's /auth/me; permissions only shape the UI, the API enforces them. */
export function SessionProvider({ me, children }: { me: MeResponse; children: ReactNode }) {
  const value = useMemo<Session>(() => ({ me, can: (p) => me.permissions.includes(p) }), [me]);
  return (
    <SessionContext.Provider value={value}>
      <SelectionProvider userId={me.id} isAdmin={me.role === 'admin'}>
        <PreferenceSync me={me} />
        {children}
      </SelectionProvider>
    </SessionContext.Provider>
  );
}

function PreferenceSync({ me }: { me: MeResponse }) {
  usePreferenceSync(me);
  return null;
}

export function useSession(): Session {
  const value = useContext(SessionContext);
  if (!value) throw new Error('useSession must be used inside <SessionProvider>');
  return value;
}

/** For components that also render without a session (Storybook, the gallery). */
export function useOptionalSession(): Session | null {
  return useContext(SessionContext);
}
