import { DocumentsView } from '@/features/site/catalogue-views';
import { pageMetadata } from '@/lib/site/metadata';

export const generateMetadata = () => pageMetadata('documents', '/documents');

export default function DocumentsPage() {
  return <DocumentsView />;
}
