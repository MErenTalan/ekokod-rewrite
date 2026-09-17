'use client';

import { Switch as RadixSwitch } from 'radix-ui';
import { useId } from 'react';

import { cn } from '@/lib/cn';

import type { FieldProps } from './field';
import { FieldMessages } from './_field-messages';

export type SwitchProps = FieldProps & { checked: boolean; onCheckedChange: (checked: boolean) => void };

export function Switch({ label, description, error, required, id, disabled, checked, onCheckedChange }: SwitchProps) {
  const generated = useId();
  const controlId = id ?? `switch-${generated}`;
  return (
    <div className="flex flex-col gap-1">
      <div data-touch-target className="flex items-center justify-between gap-3 pointer-coarse:min-h-11">
        <label htmlFor={controlId} className={cn('text-foreground type-body select-none', disabled && 'opacity-60')}>
          {label}
        </label>
        <RadixSwitch.Root
          id={controlId}
          checked={checked}
          onCheckedChange={onCheckedChange}
          disabled={disabled}
          required={required}
          aria-invalid={Boolean(error) || undefined}
          aria-describedby={FieldMessages.describedBy(controlId, description, error)}
          className="relative inline-flex h-5 w-9 shrink-0 items-center rounded-full border border-border-control bg-surface-sunken transition-colors duration-(--duration-hover) data-[state=checked]:border-primary data-[state=checked]:bg-primary disabled:opacity-60"
        >
          <RadixSwitch.Thumb className="block size-3.5 translate-x-0.5 rounded-full bg-foreground-muted transition-transform duration-(--duration-hover) data-[state=checked]:translate-x-4 data-[state=checked]:bg-on-primary rtl:data-[state=checked]:-translate-x-4" />
        </RadixSwitch.Root>
      </div>
      <FieldMessages controlId={controlId} description={description} error={error} />
    </div>
  );
}
