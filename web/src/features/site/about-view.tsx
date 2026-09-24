import { useTranslations } from 'next-intl';

import { REFERENCES } from './content';
import { CheckList, CtaBand, PageHero, Section } from './site-blocks';

const PARTNERS = ['energy', 'environment', 'selcuk', 'arti'] as const;
const STEPS = ['measure', 'analyse', 'act', 'report'] as const;

/** About (01 §7.19): hero, mission, product, partners, vision, process, metrics, leadership. */
export function AboutView() {
  const t = useTranslations('site.about');
  const home = useTranslations('site.home');
  const text = (id: 'mission' | 'product' | 'vision') => (
    <Section id={id} title={t(`${id}.title`)}>
      <p className="max-w-3xl text-foreground type-body-lg">{t(`${id}.text`)}</p>
    </Section>
  );
  const metrics = [
    { value: '2020', label: t('metrics.founded') },
    { value: String(REFERENCES.length), label: t('metrics.references') },
    { value: '2', label: t('metrics.products') },
  ];
  return (
    <>
      <PageHero title={t('title')} description={t('intro')} />
      {text('mission')}
      <div className="bg-surface-sunken">{text('product')}</div>
      <Section id="partners" title={t('partners.title')} description={t('partners.text')}>
        <CheckList items={PARTNERS.map((p) => t(`partners.list.${p}`))} />
      </Section>
      <div className="bg-surface-sunken">{text('vision')}</div>
      <Section id="process" title={t('process.title')}>
        <ol className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {STEPS.map((s, i) => (
            <li key={s} className="flex flex-col gap-2 rounded-lg border border-border bg-surface p-5">
              <span aria-hidden className="flex size-8 items-center justify-center rounded-full bg-primary-subtle font-semibold text-primary">{i + 1}</span>
              <h3 className="text-foreground type-h3">{t(`process.steps.${s}.title`)}</h3>
              <p className="text-foreground-muted">{t(`process.steps.${s}.text`)}</p>
            </li>
          ))}
        </ol>
      </Section>
      <Section id="metrics" title={t('metrics.title')} tone="sunken">
        <dl className="grid gap-4 sm:grid-cols-3">
          {metrics.map((m) => (
            <div key={m.label} className="flex flex-col-reverse gap-1 rounded-lg border border-border bg-surface p-5">
              <dt className="text-foreground-muted">{m.label}</dt>
              <dd className="text-foreground type-metric">{m.value}</dd>
            </div>
          ))}
        </dl>
      </Section>
      <Section id="leadership" title={home('leadership.title')}>
        <div className="flex flex-col gap-1">
          <p className="text-foreground type-h2">{home('leadership.name')}</p>
          <p className="text-foreground-muted">{home('leadership.role')}</p>
        </div>
      </Section>
      <CtaBand title={home('demo.title')} description={home('demo.description')} href="/request-demo" action={home('demo.action')} />
    </>
  );
}
