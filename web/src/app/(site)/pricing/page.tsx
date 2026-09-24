import { notFound } from 'next/navigation';

import { PricingView } from '@/features/site/pricing-view';
import { siteFeatures } from '@/lib/site/features';
import { pageMetadata } from '@/lib/site/metadata';

export const generateMetadata = () => pageMetadata('pricing', '/pricing');

/** Disabled in the legacy deployment: 404 unless EKOKOD_FEATURE_PRICING_PAGE is on (01 §7.19). */
export default function PricingPage() {
  if (!siteFeatures().pricing) notFound();
  return <PricingView />;
}
