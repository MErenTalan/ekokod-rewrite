'use client';

import { createContext, useCallback, useContext, useState, type ReactNode } from 'react';

type Politeness = 'polite' | 'assertive';
type Announce = (message: string, politeness?: Politeness) => void;

const AnnounceContext = createContext<Announce>(() => {});

/** Two persistent live regions (07 §9); no role, so they never collide with Alert/Toast in queries (plan I-9). */
export function LiveAnnouncerProvider({ children }: { children: ReactNode }) {
  const [messages, setMessages] = useState<Record<Politeness, string>>({ polite: '', assertive: '' });
  const announce = useCallback<Announce>((message, politeness = 'polite') => {
    // Clear first so repeating the same message is announced again.
    setMessages((m) => ({ ...m, [politeness]: '' }));
    setTimeout(() => setMessages((m) => ({ ...m, [politeness]: message })), 50);
  }, []);
  return (
    <AnnounceContext.Provider value={announce}>
      {children}
      <div aria-live="polite" aria-atomic="true" className="sr-only">
        {messages.polite}
      </div>
      <div aria-live="assertive" aria-atomic="true" className="sr-only">
        {messages.assertive}
      </div>
    </AnnounceContext.Provider>
  );
}

export function useAnnounce(): Announce {
  return useContext(AnnounceContext);
}
