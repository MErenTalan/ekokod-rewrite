'use client';

import type { LucideIcon } from 'lucide-react';
import { DropdownMenu as RadixMenu } from 'radix-ui';
import { useId, type ReactElement } from 'react';

import { cn } from '@/lib/cn';

export type DropdownMenuItem =
  | { type: 'item'; label: string; icon?: LucideIcon; description?: string; onSelect: () => void; disabled?: boolean; tone?: 'danger' }
  | { type: 'separator' }
  | { type: 'label'; label: string };

export type DropdownMenuProps = {
  trigger: ReactElement;
  items: DropdownMenuItem[];
  align?: 'start' | 'center' | 'end';
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
};

function Item({ item }: { item: Extract<DropdownMenuItem, { type: 'item' }> }) {
  const descriptionId = useId();
  const Icon = item.icon;
  return (
    <RadixMenu.Item
      disabled={item.disabled}
      onSelect={item.onSelect}
      aria-describedby={item.description ? descriptionId : undefined}
      className={cn(
        'flex cursor-pointer items-start gap-2 rounded-sm px-2 py-1.5 type-body select-none data-[disabled]:cursor-not-allowed data-[disabled]:opacity-60 data-[highlighted]:bg-surface-sunken pointer-coarse:min-h-11',
        item.tone === 'danger' ? 'text-danger' : 'text-foreground',
      )}
    >
      {Icon ? <Icon aria-hidden className="mt-0.5 size-4 shrink-0" /> : null}
      <span className="flex min-w-0 flex-col">
        <span>{item.label}</span>
        {item.description ? (
          <span id={descriptionId} className="text-foreground-muted type-caption">
            {item.description}
          </span>
        ) : null}
      </span>
    </RadixMenu.Item>
  );
}

export function DropdownMenu({ trigger, items, align = 'end', open, onOpenChange }: DropdownMenuProps) {
  return (
    <RadixMenu.Root open={open} onOpenChange={onOpenChange}>
      <RadixMenu.Trigger asChild>{trigger}</RadixMenu.Trigger>
      <RadixMenu.Portal>
        <RadixMenu.Content
          align={align}
          sideOffset={4}
          collisionPadding={8}
          className="z-50 max-h-(--radix-dropdown-menu-content-available-height) min-w-48 max-w-[calc(100vw-16px)] overflow-y-auto rounded-md border border-border bg-surface-raised p-1 shadow-md data-[state=closed]:animate-popover-out data-[state=open]:animate-popover-in"
        >
          {items.map((item, i) =>
            item.type === 'separator' ? (
              <RadixMenu.Separator key={`sep-${i}`} className="my-1 h-px bg-border" />
            ) : item.type === 'label' ? (
              <RadixMenu.Label key={`label-${item.label}`} className="px-2 py-1 text-foreground-muted type-caption">
                {item.label}
              </RadixMenu.Label>
            ) : (
              <Item key={`${item.label}-${i}`} item={item} />
            ),
          )}
        </RadixMenu.Content>
      </RadixMenu.Portal>
    </RadixMenu.Root>
  );
}
