import type { ComponentPropsWithRef } from 'react';

import { cn } from '@/lib/cn';

/**
 * Tables scroll inside their own focusable region; the page never scrolls
 * sideways (07 §7). `contain-paint` is what keeps that promise: overflow alone
 * clips the table visually but still leaves its width in the document's scroll
 * area, so a 29-column table let the whole page scroll into emptiness.
 */
export function TableContainer({ label, className, ...props }: ComponentPropsWithRef<'div'> & { label: string }) {
  return <div role="region" aria-label={label} tabIndex={0} className={cn('w-full contain-paint overflow-auto rounded-lg border border-border bg-surface-raised', className)} {...props} />;
}

export function Table({ className, ...props }: ComponentPropsWithRef<'table'>) {
  return <table className={cn('w-full border-collapse text-foreground type-body', className)} {...props} />;
}

export function TableCaption({ className, ...props }: ComponentPropsWithRef<'caption'>) {
  return <caption className={cn('sr-only', className)} {...props} />;
}

export function TableHeader({ className, ...props }: ComponentPropsWithRef<'thead'>) {
  return <thead className={cn('bg-surface-sunken', className)} {...props} />;
}

export function TableBody({ className, ...props }: ComponentPropsWithRef<'tbody'>) {
  return <tbody className={cn('[&>tr:last-child]:border-0', className)} {...props} />;
}

export function TableFooter({ className, ...props }: ComponentPropsWithRef<'tfoot'>) {
  return <tfoot className={cn('border-t border-border-strong bg-surface-sunken font-semibold', className)} {...props} />;
}

export function TableRow({ className, ...props }: ComponentPropsWithRef<'tr'>) {
  return <tr className={cn('border-b border-border', className)} {...props} />;
}

export function TableHead({ numeric, className, ...props }: ComponentPropsWithRef<'th'> & { numeric?: boolean }) {
  return (
    <th
      scope="col"
      className={cn('h-10 px-3 text-start align-middle text-foreground-muted type-caption whitespace-nowrap', numeric && 'text-end', className)}
      {...props}
    />
  );
}

export function TableCell({ numeric, className, ...props }: ComponentPropsWithRef<'td'> & { numeric?: boolean }) {
  return <td className={cn('px-3 py-2 align-middle', numeric && 'text-end type-data whitespace-nowrap', className)} {...props} />;
}
