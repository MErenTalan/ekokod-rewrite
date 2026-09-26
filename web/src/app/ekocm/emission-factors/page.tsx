import { CarbonSection } from '@/features/carbon/section-page';
import { sectionMetadata } from '@/modules/ekocm/metadata';

export const generateMetadata = () => sectionMetadata('database');

export default function Page() {
  return <CarbonSection section="database" />;
}
