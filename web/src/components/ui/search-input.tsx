'use client';

import { Search, X } from 'lucide-react';
import { useTranslations } from 'next-intl';
import type { ComponentPropsWithRef } from 'react';

import { cn } from '@/lib/cn';

import { controlClasses, Field, type FieldProps } from './field';

export type SearchInputProps = FieldProps &
  Omit<ComponentPropsWithRef<'input'>, 'id' | 'value' | 'onChange' | 'type'> & {
    value: string;
    onValueChange: (value: string) => void;
    /** `hidden` only for the top-bar global search (plan D14). */
    labelVisibility?: 'visible' | 'hidden';
  };

export function SearchInput({ label, description, error, required, id, disabled, value, onValueChange, labelVisibility, className, ...props }: SearchInputProps) {
  const t = useTranslations('forms');
  return (
    <Field label={label} description={description} error={error} required={required} id={id} disabled={disabled} labelVisibility={labelVisibility}>
      {({ controlId, describedBy, invalid }) => (
        <div className="relative">
          <Search aria-hidden className="pointer-events-none absolute start-3 top-1/2 size-4 -translate-y-1/2 text-foreground-muted" />
          <input
            id={controlId}
            type="search"
            value={value}
            onChange={(e) => onValueChange(e.target.value)}
            aria-describedby={describedBy}
            aria-invalid={invalid || undefined}
            disabled={disabled}
            className={cn(controlClasses, 'ps-9 pe-9 [&::-webkit-search-cancel-button]:appearance-none', className)}
            {...props}
          />
          {value && !disabled ? (
            <button
              type="button"
              onClick={() => onValueChange('')}
              aria-label={t('clear')}
              className="absolute end-1 top-1/2 inline-flex size-7 -translate-y-1/2 items-center justify-center rounded-sm text-foreground-muted hover:bg-surface-sunken hover:text-foreground pointer-coarse:size-11"
            >
              <X aria-hidden className="size-4" />
            </button>
          ) : null}
        </div>
      )}
    </Field>
  );
}
