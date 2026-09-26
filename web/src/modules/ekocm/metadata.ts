import type { Metadata } from 'next';
import { getTranslations } from 'next-intl/server';

import type { CarbonTab } from '@/features/carbon/labels';

/** The browser title of an EKOCM page: its section name. */
export async function sectionMetadata(section: CarbonTab): Promise<Metadata> {
  const t = await getTranslations('carbon');
  return { title: t(`tabs.${section}`) };
}
