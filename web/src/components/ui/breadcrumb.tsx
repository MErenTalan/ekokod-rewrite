'use client';

import { ChevronRight } from 'lucide-react';
import Link from 'next/link';
import { useTranslations } from 'next-intl';

export type BreadcrumbItem = { label: string; href?: string };

export function Breadcrumb({ items }: { items: BreadcrumbItem[] }) {
  const t = useTranslations('feedback');
  return (
    <nav aria-label={t('breadcrumb')}>
      <ol className="flex flex-wrap items-center gap-1 type-small">
        {items.map((item, i) => {
          const last = i === items.length - 1;
          return (
            <li key={`${item.label}-${i}`} className="flex min-w-0 items-center gap-1">
              {last || !item.href ? (
                <span aria-current={last ? 'page' : undefined} className={last ? 'text-foreground' : 'text-foreground-muted'}>
                  {item.label}
                </span>
              ) : (
                <Link href={item.href} className="rounded-sm text-foreground-muted hover:text-foreground hover:underline pointer-coarse:inline-flex pointer-coarse:min-h-11 pointer-coarse:items-center">
                  {item.label}
                </Link>
              )}
              {last ? null : <ChevronRight aria-hidden className="size-3.5 shrink-0 rtl:rotate-180 text-foreground-subtle" />}
            </li>
          );
        })}
      </ol>
    </nav>
  );
}
