import { getTranslations } from 'next-intl/server';

import { LeadFormPanel } from '@/features/site/lead-form-panel';
import { ContactDetails, PageHero } from '@/features/site/site-blocks';
import { pageMetadata } from '@/lib/site/metadata';

export const generateMetadata = () => pageMetadata('contact', '/contact');

/** Contact (01 §7.19): address, phone, e-mail, hours, a map link (Q-H8) and the contact form. */
export default async function ContactPage({ searchParams }: { searchParams: Promise<{ subject?: string | string[] }> }) {
  const [t, { subject }] = await Promise.all([getTranslations('site'), searchParams]);
  const initial = typeof subject === 'string' ? subject.slice(0, 200) : undefined;
  return (
    <>
      <PageHero title={t('contact.title')} description={t('contact.description')} />
      <div className="mx-auto grid max-w-7xl gap-10 px-4 py-12 sm:px-6 lg:grid-cols-[1fr_1.5fr]">
        <ContactDetails />
        <section aria-labelledby="contact-form-title" className="flex flex-col gap-4">
          <h2 id="contact-form-title" className="text-foreground type-h2">{t('forms.contactTitle')}</h2>
          <LeadFormPanel kind="contact" defaultSubject={initial} />
        </section>
      </div>
    </>
  );
}
