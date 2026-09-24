import type { Metadata } from 'next';
import { getTranslations } from 'next-intl/server';
import { cookies, headers } from 'next/headers';
import Script from 'next/script';

import { AnnouncementBar } from '@/features/site/announcement-bar';
import { SiteFooter } from '@/features/site/site-footer';
import { SiteHeader } from '@/features/site/site-header';
import { ACCESS_COOKIE, REFRESH_COOKIE } from '@/middleware';
import { analyticsScript, siteFeatures } from '@/lib/site/features';

export const metadata: Metadata = { title: { template: '%s | EkoKod', default: 'EkoKod' } };

/** The public site's frame (01 §7.19): announcement, header with drawer, footer. No session needed. */
export default async function SiteLayout({ children }: { children: React.ReactNode }) {
  const [jar, t] = await Promise.all([cookies(), getTranslations('shell')]);
  const { pricing } = siteFeatures();
  const analytics = analyticsScript();
  const nonce = (await headers()).get('x-nonce') ?? undefined; // F15a R452
  // Q-H11: a cookie only picks the header's button; the platform's own guard still decides.
  const signedIn = Boolean(jar.get(ACCESS_COOKIE)?.value || jar.get(REFRESH_COOKIE)?.value);
  const year = Number(new Intl.DateTimeFormat('en', { year: 'numeric', timeZone: 'Europe/Istanbul' }).format(new Date()));
  return (
    <div className="flex min-h-dvh flex-col bg-background text-foreground">
      <a
        href="#main-content"
        data-skip-link
        className="sr-only focus:not-sr-only focus:fixed focus:start-2 focus:top-2 focus:z-50 focus:inline-flex focus:min-h-11 focus:items-center focus:rounded-md focus:bg-primary focus:px-3 focus:text-on-primary"
      >
        {t('skipToContent')}
      </a>
      <AnnouncementBar />
      <SiteHeader pricing={pricing} signedIn={signedIn} />
      <main id="main-content" tabIndex={-1} className="flex-1">
        {children}
      </main>
      <SiteFooter year={year} pricing={pricing} />
      {analytics ? <Script defer nonce={nonce} data-domain={analytics.site} src={analytics.src} strategy="afterInteractive" /> : null}
    </div>
  );
}
