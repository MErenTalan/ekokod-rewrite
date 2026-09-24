import { ArrowRight, Check, Leaf } from 'lucide-react';
import Link from 'next/link';
import { useTranslations } from 'next-intl';

import { Accordion } from '@/components/ui/accordion';
import { Button } from '@/components/ui/button';

import { CM_FEATURES, RM_FEATURES } from './content';
import { ContactDetails, CtaBand, DocumentGrid, FeatureGrid, MoreLink, NewsList, ReferenceGrid, Section } from './site-blocks';

const POINTS = ['bills', 'consumption', 'carbon', 'alarms', 'loadProfile'] as const;
const FAQ = ['data', 'bills', 'carbon', 'install'] as const;

/** The homepage's section inventory (01 §7.19), without fabricated testimonials or metrics (Q-H10). */
export function HomeView({ pricing }: { pricing: boolean }) {
  const t = useTranslations('site.home');
  const site = useTranslations('site');
  return (
    <>
      <div className="border-b border-border bg-surface-sunken">
        <div className="mx-auto grid max-w-7xl items-center gap-10 px-4 py-14 sm:px-6 md:py-20 lg:grid-cols-[1.2fr_1fr]">
          <div className="flex flex-col gap-6">
            <h1 className="text-foreground type-display md:text-[44px] md:leading-[52px]">{t('hero.title')}</h1>
            <p className="max-w-xl text-foreground-muted type-body-lg">{t('hero.intro')}</p>
            <div className="flex flex-wrap gap-3">
              <Button asChild size="lg">
                <Link href="/request-demo">{t('hero.primary')}</Link>
              </Button>
              <Button asChild size="lg" variant="secondary">
                <Link href="/bill-calculator">{t('hero.secondary')}</Link>
              </Button>
            </div>
          </div>
          <ul className="flex flex-col gap-3 rounded-lg border border-border bg-surface p-6 shadow-sm">
            {POINTS.map((p) => (
              <li key={p} className="flex items-start gap-3 text-foreground type-body-lg">
                <Check aria-hidden className="mt-1 size-4 shrink-0 stroke-brand" />
                <span>{t(`hero.points.${p}`)}</span>
              </li>
            ))}
          </ul>
        </div>
      </div>

      <section aria-labelledby="banner-title" className="mx-auto flex max-w-7xl flex-col items-start gap-3 px-4 py-12 sm:px-6 md:flex-row md:items-center md:gap-8">
        <Leaf aria-hidden className="size-10 shrink-0 stroke-brand" />
        <div className="flex flex-col gap-1">
          <h2 id="banner-title" className="text-foreground type-h1">{t('banner.title')}</h2>
          <p className="max-w-3xl text-foreground-muted type-body-lg">{t('banner.description')}</p>
        </div>
      </section>

      <Section id="features" title={t('features.title')} description={t('features.description')} tone="sunken">
        <FeatureGrid items={RM_FEATURES.map((k) => site(`features.${k}`))} />
      </Section>

      <Section id="carbon" title={t('carbon.title')} description={t('carbon.description')}>
        <FeatureGrid items={CM_FEATURES.map((k) => site(`carbonFeatures.${k}`))} />
      </Section>

      <div className="bg-primary-subtle">
        <p className="mx-auto flex max-w-7xl flex-wrap items-center justify-between gap-4 px-4 py-6 text-foreground type-body-lg sm:px-6">
          <span>{t('strip.text')}</span>
          <Link href="/contact" className="inline-flex min-h-11 items-center gap-1 font-semibold text-primary hover:underline">
            {t('strip.action')}
            <ArrowRight aria-hidden className="size-4" />
          </Link>
        </p>
      </div>

      <Section id="references" title={t('references.title')} action={<MoreLink href="/references">{t('references.all')}</MoreLink>}>
        <ReferenceGrid stories={false} />
      </Section>

      <Section id="news" title={t('news.title')} tone="sunken">
        <NewsList />
      </Section>

      <Section id="documents" title={t('documents.title')} action={<MoreLink href="/documents">{t('documents.all')}</MoreLink>}>
        <DocumentGrid groups={['general']} />
      </Section>

      <Section id="toolkit" title={t('toolkit.title')} tone="sunken" action={<MoreLink href="/toolkit">{t('toolkit.action')}</MoreLink>}
        description={site('toolkit.description')} />

      <Section id="leadership" title={t('leadership.title')}>
        <div className="flex flex-col gap-1">
          <p className="text-foreground type-h2">{t('leadership.name')}</p>
          <p className="text-foreground-muted">{t('leadership.role')}</p>
        </div>
      </Section>

      <Section id="faq" title={t('faq.title')} tone="sunken">
        <div className="max-w-3xl">
          <Accordion type="single" items={FAQ.map((k) => ({ value: k, title: t(`faq.items.${k}.q`), content: t(`faq.items.${k}.a`) }))} />
        </div>
      </Section>

      {pricing ? (
        <Section id="pricing" title={t('pricing.title')} description={t('pricing.description')} action={<MoreLink href="/pricing">{t('pricing.action')}</MoreLink>} />
      ) : null}

      <CtaBand title={t('demo.title')} description={t('demo.description')} href="/request-demo" action={t('demo.action')} />

      <Section id="contact" title={t('contact.title')} action={<MoreLink href="/contact">{t('contact.action')}</MoreLink>}>
        <ContactDetails />
      </Section>
    </>
  );
}
