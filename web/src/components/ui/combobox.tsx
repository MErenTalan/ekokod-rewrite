'use client';

import { Command } from 'cmdk';
import { Check, ChevronsUpDown } from 'lucide-react';
import { useId, useState } from 'react';

import { cn } from '@/lib/cn';

import { controlClasses, Field, type FieldProps, type Option } from './field';
import { Popover } from './popover';

export type ComboboxProps = FieldProps & {
  options: Option[];
  value: string | null;
  onValueChange: (value: string | null) => void;
  searchPlaceholder: string;
  emptyText: string;
};

/** Case-folds Turkish text so `istanbul`, `ISTANBUL` and `ıstanbul` all match `İstanbul` (plan I-11). */
export function normaliseTurkish(text: string): string {
  return text.toLocaleLowerCase('tr').replaceAll('ı', 'i');
}

export function filterOptions(options: Option[], query: string): Option[] {
  const q = normaliseTurkish(query.trim());
  return q ? options.filter((o) => normaliseTurkish(o.label).includes(q)) : options;
}

export const listClasses = 'max-h-64 overflow-y-auto p-1';
export const itemClasses =
  'flex cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-foreground type-body select-none ' +
  'data-[selected=true]:bg-surface-sunken data-[disabled=true]:cursor-not-allowed data-[disabled=true]:opacity-60 pointer-coarse:min-h-11';
export const searchClasses =
  'h-9 w-full border-b border-border bg-transparent px-3 text-foreground type-body placeholder:text-foreground-subtle pointer-coarse:min-h-11';

export function Combobox({ label, description, error, required, id, disabled, options, value, onValueChange, searchPlaceholder, emptyText }: ComboboxProps) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const listId = useId();
  const selected = options.find((o) => o.value === value);
  const onOpenChange = (next: boolean) => {
    setOpen(next);
    if (!next) setQuery('');
  };
  return (
    <Field label={label} description={description} error={error} required={required} id={id} disabled={disabled}>
      {({ controlId, describedBy, invalid }) => (
        <Popover
          open={open}
          onOpenChange={onOpenChange}
          label={label}
          className="w-(--radix-popover-trigger-width) min-w-56 p-0"
          trigger={
            <button
              id={controlId}
              type="button"
              role="combobox"
              aria-expanded={open}
              aria-haspopup="listbox"
              aria-controls={open ? listId : undefined}
              aria-describedby={describedBy}
              aria-invalid={invalid || undefined}
              disabled={disabled}
              className={cn(controlClasses, 'flex items-center justify-between gap-2 text-start')}
            >
              <span className={cn('truncate', !selected && 'text-foreground-subtle')}>
                {selected?.label ?? searchPlaceholder}
              </span>
              <ChevronsUpDown aria-hidden className="size-4 shrink-0 text-foreground-muted" />
            </button>
          }
        >
          <Command shouldFilter={false} label={searchPlaceholder} className="flex flex-col">
            <Command.Input value={query} onValueChange={setQuery} placeholder={searchPlaceholder} className={searchClasses} />
            <Command.List id={listId} label={label} className={listClasses}>
              <Command.Empty className="px-2 py-3 text-center text-foreground-muted type-small">{emptyText}</Command.Empty>
              {filterOptions(options, query).map((option) => (
                <Command.Item
                  key={option.value}
                  value={option.value}
                  disabled={option.disabled}
                  onSelect={() => {
                    onValueChange(option.value === value ? null : option.value);
                    onOpenChange(false);
                  }}
                  className={itemClasses}
                >
                  <Check aria-hidden className={cn('size-4 shrink-0 text-primary', option.value !== value && 'invisible')} />
                  {option.label}
                </Command.Item>
              ))}
            </Command.List>
          </Command>
        </Popover>
      )}
    </Field>
  );
}
