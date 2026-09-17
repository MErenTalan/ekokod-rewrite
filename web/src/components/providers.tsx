'use client';

import { Tooltip } from 'radix-ui';
import type { ReactNode } from 'react';

import { LiveAnnouncerProvider } from './ui/live-announcer';
import { Toaster } from './ui/toast';

/** Client providers shared by the app, Storybook and component tests. */
export function AppProviders({ children }: { children: ReactNode }) {
  return (
    <Tooltip.Provider delayDuration={300}>
      <LiveAnnouncerProvider>
        <Toaster>{children}</Toaster>
      </LiveAnnouncerProvider>
    </Tooltip.Provider>
  );
}
