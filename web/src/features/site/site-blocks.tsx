import { ArrowRight, Check, Clock, ExternalLink, FileText, Mail, MapPin, Phone } from 'lucide-react';
import Image from 'next/image';
import Link from 'next/link';
import { useTranslations } from 'next-intl';
import type { ReactNode } from 'react';

import { Button } from '@/components/ui/button';

import { CM_FEATURES, DOCUMENT_GROUPS, NEWS, REFERENCES, RM_FEATURES } from './content';

// Building blocks shared by the public pages (01 §7.19); each is a server-renderable view.

export function PageHero({ title, description, children }: { title: string; description?: string; children?: ReactNode }) {
  return (
    <div className="border-b border-border bg-surface-sunken">
      <div className="mx-auto flex max-w-7xl flex-col gap-4 px-4 py-12 sm:px-6 md:py-16">
        <h1 className="max-w-3xl text-foreground type-display md:text-[40px] md:leading-[48px]">{title}</h1>
        {description ? <p className="max-w-2xl text-foreground-muted type-body-lg">{description}</p> : null}
        {children}
      </div>
    </div>
  );
}

export function Section({ id, title, description, action, children, tone = 'plain' }: {
  id: string; title: string; description?: string; action?: ReactNode; children?: ReactNode; tone?: 'plain' | 'sunken';
}) {
  return (
    <section aria-labelledby={`${id}-title`} className={tone === 'sunken' ? 'bg-surface-sunken' : undefined}>
      <div className="mx-auto flex max-w-7xl flex-col gap-8 px-4 py-14 sm:px-6">
        <div className="flex flex-wrap items-end justify-between gap-4">
          <div className="flex max-w-3xl flex-col gap-2">
            <h2 id={`${id}-title`} className="text-foreground type-h1">{title}</h2>
            {description ? <p className="text-foreground-muted type-body-lg">{description}</p> : null}
          </div>
          {action}
        </div>
        {children}
      </div>
    </section>
  );
}

export function CheckList({ items }: { items: string[] }) {
  return (
    <ul className="flex flex-col gap-3">
      {items.map((item) => (
        <li key={item} className="flex items-start gap-3 text-foreground type-body-lg">
          <Check aria-hidden className="mt-1 size-4 shrink-0 stroke-brand" />
          <span>{item}</span>
        </li>
      ))}
    </ul>
  );
}

export function FeatureGrid({ items }: { items: string[] }) {
  return (
    <ul className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {items.map((item) => (
        <li key={item} className="flex items-start gap-3 rounded-lg border border-border bg-surface p-5 text-foreground type-body-lg">
          <Check aria-hidden className="mt-1 size-4 shrink-0 stroke-brand" />
          <span>{item}</span>
        </li>
      ))}
    </ul>
  );
}

export function MoreLink({ href, children }: { href: string; children: ReactNode }) {
  return (
    <Button asChild variant="secondary">
      <Link href={href}>
        {children}
        <ArrowRight aria-hidden className="size-4" />
      </Link>
    </Button>
  );
}

export function CtaBand({ title, description, href, action }: { title: string; description?: string; href: string; action: string }) {
  return (
    <section aria-label={title} className="bg-primary text-on-primary">
      <div className="mx-auto flex max-w-7xl flex-col items-start gap-4 px-4 py-12 sm:px-6 md:flex-row md:items-center md:justify-between">
        <div className="flex max-w-2xl flex-col gap-2">
          <p className="type-h1">{title}</p>
          {description ? <p className="type-body-lg opacity-90">{description}</p> : null}
        </div>
        <Button asChild size="lg" variant="secondary">
          <Link href={href}>
            {action}
            <ArrowRight aria-hidden className="size-4" />
          </Link>
        </Button>
      </div>
    </section>
  );
}

/** EKO-RM and EKO-CM feature lists side by side. */
export function ProductColumns() {
  const t = useTranslations('site');
  const card = (title: string, description: string, items: string[]) => (
    <article className="flex flex-col gap-4 rounded-lg border border-border bg-surface p-6">
      <h3 className="text-foreground type-h2">{title}</h3>
      <p className="text-foreground-muted">{description}</p>
      <CheckList items={items} />
    </article>
  );
  return (
    <div className="grid gap-6 lg:grid-cols-2">
      {card(t('toolkit.rm.title'), t('toolkit.rm.description'), RM_FEATURES.map((k) => t(`features.${k}`)))}
      {card(t('toolkit.cm.title'), t('toolkit.cm.description'), CM_FEATURES.map((k) => t(`carbonFeatures.${k}`)))}
    </div>
  );
}

export function ReferenceGrid({ stories = true, level = 3 }: { stories?: boolean; level?: 2 | 3 }) {
  const H = `h${level}` as const;
  const t = useTranslations('site.references');
  return (
    <ul className="grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
      {REFERENCES.map((r) => (
        <li key={r.key} className="flex flex-col gap-4 rounded-lg border border-border bg-surface p-6">
          <div className="flex h-20 items-center justify-center rounded-md bg-logo-plate p-3">
            <Image src={r.logo} alt={t('logoAlt', { name: t(`items.${r.key}.name`) })} width={r.width} height={r.height} className="max-h-full w-auto object-contain" sizes="200px" />
          </div>
          <H className="text-foreground type-h3">{t(`items.${r.key}.name`)}</H>
          {stories ? <p className="text-foreground-muted">{t(`items.${r.key}.story`)}</p> : null}
        </li>
      ))}
    </ul>
  );
}

/** `level` is the first heading level used: group headings when there are several groups, else the items. */
export function DocumentGrid({ groups = ['general', 'technical', 'training'], level = 3 }: { groups?: (keyof typeof DOCUMENT_GROUPS)[]; level?: 2 | 3 }) {
  const Group = `h${level}` as const;
  const Item = (groups.length > 1 ? `h${level + 1}` : `h${level}`) as 'h3' | 'h4';
  const t = useTranslations('site.documents');
  return (
    <div className="flex flex-col gap-10">
      {groups.map((g) => (
        <div key={g} className="flex flex-col gap-4">
          {groups.length > 1 ? <Group className="text-foreground type-h2">{t(`groups.${g}`)}</Group> : null}
          <ul className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
            {DOCUMENT_GROUPS[g].map((d) => {
              const title = t(`items.${d}.title`);
              const action = d === 'articles' ? t('items.articles.action') : t('request');
              const href = d === 'articles' ? '/blog' : `/contact?subject=${encodeURIComponent(t('requestSubject', { title }))}`;
              return (
                <li key={d} className="flex flex-col gap-3 rounded-lg border border-border bg-surface p-5">
                  <FileText aria-hidden className="size-6 stroke-brand" />
                  <Item className="text-foreground type-h3">{title}</Item>
                  <p className="flex-1 text-foreground-muted">{t(`items.${d}.description`)}</p>
                  <Link href={href} aria-label={`${action}: ${title}`} className="inline-flex min-h-8 items-center gap-1 font-semibold text-primary underline-offset-2 hover:underline pointer-coarse:min-h-11">
                    {action}
                    <ArrowRight aria-hidden className="size-4" />
                  </Link>
                </li>
              );
            })}
          </ul>
        </div>
      ))}
    </div>
  );
}

export function NewsList() {
  const t = useTranslations('site.home.news');
  return (
    <ul className="grid gap-6 lg:grid-cols-3">
      {NEWS.map((n) => {
        const title = t(`items.${n.key}.title`);
        const [cover] = n.photos;
        return (
          <li key={n.key} className="flex flex-col overflow-hidden rounded-lg border border-border bg-surface">
            <Image src={cover.src} alt={t('photo', { title, n: 1 })} width={cover.width} height={cover.height} className="aspect-[4/3] w-full object-cover" sizes="(min-width: 1024px) 400px, 100vw" />
            <div className="flex flex-1 flex-col gap-2 p-5">
              <p className="text-foreground-muted type-caption">{t('date')}</p>
              <h3 className="text-foreground type-h3">{title}</h3>
              <p className="flex-1 text-foreground-muted">{t(`items.${n.key}.body`)}</p>
              {n.video ? (
                <a href={n.video} target="_blank" rel="noopener noreferrer" className="inline-flex min-h-8 items-center gap-1 font-semibold text-primary hover:underline pointer-coarse:min-h-11">
                  {t('watch')}
                  <span className="sr-only"> {t('opensNew')}</span>
                  <ExternalLink aria-hidden className="size-4" />
                </a>
              ) : null}
            </div>
          </li>
        );
      })}
    </ul>
  );
}

const TAP = 'inline-flex min-h-8 items-center hover:underline pointer-coarse:min-h-11';

export const MAP_URL = 'https://www.openstreetmap.org/search?query=Konya%20Teknokent%20Sel%C3%A7uklu';

/** Address, phone, e-mail, hours and a map link (Q-H8: no third-party iframe). */
export function ContactDetails() {
  const t = useTranslations('site.contactInfo');
  const phone = t('phoneValue');
  const email = t('emailValue');
  const row = (Icon: typeof MapPin, label: string, value: ReactNode) => (
    <div className="relative flex flex-col ps-8">
      <dt className="text-foreground-muted type-small">
        <Icon aria-hidden className="absolute start-0 top-0.5 size-5 stroke-brand" />
        {label}
      </dt>
      <dd className="text-foreground type-body-lg">{value}</dd>
    </div>
  );
  return (
    <dl className="flex flex-col gap-5">
      {row(MapPin, t('address'), (
        <>
          <span className="block">{t('addressValue')}</span>
          <a href={MAP_URL} target="_blank" rel="noopener noreferrer" className="inline-flex min-h-8 items-center gap-1 font-semibold text-primary hover:underline pointer-coarse:min-h-11">
            {t('map')}
            <span className="sr-only"> ({t('mapHint')})</span>
            <ExternalLink aria-hidden className="size-4" />
          </a>
        </>
      ))}
      {row(Phone, t('phone'), <a href={`tel:${phone.replace(/\s/g, '')}`} className={TAP}>{phone}</a>)}
      {row(Mail, t('email'), <a href={`mailto:${email}`} className={TAP}>{email}</a>)}
      {row(Clock, t('hours'), t('hoursValue'))}
    </dl>
  );
}
