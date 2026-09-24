import { AboutView } from '@/features/site/about-view';
import { pageMetadata } from '@/lib/site/metadata';

export const generateMetadata = () => pageMetadata('about', '/about');

export default function AboutPage() {
  return <AboutView />;
}
