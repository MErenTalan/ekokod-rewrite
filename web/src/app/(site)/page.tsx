import { JsonLd } from '@/features/site/json-ld';
import { HomeView } from '@/features/site/home-view';
import { siteFeatures } from '@/lib/site/features';
import { pageMetadata } from '@/lib/site/metadata';
import { siteUrl } from '@/lib/site/seo';
import { organizationLd } from '@/lib/site/structured-data';

export const generateMetadata = () => pageMetadata('home', '/');

export default function HomePage() {
  const base = siteUrl();
  return (
    <>
      <JsonLd data={[organizationLd(base), { '@context': 'https://schema.org', '@type': 'WebSite', name: 'EkoKod', url: base }]} />
      <HomeView pricing={siteFeatures().pricing} />
    </>
  );
}
