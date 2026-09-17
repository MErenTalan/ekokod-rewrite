'use client';

import { RadioGroup as RadixRadioGroup } from 'radix-ui';
import { useId } from 'react';

import { cn } from '@/lib/cn';

import type { FieldProps, Option } from './field';
import { FieldMessages } from './_field-messages';

export type RadioGroupProps = FieldProps & {
  options: Option[];
  value: string;
  onValueChange: (value: string) => void;
  orientation?: 'horizontal' | 'vertical';
};

/** The label is the fieldset legend, so screen readers announce it with every option. */
export function RadioGroup({ label, description, error, required, id, disabled, options, value, onValueChange, orientation = 'vertical' }: RadioGroupProps) {
  const generated = useId();
  const groupId = id ?? `radio-${generated}`;
  return (
    <fieldset className="flex min-w-0 flex-col gap-1.5" disabled={disabled}>
      <legend className="mb-1.5 text-foreground type-small font-semibold">{label}</legend>
      <RadixRadioGroup.Root
        id={groupId}
        value={value}
        onValueChange={onValueChange}
        orientation={orientation}
        required={required}
        aria-describedby={FieldMessages.describedBy(groupId, description, error)}
        className={cn('flex gap-x-4 gap-y-1', orientation === 'vertical' ? 'flex-col' : 'flex-row flex-wrap')}
      >
        {options.map((option) => {
          const optionId = `${groupId}-${option.value}`;
          return (
            <div key={option.value} data-touch-target className="flex items-center gap-2 pointer-coarse:min-h-11">
              <RadixRadioGroup.Item
                id={optionId}
                value={option.value}
                disabled={option.disabled}
                className="flex size-4 shrink-0 items-center justify-center rounded-full border border-border-control bg-surface transition-colors duration-(--duration-hover) data-[state=checked]:border-primary disabled:opacity-60"
              >
                <RadixRadioGroup.Indicator className="block size-2 rounded-full bg-primary" />
              </RadixRadioGroup.Item>
              <label htmlFor={optionId} className={cn('text-foreground type-body select-none', option.disabled && 'opacity-60')}>
                {option.label}
              </label>
            </div>
          );
        })}
      </RadixRadioGroup.Root>
      <FieldMessages controlId={groupId} description={description} error={error} />
    </fieldset>
  );
}
