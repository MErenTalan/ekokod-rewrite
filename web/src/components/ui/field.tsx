'use client';

import { AlertCircle } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useId, type ReactElement, type ReactNode } from 'react';

import { cn } from '@/lib/cn';

export type FieldProps = {
  label: string;
  description?: string;
  error?: string;
  required?: boolean;
  id?: string;
  disabled?: boolean;
};
export type Option = { value: string; label: string; disabled?: boolean };
export type FieldIds = { controlId: string; labelId: string; describedBy: string | undefined; invalid: boolean };

/** Shared control look: 36 px tall, 44 px under a coarse pointer (plan D13), control border at 3:1 (D3). */
export const controlClasses =
  'h-9 w-full rounded-md border border-border-control bg-surface px-3 text-foreground type-body ' +
  'placeholder:text-foreground-subtle transition-colors duration-(--duration-hover) ease-out ' +
  'disabled:opacity-60 aria-invalid:border-danger pointer-coarse:min-h-11';

/** Visible label, description and error wired to the control through ids (07 §9: placeholders are never labels). */
export function Field({
  label,
  description,
  error,
  required,
  id,
  disabled,
  labelVisibility = 'visible',
  as = 'label',
  className,
  children,
}: FieldProps & {
  labelVisibility?: 'visible' | 'hidden';
  /** `span` for controls a <label> cannot name (slider thumbs); they use `labelId` via aria-labelledby. */
  as?: 'label' | 'span';
  className?: string;
  children: (ids: FieldIds) => ReactNode;
}): ReactElement {
  const t = useTranslations('forms');
  const generated = useId();
  const controlId = id ?? `field-${generated}`;
  const labelId = `${controlId}-label`;
  const descriptionId = description ? `${controlId}-description` : undefined;
  const errorId = error ? `${controlId}-error` : undefined;
  const describedBy = [descriptionId, errorId].filter(Boolean).join(' ') || undefined;
  const Label = as;
  return (
    <div className={cn('flex min-w-0 flex-col gap-1.5', className)}>
      <Label
        id={labelId}
        htmlFor={as === 'label' ? controlId : undefined}
        className={cn(
          'type-small font-semibold',
          // Muted, not faded: opacity on text would break contrast (07 §9).
          disabled ? 'text-foreground-muted' : 'text-foreground',
          labelVisibility === 'hidden' && 'sr-only',
        )}
      >
        {label}
        {required ? <span className="ms-1 font-normal text-foreground-muted">({t('required')})</span> : null}
      </Label>
      {children({ controlId, labelId, describedBy, invalid: Boolean(error) })}
      {description ? (
        <p id={descriptionId} className="text-foreground-muted type-small">
          {description}
        </p>
      ) : null}
      {error ? (
        <p id={errorId} className="flex items-start gap-1.5 text-danger type-small">
          <AlertCircle aria-hidden className="mt-0.5 size-3.5 shrink-0" />
          <span>{error}</span>
        </p>
      ) : null}
    </div>
  );
}
