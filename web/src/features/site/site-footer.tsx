import { Leaf } from 'lucide-react';
import Link from 'next/link';
import { useTranslations } from 'next-intl';

import { siteNav, type SiteNavKey } from './site-nav';

export type SiteFooterProps = { year: number; pricing: boolean };

const COMPANY = ['about', 'references', 'contact'] as const;
const RESOURCES = ['documents', 'blog', 'calculator'] as const;

/** Site footer: product links, contact details, copyright (01 §7.19). */
export function SiteFooter({ year, pricing }: SiteFooterProps) {
  const t = useTranslations('site');
  const links = siteNav(pricing);
  const pick = (keys: readonly SiteNavKey[]) => links.filter((l) => keys.includes(l.key));
  const product = [...pick(['toolkit', 'pricing']), { key: 'requestDemo' as const, href: '/request-demo' }];
  const column = (title: string, items: { key: SiteNavKey | 'requestDemo'; href: string }[]) => (
    <div className="flex flex-col gap-3">
      <h2 className="type-h3 text-foreground">{title}</h2>
      <ul className="flex flex-col gap-1">
        {items.map((l) => (
          <li key={l.key}>
            <Link href={l.href} className="inline-flex min-h-8 items-center text-foreground-muted hover:text-foreground hover:underline pointer-coarse:min-h-11">
              {t(`nav.${l.key}`)}
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
  const phone = t('contactInfo.phoneValue');
  const email = t('contactInfo.emailValue');
  return (
    <footer aria-label={t('footer.label')} className="border-t border-border bg-surface-sunken">
      <div className="mx-auto grid max-w-7xl gap-10 px-4 py-12 sm:px-6 md:grid-cols-2 lg:grid-cols-5">
        <div className="flex flex-col gap-3 lg:col-span-2">
          <span className="flex items-center gap-1.5">
            <Leaf aria-hidden className="size-6 stroke-brand" />
            <span className="font-heading text-lg font-semibold text-foreground">{t('brand')}</span>
          </span>
          <p className="max-w-sm text-foreground-muted">{t('footer.tagline')}</p>
        </div>
        {column(t('footer.company'), pick(COMPANY))}
        {column(t('footer.product'), product)}
        {column(t('footer.resources'), pick(RESOURCES))}
        <address className="flex flex-col gap-2 not-italic text-foreground-muted md:col-span-2 lg:col-span-5">
          <h2 className="type-h3 text-foreground">{t('footer.contactTitle')}</h2>
          <span>{t('contactInfo.addressValue')}</span>
          <span className="flex flex-wrap gap-x-4 gap-y-1">
            <a href={`tel:${phone.replace(/\s/g, '')}`} className="inline-flex min-h-8 items-center hover:text-foreground hover:underline pointer-coarse:min-h-11">{phone}</a>
            <a href={`mailto:${email}`} className="inline-flex min-h-8 items-center hover:text-foreground hover:underline pointer-coarse:min-h-11">{email}</a>
          </span>
        </address>
      </div>
      <p className="border-t border-border px-4 py-4 text-center text-foreground-muted type-small">{t('footer.copyright', { year })}</p>
    </footer>
  );
}
