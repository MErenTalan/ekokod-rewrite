'use client';

import { Command } from 'cmdk';
import { Check, ChevronsUpDown } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useId, useState } from 'react';

import { cn } from '@/lib/cn';

import { filterOptions, itemClasses, listClasses, searchClasses } from './combobox';
import { controlClasses, Field, type FieldProps, type Option } from './field';
import { Popover } from './popover';

export type MultiSelectProps = FieldProps & {
  options: Option[];
  value: string[];
  onValueChange: (value: string[]) => void;
  searchPlaceholder: string;
  emptyText: string;
  maxChips?: number;
};

export function MultiSelect({ label, description, error, required, id, disabled, options, value, onValueChange, searchPlaceholder, emptyText, maxChips = 3 }: MultiSelectProps) {
  const t = useTranslations('forms');
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const listId = useId();
  const chosen = options.filter((o) => value.includes(o.value));
  const toggle = (v: string) => onValueChange(value.includes(v) ? value.filter((x) => x !== v) : [...value, v]);
  return (
    <Field label={label} description={description} error={error} required={required} id={id} disabled={disabled}>
      {({ controlId, describedBy, invalid }) => (
        <Popover
          open={open}
          onOpenChange={(next) => {
            setOpen(next);
            if (!next) setQuery('');
          }}
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
              className={cn(controlClasses, 'flex h-auto min-h-9 items-center justify-between gap-2 py-1 text-start')}
            >
              <span className="flex min-w-0 flex-wrap items-center gap-1">
                {chosen.length === 0 ? <span className="text-foreground-subtle">{searchPlaceholder}</span> : null}
                {chosen.slice(0, maxChips).map((o) => (
                  <span key={o.value} className="max-w-40 truncate rounded-sm bg-surface-sunken px-1.5 py-0.5 text-foreground type-caption">
                    {o.label}
                  </span>
                ))}
                {chosen.length > maxChips ? (
                  <span className="text-foreground-muted type-caption">{t('moreSelected', { count: chosen.length - maxChips })}</span>
                ) : null}
              </span>
              <ChevronsUpDown aria-hidden className="size-4 shrink-0 text-foreground-muted" />
            </button>
          }
        >
          <Command shouldFilter={false} label={searchPlaceholder} className="flex flex-col">
            <Command.Input value={query} onValueChange={setQuery} placeholder={searchPlaceholder} className={searchClasses} />
            <Command.List id={listId} label={label} aria-multiselectable className={listClasses}>
              <Command.Empty className="px-2 py-3 text-center text-foreground-muted type-small">{emptyText}</Command.Empty>
              {filterOptions(options, query).map((option) => {
                const checked = value.includes(option.value);
                return (
                  <Command.Item
                    key={option.value}
                    value={option.value}
                    disabled={option.disabled}
                    aria-checked={checked}
                    onSelect={() => toggle(option.value)}
                    className={itemClasses}
                  >
                    <span
                      aria-hidden
                      className={cn(
                        'flex size-4 shrink-0 items-center justify-center rounded-sm border border-border-control',
                        checked && 'border-primary bg-primary text-on-primary',
                      )}
                    >
                      {checked ? <Check className="size-3" /> : null}
                    </span>
                    {option.label}
                  </Command.Item>
                );
              })}
            </Command.List>
          </Command>
        </Popover>
      )}
    </Field>
  );
}
