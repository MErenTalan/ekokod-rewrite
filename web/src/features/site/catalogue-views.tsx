import { useTranslations } from 'next-intl';

import { CtaBand, DocumentGrid, PageHero, ProductColumns, ReferenceGrid, Section } from './site-blocks';

function DemoBand() {
  const t = useTranslations('site.home.demo');
  return <CtaBand title={t('title')} description={t('description')} href="/request-demo" action={t('action')} />;
}

/** References (01 §7.19): customer references with their success stories. */
export function ReferencesView() {
  const t = useTranslations('site.references');
  return (
    <>
      <PageHero title={t('title')} description={t('description')} />
      <div className="mx-auto max-w-7xl px-4 py-14 sm:px-6">
        <ReferenceGrid level={2} />
      </div>
      <DemoBand />
    </>
  );
}

/** Documents (01 §7.19): the downloadable-documents listing; requests go through contact. */
export function DocumentsView() {
  const t = useTranslations('site.documents');
  return (
    <>
      <PageHero title={t('title')} description={t('description')} />
      <div className="mx-auto max-w-7xl px-4 py-14 sm:px-6">
        <DocumentGrid level={2} />
      </div>
    </>
  );
}

/** Toolkit (01 §7.19): the source/toolkit feature listing for both products. */
export function ToolkitView() {
  const t = useTranslations('site');
  return (
    <>
      <PageHero title={t('toolkit.title')} description={t('toolkit.description')} />
      <Section id="products" title={t('home.features.title')} description={t('home.features.description')}>
        <ProductColumns />
      </Section>
      <DemoBand />
    </>
  );
}
