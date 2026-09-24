'use client';

import { Popover as RadixPopover } from 'radix-ui';
import type { ReactElement, ReactNode } from 'react';

import { cn } from '@/lib/cn';

export type PopoverProps = {
  trigger: ReactElement;
  children: ReactNode;
  align?: 'start' | 'center' | 'end';
  side?: 'top' | 'right' | 'bottom' | 'left';
  /** Accessible name of the popover dialog. */
  label?: string;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  className?: string;
};

export function Popover({ trigger, children, align = 'start', side = 'bottom', label, open, onOpenChange, className }: PopoverProps) {
  return (
    <RadixPopover.Root open={open} onOpenChange={onOpenChange}>
      <RadixPopover.Trigger asChild>{trigger}</RadixPopover.Trigger>
      <RadixPopover.Portal>
        <RadixPopover.Content
          align={align}
          side={side}
          sideOffset={6}
          collisionPadding={8}
          aria-label={label}
          className={cn(
            'z-50 max-w-[calc(100vw-16px)] rounded-md border border-border bg-surface-raised p-3 text-foreground shadow-md data-[state=open]:animate-popover-in data-[state=closed]:animate-popover-out',
            className,
          )}
        >
          {children}
        </RadixPopover.Content>
      </RadixPopover.Portal>
    </RadixPopover.Root>
  );
}
