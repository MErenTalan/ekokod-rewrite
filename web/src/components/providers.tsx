'use client';

import { QueryClientProvider } from '@tanstack/react-query';
import { Tooltip } from 'radix-ui';
import { useState, type ReactNode } from 'react';

import { makeQueryClient } from '@/lib/api/query';

import { LiveAnnouncerProvider } from './ui/live-announcer';
import { Toaster } from './ui/toast';

/** Client providers shared by the app, Storybook and component tests. */
export function AppProviders({ children }: { children: ReactNode }) {
  const [queryClient] = useState(makeQueryClient);
  return (
    <QueryClientProvider client={queryClient}>
      <Tooltip.Provider delayDuration={300}>
        <LiveAnnouncerProvider>
          <Toaster>{children}</Toaster>
        </LiveAnnouncerProvider>
      </Tooltip.Provider>
    </QueryClientProvider>
  );
}
