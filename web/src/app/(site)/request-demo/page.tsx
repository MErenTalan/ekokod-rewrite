import { getTranslations } from 'next-intl/server';

import { LeadFormPanel } from '@/features/site/lead-form-panel';
import { PageHero } from '@/features/site/site-blocks';
import { pageMetadata } from '@/lib/site/metadata';

export const generateMetadata = () => pageMetadata('requestDemo', '/request-demo');

/** Request demo (01 §7.19): hero plus the lead-capture form, mailed to the operator (F12a R353). */
export default async function RequestDemoPage() {
  const t = await getTranslations('site');
  return (
    <>
      <PageHero title={t('demo.title')} description={t('demo.description')} />
      <section aria-labelledby="demo-form-title" className="mx-auto flex max-w-3xl flex-col gap-4 px-4 py-12 sm:px-6">
        <h2 id="demo-form-title" className="text-foreground type-h2">{t('forms.demoTitle')}</h2>
        <LeadFormPanel kind="demo" />
      </section>
    </>
  );
}
