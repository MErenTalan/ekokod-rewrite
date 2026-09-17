'use client';

import { Check, ChevronDown } from 'lucide-react';
import { Select as RadixSelect } from 'radix-ui';

import { cn } from '@/lib/cn';

import { controlClasses, Field, type FieldProps, type Option } from './field';

export type SelectProps = FieldProps & {
  options: Option[];
  value: string | null;
  onValueChange: (value: string) => void;
  placeholder?: string;
};

export function Select({ label, description, error, required, id, disabled, options, value, onValueChange, placeholder }: SelectProps) {
  return (
    <Field label={label} description={description} error={error} required={required} id={id} disabled={disabled}>
      {({ controlId, describedBy, invalid }) => (
        <RadixSelect.Root value={value ?? ''} onValueChange={onValueChange} disabled={disabled} required={required}>
          <RadixSelect.Trigger
            id={controlId}
            aria-describedby={describedBy}
            aria-invalid={invalid || undefined}
            className={cn(controlClasses, 'flex items-center justify-between gap-2 text-start data-[placeholder]:text-foreground-subtle')}
          >
            <RadixSelect.Value placeholder={placeholder} />
            <RadixSelect.Icon>
              <ChevronDown aria-hidden className="size-4 text-foreground-muted" />
            </RadixSelect.Icon>
          </RadixSelect.Trigger>
          <RadixSelect.Portal>
            <RadixSelect.Content
              position="popper"
              sideOffset={4}
              className="z-50 max-h-(--radix-select-content-available-height) min-w-(--radix-select-trigger-width) overflow-hidden rounded-md border border-border bg-surface-raised text-foreground shadow-md data-[state=open]:animate-popover-in data-[state=closed]:animate-popover-out"
            >
              <RadixSelect.Viewport className="p-1">
                {options.map((option) => (
                  <RadixSelect.Item
                    key={option.value}
                    value={option.value}
                    disabled={option.disabled}
                    className="relative flex h-8 items-center rounded-sm ps-7 pe-3 type-body select-none data-[disabled]:opacity-60 data-[highlighted]:bg-surface-sunken pointer-coarse:min-h-11"
                  >
                    <RadixSelect.ItemIndicator className="absolute start-2">
                      <Check aria-hidden className="size-4 text-primary" />
                    </RadixSelect.ItemIndicator>
                    <RadixSelect.ItemText>{option.label}</RadixSelect.ItemText>
                  </RadixSelect.Item>
                ))}
              </RadixSelect.Viewport>
            </RadixSelect.Content>
          </RadixSelect.Portal>
        </RadixSelect.Root>
      )}
    </Field>
  );
}
