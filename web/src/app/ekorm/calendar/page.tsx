import { getTranslations } from 'next-intl/server';

import { CalendarPage } from '@/features/calendar/calendar-page';

export const dynamic = 'force-dynamic';

export async function generateMetadata() {
  const t = await getTranslations('calendar');
  return { title: t('title') };
}

export default function Page() {
  return <CalendarPage />;
}
