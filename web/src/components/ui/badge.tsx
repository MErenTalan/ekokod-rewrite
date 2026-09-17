import type { ComponentPropsWithRef, ReactNode } from 'react';

import { cn } from '@/lib/cn';

export type BadgeTone = 'neutral' | 'brand' | 'info' | 'success' | 'warning' | 'danger';

// Text on a subtle tone is the tone itself or foreground: both pairs are contrast-checked (scripts/contrast-pairs.ts).
const tones: Record<BadgeTone, string> = {
  neutral: 'bg-surface-sunken text-foreground',
  brand: 'bg-primary-subtle text-primary',
  info: 'bg-info-subtle text-info',
  success: 'bg-success-subtle text-success',
  warning: 'bg-warning-subtle text-warning',
  danger: 'bg-danger-subtle text-danger',
};

export type BadgeProps = ComponentPropsWithRef<'span'> & { tone: BadgeTone; children: ReactNode };

export function Badge({ tone, className, ...props }: BadgeProps) {
  return (
    <span
      className={cn('inline-flex max-w-full items-center gap-1 rounded-sm px-1.5 py-0.5 type-caption whitespace-nowrap', tones[tone], className)}
      {...props}
    />
  );
}
