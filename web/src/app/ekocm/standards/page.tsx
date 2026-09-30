import { CarbonSection } from '@/features/carbon/section-page';
import { sectionMetadata } from '@/modules/ekocm/metadata';

export const generateMetadata = () => sectionMetadata('standards');

export default function Page() {
  return <CarbonSection section="standards" />;
}
