'use client';

import { useSyncExternalStore } from 'react';

/** Only for behaviour (closing an open drawer at lg); layout itself is CSS-first (plan I-16). */
export function useMediaQuery(query: string): boolean {
  return useSyncExternalStore(
    (callback) => {
      const mql = window.matchMedia(query);
      mql.addEventListener('change', callback);
      return () => mql.removeEventListener('change', callback);
    },
    () => window.matchMedia(query).matches,
    () => false,
  );
}
