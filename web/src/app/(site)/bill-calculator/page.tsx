import { getTranslations } from 'next-intl/server';

import { BillCalculatorPanel } from '@/features/site/bill-calculator-panel';
import { PageHero } from '@/features/site/site-blocks';
import { istanbulToday } from '@/lib/dates';
import { pageMetadata } from '@/lib/site/metadata';

export const generateMetadata = () => pageMetadata('calculator', '/bill-calculator');

export default async function BillCalculatorPage() {
  const t = await getTranslations('site.calculator');
  return (
    <>
      <PageHero title={t('title')} description={t('description')} />
      <BillCalculatorPanel today={istanbulToday()} />
    </>
  );
}
