'use client';

import { Tooltip as RadixTooltip } from 'radix-ui';
import type { ReactElement, ReactNode } from 'react';

export type TooltipProps = {
  content: ReactNode;
  side?: 'top' | 'right' | 'bottom' | 'left';
  children: ReactElement;
};

export function Tooltip({ content, side = 'top', children }: TooltipProps) {
  return (
    <RadixTooltip.Root>
      <RadixTooltip.Trigger asChild>{children}</RadixTooltip.Trigger>
      <RadixTooltip.Portal>
        <RadixTooltip.Content
          side={side}
          sideOffset={6}
          collisionPadding={8}
          className="z-50 max-w-72 rounded-md border border-border bg-surface-raised px-2.5 py-1.5 text-foreground type-small shadow-md data-[state=closed]:animate-popover-out data-[state=delayed-open]:animate-popover-in data-[state=instant-open]:animate-popover-in"
        >
          {content}
        </RadixTooltip.Content>
      </RadixTooltip.Portal>
    </RadixTooltip.Root>
  );
}
