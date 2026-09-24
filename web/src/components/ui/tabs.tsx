'use client';

import { Tabs as RadixTabs } from 'radix-ui';
import type { ReactNode } from 'react';

export type TabItem = { value: string; label: string; content: ReactNode; disabled?: boolean };
export type TabsProps = { items: TabItem[]; value?: string; defaultValue?: string; onValueChange?: (value: string) => void };

export function Tabs({ items, value, defaultValue, onValueChange }: TabsProps) {
  return (
    <RadixTabs.Root value={value} defaultValue={defaultValue ?? items[0]?.value} onValueChange={onValueChange} className="flex min-w-0 flex-col">
      <RadixTabs.List className="flex max-w-full gap-1 overflow-x-auto overflow-y-hidden border-b border-border [scrollbar-width:thin]">
        {items.map((item) => (
          <RadixTabs.Trigger
            key={item.value}
            value={item.value}
            disabled={item.disabled}
            className="relative h-9 shrink-0 px-3 text-foreground-muted type-body whitespace-nowrap transition-colors duration-(--duration-tab) ease-in-out after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 after:rounded-full hover:text-foreground disabled:opacity-60 data-[state=active]:text-foreground data-[state=active]:after:bg-primary pointer-coarse:min-h-11"
          >
            {item.label}
          </RadixTabs.Trigger>
        ))}
      </RadixTabs.List>
      {items.map((item) => (
        <RadixTabs.Content key={item.value} value={item.value} className="pt-4">
          {item.content}
        </RadixTabs.Content>
      ))}
    </RadixTabs.Root>
  );
}
