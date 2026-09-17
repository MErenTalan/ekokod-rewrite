'use client';

import { Tooltip } from 'radix-ui';
import type { ReactNode } from 'react';

/** Client providers shared by the app, Storybook and component tests. */
export function AppProviders({ children }: { children: ReactNode }) {
  return <Tooltip.Provider delayDuration={300}>{children}</Tooltip.Provider>;
}
