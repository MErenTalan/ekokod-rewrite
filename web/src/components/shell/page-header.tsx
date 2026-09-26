'use client';

import { usePathname } from 'next/navigation';
import { useTranslations } from 'next-intl';
import type { ReactNode } from 'react';

import { trailFor } from '@/modules/registry';

import { Breadcrumb, type BreadcrumbItem } from '../ui/breadcrumb';

export type PageHeaderProps = { title: string; description?: string; actions?: ReactNode; breadcrumb?: BreadcrumbItem[] };

/** Page h1 with the breadcrumb derived from the route, overridable per page (plan I-15). */
export function PageHeader({ title, description, actions, breadcrumb }: PageHeaderProps) {
  const t = useTranslations();
  const pathname = usePathname();
  const trail = trailFor(pathname);
  const items: BreadcrumbItem[] =
    breadcrumb ?? (trail ? [...(trail.group ? [{ label: t(trail.group.label) }] : []), { label: t(trail.leaf.label) }] : []);
  return (
    <header className="flex flex-col gap-2">
      {items.length > 0 ? <Breadcrumb items={items} /> : null}
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="flex min-w-0 flex-col gap-1">
          <h1 className="text-foreground type-h1">{title}</h1>
          {description ? <p className="text-foreground-muted type-body">{description}</p> : null}
        </div>
        {actions ? <div className="flex flex-wrap items-center gap-2">{actions}</div> : null}
      </div>
    </header>
  );
}
