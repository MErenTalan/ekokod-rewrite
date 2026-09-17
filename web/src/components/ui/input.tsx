'use client';

import type { ComponentPropsWithRef } from 'react';

import { cn } from '@/lib/cn';

import { controlClasses, Field, type FieldProps } from './field';

export type InputProps = FieldProps & Omit<ComponentPropsWithRef<'input'>, 'id'>;

export function Input({ label, description, error, required, id, disabled, className, ...props }: InputProps) {
  return (
    <Field label={label} description={description} error={error} required={required} id={id} disabled={disabled}>
      {({ controlId, describedBy, invalid }) => (
        <input
          id={controlId}
          aria-describedby={describedBy}
          aria-invalid={invalid || undefined}
          required={required}
          disabled={disabled}
          className={cn(controlClasses, className)}
          {...props}
        />
      )}
    </Field>
  );
}
