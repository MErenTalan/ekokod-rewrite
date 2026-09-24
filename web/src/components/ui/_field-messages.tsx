import { AlertCircle } from 'lucide-react';

import { cn } from '@/lib/cn';

type Props = { controlId: string; description?: string; error?: string; className?: string };

/** Description and error lines for controls whose label is not rendered by `Field` (checkbox, switch rows). */
export function FieldMessages({ controlId, description, error, className }: Props) {
  if (!description && !error) return null;
  return (
    <div className={cn('flex flex-col gap-1', className)}>
      {description ? (
        <p id={`${controlId}-description`} className="text-foreground-muted type-small">
          {description}
        </p>
      ) : null}
      {error ? (
        <p id={`${controlId}-error`} className="flex items-start gap-1.5 text-danger type-small">
          <AlertCircle aria-hidden className="mt-0.5 size-3.5 shrink-0" />
          <span>{error}</span>
        </p>
      ) : null}
    </div>
  );
}

FieldMessages.describedBy = (controlId: string, description?: string, error?: string) =>
  [description && `${controlId}-description`, error && `${controlId}-error`].filter(Boolean).join(' ') || undefined;
