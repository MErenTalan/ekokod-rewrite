import type { CSSProperties } from 'react';

import { cn } from '@/lib/cn';

/** Size comes from the caller so the skeleton matches the final layout (07 §11: no layout shift). */
export function Skeleton({ className, style }: { className?: string; style?: CSSProperties }) {
  return <div aria-hidden className={cn('animate-pulse rounded-md bg-surface-sunken', className)} style={style} />;
}

export function SkeletonText({ lines, className }: { lines: number; className?: string }) {
  return (
    <div aria-hidden className={cn('flex flex-col gap-2', className)}>
      {Array.from({ length: lines }, (_, i) => (
        <div key={i} data-skeleton-line className={cn('h-4 animate-pulse rounded-sm bg-surface-sunken', i === lines - 1 && lines > 1 ? 'w-2/3' : 'w-full')} />
      ))}
    </div>
  );
}
