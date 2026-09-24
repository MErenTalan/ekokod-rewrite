'use client';

import { ChevronDown } from 'lucide-react';
import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';
import { useId, useState } from 'react';

import { cn } from '@/lib/cn';

import { DropdownMenu } from '../ui/dropdown-menu';
import { Tooltip } from '../ui/tooltip';
import { findTrail, isGroup, navigation, type NavEntry, type NavGroup, type NavLeaf } from './nav-config';

export type SidebarProps = { collapsed: boolean; onNavigate?: () => void; entries?: NavEntry[] };

const row =
  'relative flex h-9 w-full items-center gap-3 rounded-md px-3 text-start type-body transition-colors duration-(--duration-hover) pointer-coarse:min-h-11';

function Leaf({ leaf, collapsed, onNavigate }: { leaf: NavLeaf; collapsed: boolean; onNavigate?: () => void }) {
  const t = useTranslations('shell');
  const pathname = usePathname();
  const label = t(`nav.${leaf.labelKey}`);
  const Icon = leaf.icon;
  const content = (
    <>
      <Icon aria-hidden className="size-4 shrink-0" />
      <span className={collapsed ? 'sr-only' : 'min-w-0 flex-1 truncate'}>{label}</span>
    </>
  );
  if (leaf.disabled) {
    // Visible and focusable but not a link: the tooltip explains why (07 §7).
    return (
      <Tooltip content={collapsed ? `${label}: ${t('notYetAvailable')}` : t('notYetAvailable')} side="right">
        <span role="link" aria-disabled="true" tabIndex={0} className={cn(row, 'cursor-not-allowed text-foreground-subtle', collapsed && 'justify-center px-0')}>
          {content}
          {collapsed ? null : (
            <span aria-hidden className="shrink-0 rounded-full border border-border px-1.5 text-foreground-subtle type-caption">{t('soon')}</span>
          )}
        </span>
      </Tooltip>
    );
  }
  const active = findTrail(pathname)?.leaf.id === leaf.id;
  const link = (
    <Link
      href={leaf.href}
      aria-current={active ? 'page' : undefined}
      onClick={onNavigate}
      className={cn(
        row,
        collapsed && 'justify-center px-0',
        active
          ? 'bg-primary-subtle font-semibold text-foreground before:absolute before:inset-y-1.5 before:start-0 before:w-1 before:rounded-full before:bg-brand'
          : 'text-foreground-muted hover:bg-surface-sunken hover:text-foreground',
      )}
    >
      {content}
    </Link>
  );
  return collapsed ? (
    <Tooltip content={label} side="right">
      {link}
    </Tooltip>
  ) : (
    link
  );
}

function Group({ group, collapsed, onNavigate }: { group: NavGroup; collapsed: boolean; onNavigate?: () => void }) {
  const t = useTranslations('shell');
  const pathname = usePathname();
  const router = useRouter();
  const listId = useId();
  const hasActive = findTrail(pathname)?.group?.id === group.id;
  const [open, setOpen] = useState(hasActive);
  const label = t(`nav.${group.labelKey}`);
  const Icon = group.icon;
  if (collapsed) {
    return (
      <DropdownMenu
        align="start"
        trigger={
          <button type="button" className={cn(row, 'justify-center px-0', hasActive ? 'bg-primary-subtle text-foreground' : 'text-foreground-muted hover:bg-surface-sunken')}>
            <Icon aria-hidden className="size-4" />
            <span className="sr-only">{label}</span>
          </button>
        }
        items={[
          { type: 'label', label },
          ...group.children.map((c) => ({
            type: 'item' as const,
            label: t(`nav.${c.labelKey}`),
            icon: c.icon,
            disabled: c.disabled,
            description: c.disabled ? t('notYetAvailable') : undefined,
            onSelect: () => {
              router.push(c.href);
              onNavigate?.();
            },
          })),
        ]}
      />
    );
  }
  return (
    <>
      <button
        type="button"
        aria-expanded={open}
        aria-controls={listId}
        onClick={() => setOpen((o) => !o)}
        className={cn(row, hasActive ? 'text-foreground' : 'text-foreground-muted', 'hover:bg-surface-sunken hover:text-foreground')}
      >
        <Icon aria-hidden className="size-4 shrink-0" />
        <span className="min-w-0 flex-1 truncate">{label}</span>
        <ChevronDown aria-hidden className={cn('size-4 shrink-0 transition-transform duration-(--duration-hover)', open && 'rotate-180')} />
      </button>
      <ul id={listId} hidden={!open} className="ms-5 mt-0.5 flex flex-col gap-0.5 border-s border-border ps-2">
        {group.children.map((c) => (
          <li key={c.id}>
            <Leaf leaf={c} collapsed={false} onNavigate={onNavigate} />
          </li>
        ))}
      </ul>
    </>
  );
}

export function Sidebar({ collapsed, onNavigate, entries = navigation }: SidebarProps) {
  const t = useTranslations('shell');
  return (
    <nav aria-label={t('nav.label')} className={cn('flex flex-col p-2', collapsed ? 'w-16' : 'w-64')}>
      <ul className="flex flex-col gap-0.5">
        {entries.map((entry) => (
          <li key={entry.id}>
            {isGroup(entry) ? <Group group={entry} collapsed={collapsed} onNavigate={onNavigate} /> : <Leaf leaf={entry} collapsed={collapsed} onNavigate={onNavigate} />}
          </li>
        ))}
      </ul>
    </nav>
  );
}
