import { ToolkitView } from '@/features/site/catalogue-views';
import { pageMetadata } from '@/lib/site/metadata';

export const generateMetadata = () => pageMetadata('toolkit', '/toolkit');

export default function ToolkitPage() {
  return <ToolkitView />;
}
