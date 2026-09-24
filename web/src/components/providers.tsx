'use client';

import { QueryClientProvider } from '@tanstack/react-query';
import { Tooltip } from 'radix-ui';
import { useState, type ReactNode } from 'react';

import { makeQueryClient } from '@/lib/api/query';
import { QueryErrorToaster } from '@/lib/api/query-errors';

import { LiveAnnouncerProvider } from './ui/live-announcer';
import { Toaster } from './ui/toast';

/** Client providers shared by the app, Storybook and component tests. */
export function AppProviders({ children }: { children: ReactNode }) {
  const [queryClient] = useState(makeQueryClient);
  return (
    <QueryClientProvider client={queryClient}>
      <Tooltip.Provider delayDuration={300}>
        <LiveAnnouncerProvider>
          <Toaster>
            <QueryErrorToaster />
            {children}
          </Toaster>
        </LiveAnnouncerProvider>
      </Tooltip.Provider>
    </QueryClientProvider>
  );
}
