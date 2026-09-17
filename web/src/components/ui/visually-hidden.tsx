import type { ComponentPropsWithRef } from 'react';

import { cn } from '@/lib/cn';

/** Text for assistive technology only; stays in the accessibility tree. */
export function VisuallyHidden({ className, ...props }: ComponentPropsWithRef<'span'>) {
  return <span className={cn('sr-only', className)} {...props} />;
}
