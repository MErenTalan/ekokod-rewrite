import { ReferencesView } from '@/features/site/catalogue-views';
import { pageMetadata } from '@/lib/site/metadata';

export const generateMetadata = () => pageMetadata('references', '/references');

export default function ReferencesPage() {
  return <ReferencesView />;
}
