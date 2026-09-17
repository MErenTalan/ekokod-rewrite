'use client';

import { AlertCircle } from 'lucide-react';
import { useEffect, useId, useRef } from 'react';

export type FormErrorSummaryProps = { errors: { fieldId: string; message: string }[]; title: string };

/** Errors at the top of the form, linked to their fields; takes focus when errors first appear (07 §9). */
export function FormErrorSummary({ errors, title }: FormErrorSummaryProps) {
  const ref = useRef<HTMLDivElement>(null);
  const previous = useRef(0);
  const titleId = useId();
  useEffect(() => {
    if (previous.current === 0 && errors.length > 0) ref.current?.focus();
    previous.current = errors.length;
  }, [errors.length]);
  if (errors.length === 0) return null;
  return (
    <div ref={ref} role="alert" tabIndex={-1} aria-labelledby={titleId} className="rounded-md border border-danger bg-danger-subtle p-4">
      <p id={titleId} className="flex items-center gap-2 text-danger type-h3">
        <AlertCircle aria-hidden className="size-5 shrink-0" />
        {title}
      </p>
      <ul className="mt-2 flex list-disc flex-col gap-1 ps-9 text-foreground">
        {errors.map((e) => (
          <li key={e.fieldId}>
            <a href={`#${e.fieldId}`} className="text-foreground underline underline-offset-2 pointer-coarse:inline-flex pointer-coarse:min-h-11 pointer-coarse:items-center">
              {e.message}
            </a>
          </li>
        ))}
      </ul>
    </div>
  );
}
