import type { ComponentPropsWithRef } from 'react';

import { cn } from '@/lib/cn';

/** In shadow mode (customiser) dark shadows vanish, so `card-edge` draws the edge instead (plan D4, D15). */
export function Card({ className, ...props }: ComponentPropsWithRef<'div'>) {
  return (
    <div
      className={cn(
        'flex flex-col rounded-lg border border-border bg-surface-raised text-foreground in-data-[card=shadow]:border-card-edge in-data-[card=shadow]:shadow-md',
        className,
      )}
      {...props}
    />
  );
}

export function CardHeader({ className, ...props }: ComponentPropsWithRef<'div'>) {
  return <div className={cn('flex flex-col gap-1 px-4 pt-4', className)} {...props} />;
}

export function CardTitle({ as: Tag = 'h3', className, ...props }: ComponentPropsWithRef<'h3'> & { as?: 'h2' | 'h3' }) {
  return <Tag className={cn('text-foreground type-h3', className)} {...props} />;
}

export function CardDescription({ className, ...props }: ComponentPropsWithRef<'p'>) {
  return <p className={cn('text-foreground-muted type-small', className)} {...props} />;
}

export function CardContent({ className, ...props }: ComponentPropsWithRef<'div'>) {
  return <div className={cn('px-4 py-4', className)} {...props} />;
}

export function CardFooter({ className, ...props }: ComponentPropsWithRef<'div'>) {
  return <div className={cn('flex items-center gap-2 border-t border-border px-4 py-3', className)} {...props} />;
}
