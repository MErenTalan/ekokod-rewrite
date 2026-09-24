'use client';

import type { ComponentPropsWithRef } from 'react';

import { cn } from '@/lib/cn';

import { controlClasses, Field, type FieldProps } from './field';

export type TextareaProps = FieldProps & Omit<ComponentPropsWithRef<'textarea'>, 'id'>;

export function Textarea({ label, description, error, required, id, disabled, rows = 4, className, ...props }: TextareaProps) {
  return (
    <Field label={label} description={description} error={error} required={required} id={id} disabled={disabled}>
      {({ controlId, describedBy, invalid }) => (
        <textarea
          id={controlId}
          rows={rows}
          aria-describedby={describedBy}
          aria-invalid={invalid || undefined}
          required={required}
          disabled={disabled}
          className={cn(controlClasses, 'h-auto py-2', className)}
          {...props}
        />
      )}
    </Field>
  );
}
