'use client';

import { Children, type ReactNode } from 'react';

import { cn } from '@/lib/cn';

/**
 * A card grid whose children fade in one after another (07 §8, plan D26): each
 * child carries its index, and the CSS turns that into a delay of
 * `--duration-stagger-step`. Under `prefers-reduced-motion` nothing animates.
 */
export function StaggerGrid({ className, children }: { className?: string; children: ReactNode }) {
  return (
    <div className={cn('grid gap-4', className)}>
      {Children.map(children, (child, index) => (
        <div className="stagger-item min-w-0" style={{ ['--i' as string]: index }}>
          {child}
        </div>
      ))}
    </div>
  );
}
