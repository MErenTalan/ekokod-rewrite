import { getTranslations } from 'next-intl/server';

import { PageHeader } from '@/components/shell/page-header';
import { Card } from '@/components/ui/card';
import { LeadFormPanel } from '@/features/site/lead-form-panel';
import { ContactDetails } from '@/features/site/site-blocks';

export async function generateMetadata() {
  const t = await getTranslations('site');
  return { title: t('contact.title') };
}

/** The sidebar's Contact (R167: app routes live under /ekorm): the public page's details and form inside the shell. */
export default async function Page() {
  const t = await getTranslations('site');
  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('contact.title')} description={t('contact.description')} />
      <div className="grid gap-6 lg:grid-cols-[1fr_1.5fr] lg:items-start">
        <Card className="p-6">
          <ContactDetails />
        </Card>
        <Card className="flex flex-col gap-4 p-6">
          <h2 className="text-foreground type-h2">{t('forms.contactTitle')}</h2>
          <LeadFormPanel kind="contact" />
        </Card>
      </div>
    </div>
  );
}
