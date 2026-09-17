'use client';

import { Check, Minus } from 'lucide-react';
import { Checkbox as RadixCheckbox } from 'radix-ui';
import { useId } from 'react';

import { cn } from '@/lib/cn';

import type { FieldProps } from './field';
import { FieldMessages } from './_field-messages';

export type CheckboxProps = FieldProps & {
  checked: boolean | 'indeterminate';
  onCheckedChange: (checked: boolean) => void;
};

export function Checkbox({ label, description, error, required, id, disabled, checked, onCheckedChange }: CheckboxProps) {
  const generated = useId();
  const controlId = id ?? `checkbox-${generated}`;
  return (
    <div className="flex flex-col gap-1">
      <div data-touch-target className="flex items-center gap-2 pointer-coarse:min-h-11">
        <RadixCheckbox.Root
          id={controlId}
          checked={checked}
          onCheckedChange={(v) => onCheckedChange(v === true)}
          disabled={disabled}
          required={required}
          aria-invalid={Boolean(error) || undefined}
          aria-describedby={FieldMessages.describedBy(controlId, description, error)}
          className="flex size-4 shrink-0 items-center justify-center rounded-sm border border-border-control bg-surface text-on-primary transition-colors duration-(--duration-hover) data-[state=checked]:border-primary data-[state=checked]:bg-primary data-[state=indeterminate]:border-primary data-[state=indeterminate]:bg-primary disabled:opacity-60 aria-invalid:border-danger"
        >
          <RadixCheckbox.Indicator>
            {checked === 'indeterminate' ? <Minus aria-hidden className="size-3" /> : <Check aria-hidden className="size-3" />}
          </RadixCheckbox.Indicator>
        </RadixCheckbox.Root>
        <label htmlFor={controlId} className={cn('text-foreground type-body select-none', disabled && 'opacity-60')}>
          {label}
        </label>
      </div>
      <FieldMessages controlId={controlId} description={description} error={error} className="ps-6" />
    </div>
  );
}
